// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricdispatcher provides a unified interface for loading and caching job metric data.
//
// This package serves as a central dispatcher that routes metric data requests to the appropriate
// backend based on job state. For running jobs, data is fetched from the metric store (e.g., cc-metric-store).
// For completed jobs, data is retrieved from the file-based job archive.
//
// # Key Features
//
//   - Automatic backend selection based on job state (running vs. archived)
//   - LRU cache for performance optimization (128 MB default cache size)
//   - Data resampling using Largest Triangle Three Bucket algorithm for archived data
//   - Automatic statistics series generation for jobs with many nodes
//   - Support for scoped metrics (node, socket, accelerator, core)
//
// # Cache Behavior
//
// Cached data has different TTL (time-to-live) values depending on job state:
//   - Running jobs: 2 minutes (data changes frequently)
//   - Completed jobs: 5 hours (data is static)
//
// The cache key is based on job ID, state, requested metrics, scopes, and resolution.
//
// # Usage
//
// The primary entry point is LoadData, which automatically handles both running and archived jobs:
//
//	jobData, err := metricdispatcher.LoadData(job, metrics, scopes, ctx, resolution)
//	if err != nil {
//	    // Handle error
//	}
//
// For statistics only, use LoadJobStats, LoadScopedJobStats, or LoadAverages depending on the required format.
package metricdispatcher

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/memorystore"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/lrucache"
	"github.com/ClusterCockpit/cc-lib/resampler"
	"github.com/ClusterCockpit/cc-lib/schema"
)

// cache is an LRU cache with 128 MB capacity for storing loaded job metric data.
// The cache reduces load on both the metric store and archive backends.
var cache *lrucache.Cache = lrucache.New(128 * 1024 * 1024)

// cacheKey generates a unique cache key for a job's metric data based on job ID, state,
// requested metrics, scopes, and resolution. Duration and StartTime are intentionally excluded
// because job.ID is more unique and the cache TTL ensures entries don't persist indefinitely.
func cacheKey(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
) string {
	return fmt.Sprintf("%d(%s):[%v],[%v]-%d",
		job.ID, job.State, metrics, scopes, resolution)
}

// LoadData retrieves metric data for a job from the appropriate backend (memory store for running jobs,
// archive for completed jobs) and applies caching, resampling, and statistics generation as needed.
//
// For running jobs or when archive is disabled, data is fetched from the metric store.
// For completed archived jobs, data is loaded from the job archive and resampled if needed.
//
// Parameters:
//   - job: The job for which to load metric data
//   - metrics: List of metric names to load (nil loads all metrics for the cluster)
//   - scopes: Metric scopes to include (nil defaults to node scope)
//   - ctx: Context for cancellation and timeouts
//   - resolution: Target number of data points for resampling (only applies to archived data)
//
// Returns the loaded job data and any error encountered. For partial errors (some metrics failed),
// the function returns the successfully loaded data with a warning logged.
func LoadData(job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int,
) (schema.JobData, error) {
	data := cache.Get(cacheKey(job, metrics, scopes, resolution), func() (_ any, ttl time.Duration, size int) {
		var jd schema.JobData
		var err error

		if job.State == schema.JobStateRunning ||
			job.MonitoringStatus == schema.MonitoringStatusRunningOrArchiving ||
			config.Keys.DisableArchive {

			if scopes == nil {
				scopes = append(scopes, schema.MetricScopeNode)
			}

			if metrics == nil {
				cluster := archive.GetCluster(job.Cluster)
				for _, mc := range cluster.MetricConfig {
					metrics = append(metrics, mc.Name)
				}
			}

			jd, err = memorystore.LoadData(job, metrics, scopes, ctx, resolution)
			if err != nil {
				if len(jd) != 0 {
					cclog.Warnf("partial error loading metrics from store for job %d (user: %s, project: %s): %s",
						job.JobID, job.User, job.Project, err.Error())
				} else {
					cclog.Errorf("failed to load job data from metric store for job %d (user: %s, project: %s): %s",
						job.JobID, job.User, job.Project, err.Error())
					return err, 0, 0
				}
			}
			size = jd.Size()
		} else {
			var jdTemp schema.JobData
			jdTemp, err = archive.GetHandle().LoadJobData(job)
			if err != nil {
				cclog.Errorf("failed to load job data from archive for job %d (user: %s, project: %s): %s",
					job.JobID, job.User, job.Project, err.Error())
				return err, 0, 0
			}

			jd = deepCopy(jdTemp)

			// Resample archived data using Largest Triangle Three Bucket algorithm to reduce data points
			// to the requested resolution, improving transfer performance and client-side rendering.
			for _, v := range jd {
				for _, v_ := range v {
					timestep := int64(0)
					for i := 0; i < len(v_.Series); i += 1 {
						v_.Series[i].Data, timestep, err = resampler.LargestTriangleThreeBucket(v_.Series[i].Data, int64(v_.Timestep), int64(resolution))
						if err != nil {
							return err, 0, 0
						}
					}
					v_.Timestep = int(timestep)
				}
			}

			// Filter job data to only include requested metrics and scopes, avoiding unnecessary data transfer.
			if metrics != nil || scopes != nil {
				if metrics == nil {
					metrics = make([]string, 0, len(jd))
					for k := range jd {
						metrics = append(metrics, k)
					}
				}

				res := schema.JobData{}
				for _, metric := range metrics {
					if perscope, ok := jd[metric]; ok {
						if len(perscope) > 1 {
							subset := make(map[schema.MetricScope]*schema.JobMetric)
							for _, scope := range scopes {
								if jm, ok := perscope[scope]; ok {
									subset[scope] = jm
								}
							}

							if len(subset) > 0 {
								perscope = subset
							}
						}

						res[metric] = perscope
					}
				}
				jd = res
			}
			size = jd.Size()
		}

		ttl = 5 * time.Hour
		if job.State == schema.JobStateRunning {
			ttl = 2 * time.Minute
		}

		// Generate statistics series for jobs with many nodes to enable min/median/max graphs
		// instead of overwhelming the UI with individual node lines. Note that newly calculated
		// statistics use min/median/max, while archived statistics may use min/mean/max.
		const maxSeriesSize int = 15
		for _, scopes := range jd {
			for _, jm := range scopes {
				if jm.StatisticsSeries != nil || len(jm.Series) <= maxSeriesSize {
					continue
				}

				jm.AddStatisticsSeries()
			}
		}

		nodeScopeRequested := false
		for _, scope := range scopes {
			if scope == schema.MetricScopeNode {
				nodeScopeRequested = true
			}
		}

		if nodeScopeRequested {
			jd.AddNodeScope("flops_any")
			jd.AddNodeScope("mem_bw")
		}

		// Round Resulting Stat Values
		jd.RoundMetricStats()

		return jd, ttl, size
	})

	if err, ok := data.(error); ok {
		cclog.Errorf("error in cached dataset for job %d: %s", job.JobID, err.Error())
		return nil, err
	}

	return data.(schema.JobData), nil
}

// LoadAverages computes average values for the specified metrics across all nodes of a job.
// For running jobs, it loads statistics from the metric store. For completed jobs, it uses
// the pre-calculated averages from the job archive. The results are appended to the data slice.
func LoadAverages(
	job *schema.Job,
	metrics []string,
	data [][]schema.Float,
	ctx context.Context,
) error {
	if job.State != schema.JobStateRunning && !config.Keys.DisableArchive {
		return archive.LoadAveragesFromArchive(job, metrics, data) // #166 change also here?
	}

	stats, err := memorystore.LoadStats(job, metrics, ctx)
	if err != nil {
		cclog.Errorf("failed to load statistics from metric store for job %d (user: %s, project: %s): %s",
			job.JobID, job.User, job.Project, err.Error())
		return err
	}

	for i, m := range metrics {
		nodes, ok := stats[m]
		if !ok {
			data[i] = append(data[i], schema.NaN)
			continue
		}

		sum := 0.0
		for _, node := range nodes {
			sum += node.Avg
		}
		data[i] = append(data[i], schema.Float(sum))
	}

	return nil
}

// LoadScopedJobStats retrieves job statistics organized by metric scope (node, socket, core, accelerator).
// For running jobs, statistics are computed from the metric store. For completed jobs, pre-calculated
// statistics are loaded from the job archive.
func LoadScopedJobStats(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
) (schema.ScopedJobStats, error) {
	if job.State != schema.JobStateRunning && !config.Keys.DisableArchive {
		return archive.LoadScopedStatsFromArchive(job, metrics, scopes)
	}

	scopedStats, err := memorystore.LoadScopedStats(job, metrics, scopes, ctx)
	if err != nil {
		cclog.Errorf("failed to load scoped statistics from metric store for job %d (user: %s, project: %s): %s",
			job.JobID, job.User, job.Project, err.Error())
		return nil, err
	}

	return scopedStats, nil
}

// LoadJobStats retrieves aggregated statistics (min/avg/max) for each requested metric across all job nodes.
// For running jobs, statistics are computed from the metric store. For completed jobs, pre-calculated
// statistics are loaded from the job archive.
func LoadJobStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context,
) (map[string]schema.MetricStatistics, error) {
	if job.State != schema.JobStateRunning && !config.Keys.DisableArchive {
		return archive.LoadStatsFromArchive(job, metrics)
	}

	data := make(map[string]schema.MetricStatistics, len(metrics))

	stats, err := memorystore.LoadStats(job, metrics, ctx)
	if err != nil {
		cclog.Errorf("failed to load statistics from metric store for job %d (user: %s, project: %s): %s",
			job.JobID, job.User, job.Project, err.Error())
		return data, err
	}

	for _, m := range metrics {
		sum, avg, min, max := 0.0, 0.0, 0.0, 0.0
		nodes, ok := stats[m]
		if !ok {
			data[m] = schema.MetricStatistics{Min: min, Avg: avg, Max: max}
			continue
		}

		for _, node := range nodes {
			sum += node.Avg
			min = math.Min(min, node.Min)
			max = math.Max(max, node.Max)
		}

		data[m] = schema.MetricStatistics{
			Avg: (math.Round((sum/float64(job.NumNodes))*100) / 100),
			Min: (math.Round(min*100) / 100),
			Max: (math.Round(max*100) / 100),
		}
	}

	return data, nil
}

// LoadNodeData retrieves metric data for specific nodes in a cluster within a time range.
// This is used for node monitoring views and system status pages. Data is always fetched from
// the metric store (not the archive) since it's for current/recent node status monitoring.
//
// Returns a nested map structure: node -> metric -> scoped data.
func LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context,
) (map[string]map[string][]*schema.JobMetric, error) {
	if metrics == nil {
		for _, m := range archive.GetCluster(cluster).MetricConfig {
			metrics = append(metrics, m.Name)
		}
	}

	data, err := memorystore.LoadNodeData(cluster, metrics, nodes, scopes, from, to, ctx)
	if err != nil {
		if len(data) != 0 {
			cclog.Warnf("partial error loading node data from metric store for cluster %s: %s", cluster, err.Error())
		} else {
			cclog.Errorf("failed to load node data from metric store for cluster %s: %s", cluster, err.Error())
			return nil, err
		}
	}

	if data == nil {
		return nil, fmt.Errorf("metric store for cluster '%s' does not support node data queries", cluster)
	}

	return data, nil
}

// LoadNodeListData retrieves time-series metric data for multiple nodes within a time range,
// with optional resampling and automatic statistics generation for large datasets.
// This is used for comparing multiple nodes or displaying node status over time.
//
// Returns a map of node names to their job-like metric data structures.
func LoadNodeListData(
	cluster, subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	ctx context.Context,
) (map[string]schema.JobData, error) {
	if metrics == nil {
		for _, m := range archive.GetCluster(cluster).MetricConfig {
			metrics = append(metrics, m.Name)
		}
	}

	data, err := memorystore.LoadNodeListData(cluster, subCluster, nodes, metrics, scopes, resolution, from, to, ctx)
	if err != nil {
		if len(data) != 0 {
			cclog.Warnf("partial error loading node list data from metric store for cluster %s, subcluster %s: %s",
				cluster, subCluster, err.Error())
		} else {
			cclog.Errorf("failed to load node list data from metric store for cluster %s, subcluster %s: %s",
				cluster, subCluster, err.Error())
			return nil, err
		}
	}

	// Generate statistics series for datasets with many series to improve visualization performance.
	// Statistics are calculated as min/median/max.
	const maxSeriesSize int = 8
	for _, jd := range data {
		for _, scopes := range jd {
			for _, jm := range scopes {
				if jm.StatisticsSeries != nil || len(jm.Series) < maxSeriesSize {
					continue
				}
				jm.AddStatisticsSeries()
			}
		}
	}

	if data == nil {
		return nil, fmt.Errorf("metric store for cluster '%s' does not support node list queries", cluster)
	}

	return data, nil
}

// deepCopy creates a deep copy of JobData to prevent cache corruption when modifying
// archived data (e.g., during resampling). This ensures the cached archive data remains
// immutable while allowing per-request transformations.
func deepCopy(source schema.JobData) schema.JobData {
	result := make(schema.JobData, len(source))

	for metricName, scopeMap := range source {
		result[metricName] = make(map[schema.MetricScope]*schema.JobMetric, len(scopeMap))

		for scope, jobMetric := range scopeMap {
			result[metricName][scope] = copyJobMetric(jobMetric)
		}
	}

	return result
}

func copyJobMetric(src *schema.JobMetric) *schema.JobMetric {
	dst := &schema.JobMetric{
		Timestep: src.Timestep,
		Unit:     src.Unit,
		Series:   make([]schema.Series, len(src.Series)),
	}

	for i := range src.Series {
		dst.Series[i] = copySeries(&src.Series[i])
	}

	if src.StatisticsSeries != nil {
		dst.StatisticsSeries = copyStatisticsSeries(src.StatisticsSeries)
	}

	return dst
}

func copySeries(src *schema.Series) schema.Series {
	dst := schema.Series{
		Hostname:   src.Hostname,
		Id:         src.Id,
		Statistics: src.Statistics,
		Data:       make([]schema.Float, len(src.Data)),
	}

	copy(dst.Data, src.Data)
	return dst
}

func copyStatisticsSeries(src *schema.StatsSeries) *schema.StatsSeries {
	dst := &schema.StatsSeries{
		Min:    make([]schema.Float, len(src.Min)),
		Mean:   make([]schema.Float, len(src.Mean)),
		Median: make([]schema.Float, len(src.Median)),
		Max:    make([]schema.Float, len(src.Max)),
	}

	copy(dst.Min, src.Min)
	copy(dst.Mean, src.Mean)
	copy(dst.Median, src.Median)
	copy(dst.Max, src.Max)

	if len(src.Percentiles) > 0 {
		dst.Percentiles = make(map[int][]schema.Float, len(src.Percentiles))
		for percentile, values := range src.Percentiles {
			dst.Percentiles[percentile] = make([]schema.Float, len(values))
			copy(dst.Percentiles[percentile], values)
		}
	}

	return dst
}
