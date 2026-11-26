// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package graph

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/99designs/gqlgen/graphql"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

const MAX_JOBS_FOR_ANALYSIS = 1000

// Helper function for the rooflineHeatmap GraphQL query placed here so that schema.resolvers.go is not too full.
func (r *queryResolver) rooflineHeatmap(
	ctx context.Context,
	filter []*model.JobFilter,
	rows int, cols int,
	minX float64, minY float64, maxX float64, maxY float64,
) ([][]float64, error) {
	jobs, err := r.Repo.QueryJobs(ctx, filter, &model.PageRequest{Page: 1, ItemsPerPage: MAX_JOBS_FOR_ANALYSIS + 1}, nil)
	if err != nil {
		cclog.Error("Error while querying jobs for roofline")
		return nil, err
	}
	if len(jobs) > MAX_JOBS_FOR_ANALYSIS {
		return nil, fmt.Errorf("GRAPH/UTIL > too many jobs matched (max: %d)", MAX_JOBS_FOR_ANALYSIS)
	}

	fcols, frows := float64(cols), float64(rows)
	minX, minY, maxX, maxY = math.Log10(minX), math.Log10(minY), math.Log10(maxX), math.Log10(maxY)
	tiles := make([][]float64, rows)
	for i := range tiles {
		tiles[i] = make([]float64, cols)
	}

	for _, job := range jobs {
		if job.MonitoringStatus == schema.MonitoringStatusDisabled || job.MonitoringStatus == schema.MonitoringStatusArchivingFailed {
			continue
		}

		// metricConfigs := archive.GetCluster(job.Cluster).MetricConfig
		// resolution := 0

		// for _, mc := range metricConfigs {
		// 	resolution = max(resolution, mc.Timestep)
		// }

		jobdata, err := metricdispatch.LoadData(job, []string{"flops_any", "mem_bw"}, []schema.MetricScope{schema.MetricScopeNode}, ctx, 0, "")
		if err != nil {
			cclog.Warnf("Error while loading roofline metrics for job %d", *job.ID)
			return nil, err
		}

		flops_, membw_ := jobdata.Metrics["flops_any"], jobdata.Metrics["mem_bw"]
		if flops_ == nil && membw_ == nil {
			cclog.Warnf("rooflineHeatmap(): 'flops_any' or 'mem_bw' missing for job %d", *job.ID)
			continue
			// return nil, fmt.Errorf("GRAPH/UTIL > 'flops_any' or 'mem_bw' missing for job %d", job.ID)
		}

		flops, ok1 := flops_["node"]
		membw, ok2 := membw_["node"]
		if !ok1 || !ok2 {
			cclog.Info("rooflineHeatmap() query not implemented for where flops_any or mem_bw not available at 'node' level")
			continue
			// TODO/FIXME:
			// return nil, errors.New("GRAPH/UTIL > todo: rooflineHeatmap() query not implemented for where flops_any or mem_bw not available at 'node' level")
		}

		for n := 0; n < len(flops.Series); n++ {
			flopsSeries, membwSeries := flops.Series[n], membw.Series[n]
			for i := 0; i < len(flopsSeries.Data); i++ {
				if i >= len(membwSeries.Data) {
					break
				}

				x, y := math.Log10(float64(flopsSeries.Data[i]/membwSeries.Data[i])), math.Log10(float64(flopsSeries.Data[i]))
				if math.IsNaN(x) || math.IsNaN(y) || x < minX || x >= maxX || y < minY || y > maxY {
					continue
				}

				x, y = math.Floor(((x-minX)/(maxX-minX))*fcols), math.Floor(((y-minY)/(maxY-minY))*frows)
				if x < 0 || x >= fcols || y < 0 || y >= frows {
					continue
				}

				tiles[int(y)][int(x)] += 1
			}
		}
	}

	return tiles, nil
}

// Helper function for the jobsFootprints GraphQL query placed here so that schema.resolvers.go is not too full.
// NOTE: Legacy Naming! This helper is for normalized histograms in analysis view only - *Not* related to DB "footprint" column!
func (r *queryResolver) jobsFootprints(ctx context.Context, filter []*model.JobFilter, metrics []string) (*model.Footprints, error) {
	jobs, err := r.Repo.QueryJobs(ctx, filter, &model.PageRequest{Page: 1, ItemsPerPage: MAX_JOBS_FOR_ANALYSIS + 1}, nil)
	if err != nil {
		cclog.Error("Error while querying jobs for footprint")
		return nil, err
	}
	if len(jobs) > MAX_JOBS_FOR_ANALYSIS {
		return nil, fmt.Errorf("GRAPH/UTIL > too many jobs matched (max: %d)", MAX_JOBS_FOR_ANALYSIS)
	}

	avgs := make([][]schema.Float, len(metrics))
	for i := range avgs {
		avgs[i] = make([]schema.Float, 0, len(jobs))
	}

	timeweights := new(model.TimeWeights)
	timeweights.NodeHours = make([]schema.Float, 0, len(jobs))
	timeweights.AccHours = make([]schema.Float, 0, len(jobs))
	timeweights.CoreHours = make([]schema.Float, 0, len(jobs))

	for _, job := range jobs {
		if job.MonitoringStatus == schema.MonitoringStatusDisabled || job.MonitoringStatus == schema.MonitoringStatusArchivingFailed {
			continue
		}

		if err := metricdispatch.LoadAverages(job, metrics, avgs, ctx); err != nil {
			cclog.Error("Error while loading averages for footprint")
			return nil, err
		}

		// job.Duration is in seconds. Jobs without accelerator or hwthread
		// counts fall back to their node hours.
		hours := float64(job.Duration) / 3600.0
		nodeHours := schema.Float(hours * float64(job.NumNodes))
		timeweights.NodeHours = append(timeweights.NodeHours, nodeHours)
		if job.NumAcc > 0 {
			timeweights.AccHours = append(timeweights.AccHours, schema.Float(hours*float64(job.NumAcc)))
		} else {
			timeweights.AccHours = append(timeweights.AccHours, nodeHours)
		}
		if job.NumHWThreads > 0 {
			timeweights.CoreHours = append(timeweights.CoreHours, schema.Float(hours*float64(job.NumHWThreads))) // SQLite HWThreads == Cores; numCoresForJob(job)
		} else {
			timeweights.CoreHours = append(timeweights.CoreHours, nodeHours)
		}
	}

	res := make([]*model.MetricFootprints, len(avgs))
	for i, arr := range avgs {
		res[i] = &model.MetricFootprints{
			Metric: metrics[i],
			Data:   arr,
		}
	}

	return &model.Footprints{
		TimeWeights: timeweights,
		Metrics:     res,
	}, nil
}

// func numCoresForJob(job *schema.Job) (numCores int) {

// 	subcluster, scerr := archive.GetSubCluster(job.Cluster, job.SubCluster)
// 	if scerr != nil {
// 		return 1
// 	}

// 	totalJobCores := 0
// 	topology := subcluster.Topology

// 	for _, host := range job.Resources {
// 		hwthreads := host.HWThreads
// 		if hwthreads == nil {
// 			hwthreads = topology.Node
// 		}

// 		hostCores, _ := topology.GetCoresFromHWThreads(hwthreads)
// 		totalJobCores += len(hostCores)
// 	}

// 	return totalJobCores
// }

func requireField(ctx context.Context, name string) bool {
	fields := graphql.CollectAllFields(ctx)

	return slices.Contains(fields, name)
}
