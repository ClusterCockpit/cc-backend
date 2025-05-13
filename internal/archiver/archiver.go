// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archiver

import (
	"context"
	"math"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricDataDispatcher"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

// Writes a running job to the job-archive
func ArchiveJob(job *schema.Job, ctx context.Context) (*schema.JobMeta, error) {
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

	jobData, err := metricDataDispatcher.LoadData(job, allMetrics, scopes, ctx, 0) // 0 Resulotion-Value retrieves highest res (60s)
	if err != nil {
		log.Error("Error wile loading job data for archiving")
		return nil, err
	}

	jobMeta := &schema.JobMeta{
		BaseJob:    job.BaseJob,
		StartTime:  job.StartTime.Unix(),
		Statistics: make(map[string]schema.JobStatistics),
	}

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
		jobMeta.Statistics[metric] = schema.JobStatistics{
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
		return jobMeta, nil
	}

	return jobMeta, archive.GetHandle().ImportJob(jobMeta, &jobData)
}
