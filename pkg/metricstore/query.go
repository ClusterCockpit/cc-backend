// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file implements high-level query functions for loading job metric data
// with automatic scope transformation and aggregation.
//
// Key Concepts:
//
// Metric Scopes: Metrics are collected at different granularities (native scope):
//   - HWThread: Per hardware thread
//   - Core: Per CPU core
//   - Socket: Per CPU socket
//   - MemoryDomain: Per memory domain (NUMA)
//   - Accelerator: Per GPU/accelerator
//   - Node: Per compute node
//
// Scope Transformation: The buildQueries functions transform between native scope
// and requested scope by:
//   - Aggregating finer-grained data (e.g., HWThread → Core → Socket → Node)
//   - Rejecting requests for finer granularity than available
//   - Handling special cases (e.g., Accelerator metrics)
//
// Query Building: Constructs APIQuery structures with proper selectors (Type, TypeIds)
// based on cluster topology and job resources.
package metricstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// TestLoadDataCallback allows tests to override LoadData behavior for testing purposes.
// When set to a non-nil function, LoadData will call this function instead of the default implementation.
var TestLoadDataCallback func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error)

// LoadData loads metric data for a specific job with automatic scope transformation.
//
// This is the primary function for retrieving job metric data. It handles:
//   - Building queries with scope transformation via buildQueries
//   - Fetching data from the metric store
//   - Organizing results by metric and scope
//   - Converting NaN statistics to 0 for JSON compatibility
//   - Partial error handling (returns data for successful queries even if some fail)
//
// Parameters:
//   - job: Job metadata including cluster, resources, and time range
//   - metrics: List of metric names to load
//   - scopes: Requested metric scopes (will be transformed to match native scopes)
//   - ctx: Context for cancellation (currently unused but reserved for future use)
//   - resolution: Data resolution in seconds (0 for native resolution)
//
// Returns:
//   - JobData: Map of metric → scope → JobMetric with time-series data and statistics
//   - Error: Returns error if query building or fetching fails, or partial error listing failed hosts
//
// Example:
//
//	jobData, err := LoadData(job, []string{"cpu_load", "mem_used"}, []schema.MetricScope{schema.MetricScopeNode}, ctx, 60)
func LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int,
) (schema.JobData, error) {
	if TestLoadDataCallback != nil {
		return TestLoadDataCallback(job, metrics, scopes, ctx, resolution)
	}

	queries, assignedScope, err := buildQueries(job, metrics, scopes, int64(resolution))
	if err != nil {
		cclog.Errorf("Error while building queries for jobId %d, Metrics %v, Scopes %v: %s", job.JobID, metrics, scopes, err.Error())
		return nil, err
	}

	req := APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  true,
	}

	resBody, err := FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	var errors []string
	jobData := make(schema.JobData)
	for i, row := range resBody.Results {
		query := req.Queries[i]
		metric := query.Metric
		scope := assignedScope[i]
		mc := archive.GetMetricConfig(job.Cluster, metric)
		if _, ok := jobData[metric]; !ok {
			jobData[metric] = make(map[schema.MetricScope]*schema.JobMetric)
		}

		res := mc.Timestep
		if len(row) > 0 {
			res = int(row[0].Resolution)
		}

		jobMetric, ok := jobData[metric][scope]
		if !ok {
			jobMetric = &schema.JobMetric{
				Unit:     mc.Unit,
				Timestep: res,
				Series:   make([]schema.Series, 0),
			}
			jobData[metric][scope] = jobMetric
		}

		for ndx, res := range row {
			if res.Error != nil {
				/* Build list for "partial errors", if any */
				errors = append(errors, fmt.Sprintf("failed to fetch '%s' from host '%s': %s", query.Metric, query.Hostname, *res.Error))
				continue
			}

			id := (*string)(nil)
			if query.Type != nil {
				id = new(string)
				*id = query.TypeIds[ndx]
			}

			sanitizeStats(&res)

			jobMetric.Series = append(jobMetric.Series, schema.Series{
				Hostname: query.Hostname,
				Id:       id,
				Statistics: schema.MetricStatistics{
					Avg: float64(res.Avg),
					Min: float64(res.Min),
					Max: float64(res.Max),
				},
				Data: res.Data,
			})
		}

		// So that one can later check len(jobData):
		if len(jobMetric.Series) == 0 {
			delete(jobData[metric], scope)
			if len(jobData[metric]) == 0 {
				delete(jobData, metric)
			}
		}
	}

	if len(errors) != 0 {
		/* Returns list for "partial errors" */
		return jobData, fmt.Errorf("METRICDATA/CCMS > Errors: %s", strings.Join(errors, ", "))
	}
	return jobData, nil
}

// Pre-converted scope strings avoid repeated string(MetricScope) allocations during
// query construction. These are used in APIQuery.Type field throughout buildQueries
// and buildNodeQueries functions. Converting once at package initialization improves
// performance for high-volume query building.
var (
	hwthreadString     = string(schema.MetricScopeHWThread)
	coreString         = string(schema.MetricScopeCore)
	memoryDomainString = string(schema.MetricScopeMemoryDomain)
	socketString       = string(schema.MetricScopeSocket)
	acceleratorString  = string(schema.MetricScopeAccelerator)
)

// buildQueries constructs APIQuery structures with automatic scope transformation for a job.
//
// This function implements the core scope transformation logic, handling all combinations of
// native metric scopes and requested scopes. It uses the cluster topology to determine which
// hardware IDs to include in each query.
//
// Scope Transformation Rules:
//   - If native scope >= requested scope: Aggregates data (Aggregate=true in APIQuery)
//   - If native scope < requested scope: Returns error (cannot increase granularity)
//   - Special handling for Accelerator scope (independent of CPU hierarchy)
//
// The function generates one or more APIQuery per (metric, scope, host) combination:
//   - For non-aggregated queries: One query with all relevant IDs
//   - For aggregated queries: May generate multiple queries (e.g., one per socket/core)
//
// Parameters:
//   - job: Job metadata including cluster, subcluster, and resource allocation
//   - metrics: List of metrics to query
//   - scopes: Requested scopes for each metric
//   - resolution: Data resolution in seconds
//
// Returns:
//   - []APIQuery: List of queries to execute
//   - []schema.MetricScope: Assigned scope for each query (after transformation)
//   - error: Returns error if topology lookup fails or unhandled scope combination encountered
func buildQueries(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int64,
) ([]APIQuery, []schema.MetricScope, error) {
	if len(job.Resources) == 0 {
		return nil, nil, fmt.Errorf("METRICDATA/CCMS > no resources allocated for job %d", job.JobID)
	}

	queries := make([]APIQuery, 0, len(metrics)*len(scopes)*len(job.Resources))
	assignedScope := []schema.MetricScope{}

	subcluster, scerr := archive.GetSubCluster(job.Cluster, job.SubCluster)
	if scerr != nil {
		return nil, nil, scerr
	}
	topology := subcluster.Topology

	for _, metric := range metrics {
		mc := archive.GetMetricConfig(job.Cluster, metric)
		if mc == nil {
			cclog.Infof("metric '%s' is not specified for cluster '%s'", metric, job.Cluster)
			continue
		}

		// Skip if metric is removed for subcluster
		if len(mc.SubClusters) != 0 {
			isRemoved := false
			for _, scConfig := range mc.SubClusters {
				if scConfig.Name == job.SubCluster && scConfig.Remove {
					isRemoved = true
					break
				}
			}
			if isRemoved {
				continue
			}
		}

		// Avoid duplicates using map for O(1) lookup
		handledScopes := make(map[schema.MetricScope]bool, 3)

		for _, requestedScope := range scopes {
			nativeScope := mc.Scope
			if nativeScope == schema.MetricScopeAccelerator && job.NumAcc == 0 {
				continue
			}

			scope := nativeScope.Max(requestedScope)
			if handledScopes[scope] {
				continue
			}
			handledScopes[scope] = true

			for _, host := range job.Resources {
				hwthreads := host.HWThreads
				if hwthreads == nil {
					hwthreads = topology.Node
				}

				// Accelerator -> Accelerator (Use "accelerator" scope if requested scope is lower than node)
				if nativeScope == schema.MetricScopeAccelerator && scope.LT(schema.MetricScopeNode) {
					if scope != schema.MetricScopeAccelerator {
						// Skip all other catched cases
						continue
					}

					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  false,
						Type:       &acceleratorString,
						TypeIds:    host.Accelerators,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, schema.MetricScopeAccelerator)
					continue
				}

				// Accelerator -> Node
				if nativeScope == schema.MetricScopeAccelerator && scope == schema.MetricScopeNode {
					if len(host.Accelerators) == 0 {
						continue
					}

					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  true,
						Type:       &acceleratorString,
						TypeIds:    host.Accelerators,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// HWThread -> HWThread
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  false,
						Type:       &hwthreadString,
						TypeIds:    intToStringSlice(hwthreads),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// HWThread -> Core
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					for _, core := range cores {
						queries = append(queries, APIQuery{
							Metric:     metric,
							Hostname:   host.Hostname,
							Aggregate:  true,
							Type:       &hwthreadString,
							TypeIds:    intToStringSlice(topology.Core[core]),
							Resolution: resolution,
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// HWThread -> Socket
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					for _, socket := range sockets {
						queries = append(queries, APIQuery{
							Metric:     metric,
							Hostname:   host.Hostname,
							Aggregate:  true,
							Type:       &hwthreadString,
							TypeIds:    intToStringSlice(topology.Socket[socket]),
							Resolution: resolution,
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// HWThread -> Node
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeNode {
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  true,
						Type:       &hwthreadString,
						TypeIds:    intToStringSlice(hwthreads),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Core -> Core
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  false,
						Type:       &coreString,
						TypeIds:    intToStringSlice(cores),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Core -> Socket
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromCores(hwthreads)
					for _, socket := range sockets {
						queries = append(queries, APIQuery{
							Metric:     metric,
							Hostname:   host.Hostname,
							Aggregate:  true,
							Type:       &coreString,
							TypeIds:    intToStringSlice(topology.Socket[socket]),
							Resolution: resolution,
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// Core -> Node
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeNode {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  true,
						Type:       &coreString,
						TypeIds:    intToStringSlice(cores),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// MemoryDomain -> MemoryDomain
				if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeMemoryDomain {
					sockets, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  false,
						Type:       &memoryDomainString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// MemoryDomain -> Node
				if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  true,
						Type:       &memoryDomainString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Socket -> Socket
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  false,
						Type:       &socketString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Socket -> Node
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Aggregate:  true,
						Type:       &socketString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Node -> Node
				if nativeScope == schema.MetricScopeNode && scope == schema.MetricScopeNode {
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   host.Hostname,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				return nil, nil, fmt.Errorf("METRICDATA/CCMS > TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
			}
		}
	}

	return queries, assignedScope, nil
}

// LoadStats loads only metric statistics (avg/min/max) for a job at node scope.
//
// This is an optimized version of LoadData that fetches only statistics without
// time-series data, reducing bandwidth and memory usage. Always queries at node scope.
//
// Parameters:
//   - job: Job metadata
//   - metrics: List of metric names
//   - ctx: Context (currently unused)
//
// Returns:
//   - Map of metric → hostname → statistics
//   - Error on query building or fetching failure
func LoadStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context,
) (map[string]map[string]schema.MetricStatistics, error) {
	// TODO(#166): Add scope parameter for analysis view accelerator normalization
	queries, _, err := buildQueries(job, metrics, []schema.MetricScope{schema.MetricScopeNode}, 0)
	if err != nil {
		cclog.Errorf("Error while building queries for jobId %d, Metrics %v: %s", job.JobID, metrics, err.Error())
		return nil, err
	}

	req := APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  false,
	}

	resBody, err := FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	stats := make(map[string]map[string]schema.MetricStatistics, len(metrics))
	for i, res := range resBody.Results {
		query := req.Queries[i]
		metric := query.Metric
		data := res[0]
		if data.Error != nil {
			cclog.Errorf("fetching %s for node %s failed: %s", metric, query.Hostname, *data.Error)
			continue
		}

		metricdata, ok := stats[metric]
		if !ok {
			metricdata = make(map[string]schema.MetricStatistics, job.NumNodes)
			stats[metric] = metricdata
		}

		if data.Avg.IsNaN() || data.Min.IsNaN() || data.Max.IsNaN() {
			cclog.Warnf("fetching %s for node %s failed: one of avg/min/max is NaN", metric, query.Hostname)
			continue
		}

		metricdata[query.Hostname] = schema.MetricStatistics{
			Avg: float64(data.Avg),
			Min: float64(data.Min),
			Max: float64(data.Max),
		}
	}

	return stats, nil
}

// LoadScopedStats loads metric statistics for a job with scope-aware grouping.
//
// Similar to LoadStats but supports multiple scopes and returns statistics grouped
// by scope with hardware IDs (e.g., per-core, per-socket statistics).
//
// Parameters:
//   - job: Job metadata
//   - metrics: List of metric names
//   - scopes: Requested metric scopes
//   - ctx: Context (currently unused)
//
// Returns:
//   - ScopedJobStats: Map of metric → scope → []ScopedStats (with hostname and ID)
//   - Error or partial error listing failed queries
func LoadScopedStats(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
) (schema.ScopedJobStats, error) {
	queries, assignedScope, err := buildQueries(job, metrics, scopes, 0)
	if err != nil {
		cclog.Errorf("Error while building queries for jobId %d, Metrics %v, Scopes %v: %s", job.JobID, metrics, scopes, err.Error())
		return nil, err
	}

	req := APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  false,
	}

	resBody, err := FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	var errors []string
	scopedJobStats := make(schema.ScopedJobStats)

	for i, row := range resBody.Results {
		query := req.Queries[i]
		metric := query.Metric
		scope := assignedScope[i]

		if _, ok := scopedJobStats[metric]; !ok {
			scopedJobStats[metric] = make(map[schema.MetricScope][]*schema.ScopedStats)
		}

		if _, ok := scopedJobStats[metric][scope]; !ok {
			scopedJobStats[metric][scope] = make([]*schema.ScopedStats, 0)
		}

		for ndx, res := range row {
			if res.Error != nil {
				/* Build list for "partial errors", if any */
				errors = append(errors, fmt.Sprintf("failed to fetch '%s' from host '%s': %s", query.Metric, query.Hostname, *res.Error))
				continue
			}

			id := (*string)(nil)
			if query.Type != nil {
				id = new(string)
				*id = query.TypeIds[ndx]
			}

			sanitizeStats(&res)

			scopedJobStats[metric][scope] = append(scopedJobStats[metric][scope], &schema.ScopedStats{
				Hostname: query.Hostname,
				Id:       id,
				Data: &schema.MetricStatistics{
					Avg: float64(res.Avg),
					Min: float64(res.Min),
					Max: float64(res.Max),
				},
			})
		}

		// So that one can later check len(scopedJobStats[metric][scope]): Remove from map if empty
		if len(scopedJobStats[metric][scope]) == 0 {
			delete(scopedJobStats[metric], scope)
			if len(scopedJobStats[metric]) == 0 {
				delete(scopedJobStats, metric)
			}
		}
	}

	if len(errors) != 0 {
		/* Returns list for "partial errors" */
		return scopedJobStats, fmt.Errorf("METRICDATA/CCMS > Errors: %s", strings.Join(errors, ", "))
	}
	return scopedJobStats, nil
}

// LoadNodeData loads metric data for specific nodes in a cluster over a time range.
//
// Unlike LoadData which operates on job resources, this function queries arbitrary nodes
// directly. Useful for system monitoring and node status views.
//
// Parameters:
//   - cluster: Cluster name
//   - metrics: List of metric names
//   - nodes: List of node hostnames (nil = all nodes in cluster via ForAllNodes)
//   - scopes: Requested metric scopes (currently unused - always node scope)
//   - from, to: Time range
//   - ctx: Context (currently unused)
//
// Returns:
//   - Map of hostname → metric → []JobMetric
//   - Error or partial error listing failed queries
func LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context,
) (map[string]map[string][]*schema.JobMetric, error) {
	req := APIQueryRequest{
		Cluster:   cluster,
		From:      from.Unix(),
		To:        to.Unix(),
		WithStats: true,
		WithData:  true,
	}

	if nodes == nil {
		req.ForAllNodes = append(req.ForAllNodes, metrics...)
	} else {
		for _, node := range nodes {
			for _, metric := range metrics {
				req.Queries = append(req.Queries, APIQuery{
					Hostname:   node,
					Metric:     metric,
					Resolution: 0, // Default for Node Queries: Will return metric $Timestep Resolution
				})
			}
		}
	}

	resBody, err := FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	var errors []string
	data := make(map[string]map[string][]*schema.JobMetric)
	for i, res := range resBody.Results {
		var query APIQuery
		if resBody.Queries != nil {
			query = resBody.Queries[i]
		} else {
			query = req.Queries[i]
		}

		metric := query.Metric
		qdata := res[0]
		if qdata.Error != nil {
			errors = append(errors, fmt.Sprintf("fetching %s for node %s failed: %s", metric, query.Hostname, *qdata.Error))
		}

		sanitizeStats(&qdata)

		hostdata, ok := data[query.Hostname]
		if !ok {
			hostdata = make(map[string][]*schema.JobMetric)
			data[query.Hostname] = hostdata
		}

		mc := archive.GetMetricConfig(cluster, metric)
		hostdata[metric] = append(hostdata[metric], &schema.JobMetric{
			Unit:     mc.Unit,
			Timestep: mc.Timestep,
			Series: []schema.Series{
				{
					Hostname: query.Hostname,
					Data:     qdata.Data,
					Statistics: schema.MetricStatistics{
						Avg: float64(qdata.Avg),
						Min: float64(qdata.Min),
						Max: float64(qdata.Max),
					},
				},
			},
		})
	}

	if len(errors) != 0 {
		/* Returns list of "partial errors" */
		return data, fmt.Errorf("METRICDATA/CCMS > Errors: %s", strings.Join(errors, ", "))
	}

	return data, nil
}

// LoadNodeListData loads metric data for a list of nodes with full scope transformation support.
//
// This is the most flexible node data loading function, supporting arbitrary scopes and
// resolution. Uses buildNodeQueries for proper scope transformation based on topology.
//
// Parameters:
//   - cluster: Cluster name
//   - subCluster: SubCluster name (empty string to infer from node names)
//   - nodes: List of node hostnames
//   - metrics: List of metric names
//   - scopes: Requested metric scopes
//   - resolution: Data resolution in seconds
//   - from, to: Time range
//   - ctx: Context (currently unused)
//
// Returns:
//   - Map of hostname → JobData (metric → scope → JobMetric)
//   - Error or partial error listing failed queries
func LoadNodeListData(
	cluster, subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	ctx context.Context,
) (map[string]schema.JobData, error) {
	// Note: Order of node data is not guaranteed after this point
	queries, assignedScope, err := buildNodeQueries(cluster, subCluster, nodes, metrics, scopes, int64(resolution))
	if err != nil {
		cclog.Errorf("Error while building node queries for Cluster %s, SubCLuster %s, Metrics %v, Scopes %v: %s", cluster, subCluster, metrics, scopes, err.Error())
		return nil, err
	}

	req := APIQueryRequest{
		Cluster:   cluster,
		Queries:   queries,
		From:      from.Unix(),
		To:        to.Unix(),
		WithStats: true,
		WithData:  true,
	}

	resBody, err := FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	var errors []string
	data := make(map[string]schema.JobData)
	for i, row := range resBody.Results {
		var query APIQuery
		if resBody.Queries != nil {
			query = resBody.Queries[i]
		} else {
			query = req.Queries[i]
		}

		metric := query.Metric
		scope := assignedScope[i]
		mc := archive.GetMetricConfig(cluster, metric)

		res := mc.Timestep
		if len(row) > 0 {
			res = int(row[0].Resolution)
		}

		// Init Nested Map Data Structures If Not Found
		hostData, ok := data[query.Hostname]
		if !ok {
			hostData = make(schema.JobData)
			data[query.Hostname] = hostData
		}

		metricData, ok := hostData[metric]
		if !ok {
			metricData = make(map[schema.MetricScope]*schema.JobMetric)
			data[query.Hostname][metric] = metricData
		}

		scopeData, ok := metricData[scope]
		if !ok {
			scopeData = &schema.JobMetric{
				Unit:     mc.Unit,
				Timestep: res,
				Series:   make([]schema.Series, 0),
			}
			data[query.Hostname][metric][scope] = scopeData
		}

		for ndx, res := range row {
			if res.Error != nil {
				/* Build list for "partial errors", if any */
				errors = append(errors, fmt.Sprintf("failed to fetch '%s' from host '%s': %s", query.Metric, query.Hostname, *res.Error))
				continue
			}

			id := (*string)(nil)
			if query.Type != nil {
				id = new(string)
				*id = query.TypeIds[ndx]
			}

			sanitizeStats(&res)

			scopeData.Series = append(scopeData.Series, schema.Series{
				Hostname: query.Hostname,
				Id:       id,
				Statistics: schema.MetricStatistics{
					Avg: float64(res.Avg),
					Min: float64(res.Min),
					Max: float64(res.Max),
				},
				Data: res.Data,
			})
		}
	}

	if len(errors) != 0 {
		/* Returns list of "partial errors" */
		return data, fmt.Errorf("METRICDATA/CCMS > Errors: %s", strings.Join(errors, ", "))
	}

	return data, nil
}

// buildNodeQueries constructs APIQuery structures for node-based queries with scope transformation.
//
// Similar to buildQueries but operates on node lists rather than job resources.
// Supports dynamic subcluster lookup when subCluster parameter is empty.
//
// Parameters:
//   - cluster: Cluster name
//   - subCluster: SubCluster name (empty = infer from node hostnames)
//   - nodes: List of node hostnames
//   - metrics: List of metric names
//   - scopes: Requested metric scopes
//   - resolution: Data resolution in seconds
//
// Returns:
//   - []APIQuery: List of queries to execute
//   - []schema.MetricScope: Assigned scope for each query
//   - error: Returns error if topology lookup fails or unhandled scope combination
func buildNodeQueries(
	cluster string,
	subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int64,
) ([]APIQuery, []schema.MetricScope, error) {
	if len(nodes) == 0 {
		return nil, nil, fmt.Errorf("METRICDATA/CCMS > no nodes specified for query")
	}

	queries := make([]APIQuery, 0, len(metrics)*len(scopes)*len(nodes))
	assignedScope := []schema.MetricScope{}

	// Get Topol before loop if subCluster given
	var subClusterTopol *schema.SubCluster
	var scterr error
	if subCluster != "" {
		subClusterTopol, scterr = archive.GetSubCluster(cluster, subCluster)
		if scterr != nil {
			cclog.Errorf("could not load cluster %s subCluster %s topology: %s", cluster, subCluster, scterr.Error())
			return nil, nil, scterr
		}
	}

	for _, metric := range metrics {
		mc := archive.GetMetricConfig(cluster, metric)
		if mc == nil {
			cclog.Warnf("metric '%s' is not specified for cluster '%s'", metric, cluster)
			continue
		}

		// Skip if metric is removed for subcluster
		if mc.SubClusters != nil {
			isRemoved := false
			for _, scConfig := range mc.SubClusters {
				if scConfig.Name == subCluster && scConfig.Remove {
					isRemoved = true
					break
				}
			}
			if isRemoved {
				continue
			}
		}

		// Avoid duplicates using map for O(1) lookup
		handledScopes := make(map[schema.MetricScope]bool, 3)

		for _, requestedScope := range scopes {
			nativeScope := mc.Scope

			scope := nativeScope.Max(requestedScope)
			if handledScopes[scope] {
				continue
			}
			handledScopes[scope] = true

			for _, hostname := range nodes {

				// If no subCluster given, get it by node
				if subCluster == "" {
					subClusterName, scnerr := archive.GetSubClusterByNode(cluster, hostname)
					if scnerr != nil {
						return nil, nil, scnerr
					}
					subClusterTopol, scterr = archive.GetSubCluster(cluster, subClusterName)
					if scterr != nil {
						return nil, nil, scterr
					}
				}

				// Always full node hwthread id list, no partial queries expected -> Use "topology.Node" directly where applicable
				// Always full accelerator id list, no partial queries expected -> Use "acceleratorIds" directly where applicable
				topology := subClusterTopol.Topology
				acceleratorIds := topology.GetAcceleratorIDs()

				// Moved check here if metric matches hardware specs
				if nativeScope == schema.MetricScopeAccelerator && len(acceleratorIds) == 0 {
					continue
				}

				// Accelerator -> Accelerator (Use "accelerator" scope if requested scope is lower than node)
				if nativeScope == schema.MetricScopeAccelerator && scope.LT(schema.MetricScopeNode) {
					if scope != schema.MetricScopeAccelerator {
						// Skip all other catched cases
						continue
					}

					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  false,
						Type:       &acceleratorString,
						TypeIds:    acceleratorIds,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, schema.MetricScopeAccelerator)
					continue
				}

				// Accelerator -> Node
				if nativeScope == schema.MetricScopeAccelerator && scope == schema.MetricScopeNode {
					if len(acceleratorIds) == 0 {
						continue
					}

					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  true,
						Type:       &acceleratorString,
						TypeIds:    acceleratorIds,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// HWThread -> HWThread
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  false,
						Type:       &hwthreadString,
						TypeIds:    intToStringSlice(topology.Node),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// HWThread -> Core
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(topology.Node)
					for _, core := range cores {
						queries = append(queries, APIQuery{
							Metric:     metric,
							Hostname:   hostname,
							Aggregate:  true,
							Type:       &hwthreadString,
							TypeIds:    intToStringSlice(topology.Core[core]),
							Resolution: resolution,
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// HWThread -> Socket
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(topology.Node)
					for _, socket := range sockets {
						queries = append(queries, APIQuery{
							Metric:     metric,
							Hostname:   hostname,
							Aggregate:  true,
							Type:       &hwthreadString,
							TypeIds:    intToStringSlice(topology.Socket[socket]),
							Resolution: resolution,
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// HWThread -> Node
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeNode {
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  true,
						Type:       &hwthreadString,
						TypeIds:    intToStringSlice(topology.Node),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Core -> Core
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(topology.Node)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  false,
						Type:       &coreString,
						TypeIds:    intToStringSlice(cores),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Core -> Socket
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromCores(topology.Node)
					for _, socket := range sockets {
						queries = append(queries, APIQuery{
							Metric:     metric,
							Hostname:   hostname,
							Aggregate:  true,
							Type:       &coreString,
							TypeIds:    intToStringSlice(topology.Socket[socket]),
							Resolution: resolution,
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// Core -> Node
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeNode {
					cores, _ := topology.GetCoresFromHWThreads(topology.Node)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  true,
						Type:       &coreString,
						TypeIds:    intToStringSlice(cores),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// MemoryDomain -> MemoryDomain
				if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeMemoryDomain {
					sockets, _ := topology.GetMemoryDomainsFromHWThreads(topology.Node)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  false,
						Type:       &memoryDomainString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// MemoryDomain -> Node
				if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetMemoryDomainsFromHWThreads(topology.Node)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  true,
						Type:       &memoryDomainString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Socket -> Socket
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(topology.Node)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  false,
						Type:       &socketString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Socket -> Node
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetSocketsFromHWThreads(topology.Node)
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Aggregate:  true,
						Type:       &socketString,
						TypeIds:    intToStringSlice(sockets),
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Node -> Node
				if nativeScope == schema.MetricScopeNode && scope == schema.MetricScopeNode {
					queries = append(queries, APIQuery{
						Metric:     metric,
						Hostname:   hostname,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				return nil, nil, fmt.Errorf("METRICDATA/CCMS > TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
			}
		}
	}

	return queries, assignedScope, nil
}

// sanitizeStats converts NaN statistics to zero for JSON compatibility.
//
// schema.Float with NaN values cannot be properly JSON-encoded, so we convert
// NaN to 0. This loses the distinction between "no data" and "zero value",
// but maintains API compatibility.
func sanitizeStats(data *APIMetricData) {
	if data.Avg.IsNaN() {
		data.Avg = schema.Float(0)
	}
	if data.Min.IsNaN() {
		data.Min = schema.Float(0)
	}
	if data.Max.IsNaN() {
		data.Max = schema.Float(0)
	}
}

// intToStringSlice converts a slice of integers to a slice of strings.
// Used to convert hardware thread/core/socket IDs from topology (int) to APIQuery TypeIds (string).
//
// Optimized to reuse a byte buffer for string conversion, reducing allocations.
func intToStringSlice(is []int) []string {
	if len(is) == 0 {
		return nil
	}

	ss := make([]string, len(is))
	buf := make([]byte, 0, 16) // Reusable buffer for integer conversion
	for i, x := range is {
		buf = strconv.AppendInt(buf[:0], int64(x), 10)
		ss[i] = string(buf)
	}
	return ss
}
