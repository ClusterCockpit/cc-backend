// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package graph

import (
	"context"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
)

// resolveResolutionFromPolicy reads the user's resample policy preference and
// computes a resolution based on job duration and metric frequency. Returns nil
// if the user has no policy set.
func resolveResolutionFromPolicy(ctx context.Context, duration int64, cluster string, metrics []string) *int {
	user := repository.GetUserFromContext(ctx)
	if user == nil {
		return nil
	}

	conf, err := repository.GetUserCfgRepo().GetUIConfig(user)
	if err != nil {
		return nil
	}

	policyVal, ok := conf["plotConfiguration_resamplePolicy"]
	if !ok {
		return nil
	}
	policyStr, ok := policyVal.(string)
	if !ok || policyStr == "" {
		return nil
	}

	policy := metricdispatch.ResamplePolicy(policyStr)
	targetPoints := metricdispatch.TargetPointsForPolicy(policy)
	if targetPoints == 0 {
		return nil
	}

	// Find the smallest metric frequency across the requested metrics
	frequency := smallestFrequency(cluster, metrics)
	if frequency <= 0 {
		return nil
	}

	res := metricdispatch.ComputeResolution(duration, int64(frequency), targetPoints)
	return &res
}

// resolveResampleAlgo returns the resampling algorithm name to use, checking
// the explicit GraphQL parameter first, then the user's preference.
func resolveResampleAlgo(ctx context.Context, resampleAlgo *model.ResampleAlgo) string {
	if resampleAlgo != nil {
		return strings.ToLower(resampleAlgo.String())
	}

	user := repository.GetUserFromContext(ctx)
	if user == nil {
		return ""
	}

	conf, err := repository.GetUserCfgRepo().GetUIConfig(user)
	if err != nil {
		return ""
	}

	algoVal, ok := conf["plotConfiguration_resampleAlgo"]
	if !ok {
		return ""
	}
	algoStr, ok := algoVal.(string)
	if !ok {
		return ""
	}

	return algoStr
}

// smallestFrequency returns the smallest metric timestep (in seconds) among the
// requested metrics for the given cluster. Falls back to 0 if nothing is found.
func smallestFrequency(cluster string, metrics []string) int {
	cl := archive.GetCluster(cluster)
	if cl == nil {
		return 0
	}

	minFreq := 0
	for _, mc := range cl.MetricConfig {
		if len(metrics) > 0 {
			found := false
			for _, m := range metrics {
				if mc.Name == m {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if minFreq == 0 || mc.Timestep < minFreq {
			minFreq = mc.Timestep
		}
	}

	return minFreq
}
