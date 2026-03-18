// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type UpdateNodeStatesRequest struct {
	Nodes   []schema.NodePayload `json:"nodes"`
	Cluster string               `json:"cluster" example:"fritz"`
}

// metricListToNames converts a map of metric configurations to a list of metric names
func metricListToNames(metricList map[string]*schema.Metric) []string {
	names := make([]string, 0, len(metricList))
	for name := range metricList {
		names = append(names, name)
	}
	return names
}

// determineState resolves multiple states to a single state using priority order:
// allocated > reserved > idle > down > mixed.
// Exception: if both idle and down are present, down is returned.
func determineState(states []string) schema.SchedulerState {
	stateSet := make(map[string]bool, len(states))
	for _, s := range states {
		stateSet[strings.ToLower(s)] = true
	}

	switch {
	case stateSet["allocated"]:
		return schema.NodeStateAllocated
	case stateSet["reserved"]:
		return schema.NodeStateReserved
	case stateSet["idle"] && stateSet["down"]:
		return schema.NodeStateDown
	case stateSet["idle"]:
		return schema.NodeStateIdle
	case stateSet["down"]:
		return schema.NodeStateDown
	case stateSet["mixed"]:
		return schema.NodeStateMixed
	}

	return schema.NodeStateUnknown
}

// updateNodeStates godoc
// @summary     Deliver updated Slurm node states
// @tags Nodestates
// @description Returns a JSON-encoded list of users.
// @description Required query-parameter defines if all users or only users with additional special roles are returned.
// @produce     json
// @param       request body UpdateNodeStatesRequest true "Request body containing nodes and their states"
// @success     200     {object} api.DefaultAPIResponse "Success message"
// @failure     400     {object} api.ErrorResponse      "Bad Request"
// @failure     401     {object} api.ErrorResponse      "Unauthorized"
// @failure     403     {object} api.ErrorResponse      "Forbidden"
// @failure     500     {object} api.ErrorResponse      "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/nodestats/ [post]
func (api *RestAPI) updateNodeStates(rw http.ResponseWriter, r *http.Request) {
	// Parse request body
	req := UpdateNodeStatesRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err),
			http.StatusBadRequest, rw)
		return
	}
	requestReceived := time.Now().Unix()
	repo := repository.GetNodeRepository()

	// Step 1: Pre-compute node states; only include non-down nodes in health check
	nodeStates := make(map[string]schema.SchedulerState, len(req.Nodes))
	for _, node := range req.Nodes {
		nodeStates[node.Hostname] = determineState(node.States)
	}

	m := make(map[string][]string)
	metricNames := make(map[string][]string)
	healthResults := make(map[string]metricstore.HealthCheckResult)

	startMs := time.Now()

	// Step 2: Build nodeList and metricList per subcluster, skipping down nodes
	for _, node := range req.Nodes {
		if nodeStates[node.Hostname] == schema.NodeStateDown {
			continue
		}
		if sc, err := archive.GetSubClusterByNode(req.Cluster, node.Hostname); err == nil {
			m[sc] = append(m[sc], node.Hostname)
		}
	}

	for sc := range m {
		if sc != "" {
			metricList := archive.GetMetricConfigSubCluster(req.Cluster, sc)
			metricNames[sc] = metricListToNames(metricList)
		}
	}

	// Step 3: Determine which metric store to query and perform health check
	healthRepo, err := metricdispatch.GetHealthCheckRepo(req.Cluster)
	if err != nil {
		cclog.Warnf("updateNodeStates: no metric store for cluster %s, skipping health check: %v", req.Cluster, err)
	} else {
		for sc, nl := range m {
			if sc != "" {
				if results, err := healthRepo.HealthCheck(req.Cluster, nl, metricNames[sc]); err == nil {
					maps.Copy(healthResults, results)
				}
			}
		}
	}

	cclog.Debugf("Timer updateNodeStates, MemStore HealthCheck: %s", time.Since(startMs))
	startDB := time.Now()

	updates := make([]repository.NodeStateUpdate, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		state := nodeStates[node.Hostname]
		var healthState schema.MonitoringState
		var healthMetrics string
		if state == schema.NodeStateDown {
			healthState = schema.MonitoringStateFull
		} else {
			healthState = schema.MonitoringStateFailed
			if result, ok := healthResults[node.Hostname]; ok {
				healthState = result.State
				healthMetrics = result.HealthMetrics
			}
		}
		nodeState := schema.NodeStateDB{
			TimeStamp:       requestReceived,
			NodeState:       state,
			CpusAllocated:   node.CpusAllocated,
			MemoryAllocated: node.MemoryAllocated,
			GpusAllocated:   node.GpusAllocated,
			HealthState:     healthState,
			HealthMetrics:   healthMetrics,
			JobsRunning:     node.JobsRunning,
		}
		updates = append(updates, repository.NodeStateUpdate{
			Hostname:  node.Hostname,
			Cluster:   req.Cluster,
			NodeState: &nodeState,
		})
	}

	if err := repo.BatchUpdateNodeStates(updates); err != nil {
		cclog.Errorf("updateNodeStates: batch update for cluster %s failed: %v", req.Cluster, err)
	}

	cclog.Debugf("Timer updateNodeStates, SQLite Inserts: %s", time.Since(startDB))
}
