// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archiver

import (
	"context"
	"math"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatcher"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
)

// ArchiveJob archives a completed job's metric data to the configured archive backend.
//
// This function performs the following operations:
//  1. Loads all metric data for the job from the metric data repository
//  2. Calculates job-level statistics (avg, min, max) for each metric
//  3. Stores the job metadata and metric data to the archive backend
//
// Metric data is retrieved at the highest available resolution (typically 60s)
// for the following scopes:
//   - Node scope (always)
//   - Core scope (for jobs with ≤8 nodes, to reduce data volume)
//   - Accelerator scope (if job used accelerators)
//
// The function respects context cancellation. If ctx is cancelled (e.g., during
// shutdown timeout), the operation will be interrupted and return an error.
//
// Parameters:
//   - job: The job to archive (must be a completed job)
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - *schema.Job with populated Statistics field
//   - error if data loading or archiving fails
//
// If config.Keys.DisableArchive is true, only job statistics are calculated
// and returned (no data is written to archive backend).
func ArchiveJob(job *schema.Job, ctx context.Context) (*schema.Job, error) {
	allMetrics := make([]string, 0)
	metricConfigs := archive.GetCluster(job.Cluster).MetricConfig
	for _, mc := range metricConfigs {
		allMetrics = append(allMetrics, mc.Name)
	}

	scopes := []schema.MetricScope{schema.MetricScopeNode}
	// FIXME: Add a config option for this
	if job.NumNodes <= 8 {
		// This will add the native scope if core scope is not available
		scopes = append(scopes, schema.MetricScopeCore)
	}

	if job.NumAcc > 0 {
		scopes = append(scopes, schema.MetricScopeAccelerator)
	}

	jobData, err := metricdispatcher.LoadData(job, allMetrics, scopes, ctx, 0) // 0 Resulotion-Value retrieves highest res (60s)
	if err != nil {
		cclog.Error("Error wile loading job data for archiving")
		return nil, err
	}

	job.Statistics = make(map[string]schema.JobStatistics)

	for metric, data := range jobData {
		avg, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32
		nodeData, ok := data["node"]
		if !ok {
			// This should never happen ?
			continue
		}

		for _, series := range nodeData.Series {
			avg += series.Statistics.Avg
			min = math.Min(min, series.Statistics.Min)
			max = math.Max(max, series.Statistics.Max)
		}

		// Round AVG Result to 2 Digits
		job.Statistics[metric] = schema.JobStatistics{
			Unit: schema.Unit{
				Prefix: archive.GetMetricConfig(job.Cluster, metric).Unit.Prefix,
				Base:   archive.GetMetricConfig(job.Cluster, metric).Unit.Base,
			},
			Avg: (math.Round((avg/float64(job.NumNodes))*100) / 100),
			Min: min,
			Max: max,
		}
	}

	// If the file based archive is disabled,
	// only return the JobMeta structure as the
	// statistics in there are needed.
	if config.Keys.DisableArchive {
		return job, nil
	}

	return job, archive.GetHandle().ImportJob(job, &jobData)
}
