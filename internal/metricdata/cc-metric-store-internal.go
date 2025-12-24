// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/memorystore"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Bloat Code
type CCMetricStoreConfigInternal struct {
	Kind  string `json:"kind"`
	Url   string `json:"url"`
	Token string `json:"token"`

	// If metrics are known to this MetricDataRepository under a different
	// name than in the `metricConfig` section of the 'cluster.json',
	// provide this optional mapping of local to remote name for this metric.
	Renamings map[string]string `json:"metricRenamings"`
}

// Bloat Code
type CCMetricStoreInternal struct{}

// Bloat Code
func (ccms *CCMetricStoreInternal) Init(rawConfig json.RawMessage) error {
	return nil
}

func (ccms *CCMetricStoreInternal) LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int,
) (schema.JobData, error) {
	queries, assignedScope, err := ccms.buildQueries(job, metrics, scopes, int64(resolution))
	if err != nil {
		cclog.Errorf("Error while building queries for jobId %d, Metrics %v, Scopes %v: %s", job.JobID, metrics, scopes, err.Error())
		return nil, err
	}

	req := memorystore.APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  true,
	}

	resBody, err := memorystore.FetchData(req)
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

			if res.Avg.IsNaN() || res.Min.IsNaN() || res.Max.IsNaN() {
				// "schema.Float()" because regular float64 can not be JSONed when NaN.
				res.Avg = schema.Float(0)
				res.Min = schema.Float(0)
				res.Max = schema.Float(0)
			}

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

var (
	hwthreadString     = string(schema.MetricScopeHWThread)
	coreString         = string(schema.MetricScopeCore)
	memoryDomainString = string(schema.MetricScopeMemoryDomain)
	socketString       = string(schema.MetricScopeSocket)
	acceleratorString  = string(schema.MetricScopeAccelerator)
)

func (ccms *CCMetricStoreInternal) buildQueries(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int64,
) ([]memorystore.APIQuery, []schema.MetricScope, error) {
	queries := make([]memorystore.APIQuery, 0, len(metrics)*len(scopes)*len(job.Resources))
	assignedScope := []schema.MetricScope{}

	subcluster, scerr := archive.GetSubCluster(job.Cluster, job.SubCluster)
	if scerr != nil {
		return nil, nil, scerr
	}
	topology := subcluster.Topology

	for _, metric := range metrics {
		mc := archive.GetMetricConfig(job.Cluster, metric)
		if mc == nil {
			// return nil, fmt.Errorf("METRICDATA/CCMS > metric '%s' is not specified for cluster '%s'", metric, job.Cluster)
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

		// Avoid duplicates...
		handledScopes := make([]schema.MetricScope, 0, 3)

	scopesLoop:
		for _, requestedScope := range scopes {
			nativeScope := mc.Scope
			if nativeScope == schema.MetricScopeAccelerator && job.NumAcc == 0 {
				continue
			}

			scope := nativeScope.Max(requestedScope)
			for _, s := range handledScopes {
				if scope == s {
					continue scopesLoop
				}
			}
			handledScopes = append(handledScopes, scope)

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

					queries = append(queries, memorystore.APIQuery{
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

					queries = append(queries, memorystore.APIQuery{
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

				// HWThread -> HWThead
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
					queries = append(queries, memorystore.APIQuery{
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
						queries = append(queries, memorystore.APIQuery{
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
						queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
						queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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

				// MemoryDoman -> Node
				if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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

func (ccms *CCMetricStoreInternal) LoadStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context,
) (map[string]map[string]schema.MetricStatistics, error) {
	queries, _, err := ccms.buildQueries(job, metrics, []schema.MetricScope{schema.MetricScopeNode}, 0) // #166 Add scope shere for analysis view accelerator normalization?
	if err != nil {
		cclog.Errorf("Error while building queries for jobId %d, Metrics %v: %s", job.JobID, metrics, err.Error())
		return nil, err
	}

	req := memorystore.APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  false,
	}

	resBody, err := memorystore.FetchData(req)
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

// Used for Job-View Statistics Table
func (ccms *CCMetricStoreInternal) LoadScopedStats(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
) (schema.ScopedJobStats, error) {
	queries, assignedScope, err := ccms.buildQueries(job, metrics, scopes, 0)
	if err != nil {
		cclog.Errorf("Error while building queries for jobId %d, Metrics %v, Scopes %v: %s", job.JobID, metrics, scopes, err.Error())
		return nil, err
	}

	req := memorystore.APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  false,
	}

	resBody, err := memorystore.FetchData(req)
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

			if res.Avg.IsNaN() || res.Min.IsNaN() || res.Max.IsNaN() {
				// "schema.Float()" because regular float64 can not be JSONed when NaN.
				res.Avg = schema.Float(0)
				res.Min = schema.Float(0)
				res.Max = schema.Float(0)
			}

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

// Used for Systems-View Node-Overview
func (ccms *CCMetricStoreInternal) LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context,
) (map[string]map[string][]*schema.JobMetric, error) {
	req := memorystore.APIQueryRequest{
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
				req.Queries = append(req.Queries, memorystore.APIQuery{
					Hostname:   node,
					Metric:     metric,
					Resolution: 0, // Default for Node Queries: Will return metric $Timestep Resolution
				})
			}
		}
	}

	resBody, err := memorystore.FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	var errors []string
	data := make(map[string]map[string][]*schema.JobMetric)
	for i, res := range resBody.Results {
		var query memorystore.APIQuery
		if resBody.Queries != nil {
			query = resBody.Queries[i]
		} else {
			query = req.Queries[i]
		}

		metric := query.Metric
		qdata := res[0]
		if qdata.Error != nil {
			/* Build list for "partial errors", if any */
			errors = append(errors, fmt.Sprintf("fetching %s for node %s failed: %s", metric, query.Hostname, *qdata.Error))
		}

		if qdata.Avg.IsNaN() || qdata.Min.IsNaN() || qdata.Max.IsNaN() {
			// return nil, fmt.Errorf("METRICDATA/CCMS > fetching %s for node %s failed: %s", metric, query.Hostname, "avg/min/max is NaN")
			qdata.Avg, qdata.Min, qdata.Max = 0., 0., 0.
		}

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

// Used for Systems-View Node-List
func (ccms *CCMetricStoreInternal) LoadNodeListData(
	cluster, subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	ctx context.Context,
) (map[string]schema.JobData, error) {

	// Note: Order of node data is not guaranteed after this point
	queries, assignedScope, err := ccms.buildNodeQueries(cluster, subCluster, nodes, metrics, scopes, int64(resolution))
	if err != nil {
		cclog.Errorf("Error while building node queries for Cluster %s, SubCLuster %s, Metrics %v, Scopes %v: %s", cluster, subCluster, metrics, scopes, err.Error())
		return nil, err
	}

	req := memorystore.APIQueryRequest{
		Cluster:   cluster,
		Queries:   queries,
		From:      from.Unix(),
		To:        to.Unix(),
		WithStats: true,
		WithData:  true,
	}

	resBody, err := memorystore.FetchData(req)
	if err != nil {
		cclog.Errorf("Error while fetching data : %s", err.Error())
		return nil, err
	}

	var errors []string
	data := make(map[string]schema.JobData)
	for i, row := range resBody.Results {
		var query memorystore.APIQuery
		if resBody.Queries != nil {
			query = resBody.Queries[i]
		} else {
			query = req.Queries[i]
		}
		// qdata := res[0]
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

			if res.Avg.IsNaN() || res.Min.IsNaN() || res.Max.IsNaN() {
				// "schema.Float()" because regular float64 can not be JSONed when NaN.
				res.Avg = schema.Float(0)
				res.Min = schema.Float(0)
				res.Max = schema.Float(0)
			}

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

func (ccms *CCMetricStoreInternal) buildNodeQueries(
	cluster string,
	subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int64,
) ([]memorystore.APIQuery, []schema.MetricScope, error) {
	queries := make([]memorystore.APIQuery, 0, len(metrics)*len(scopes)*len(nodes))
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
		metric := metric
		mc := archive.GetMetricConfig(cluster, metric)
		if mc == nil {
			// return nil, fmt.Errorf("METRICDATA/CCMS > metric '%s' is not specified for cluster '%s'", metric, cluster)
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

		// Avoid duplicates...
		handledScopes := make([]schema.MetricScope, 0, 3)

	scopesLoop:
		for _, requestedScope := range scopes {
			nativeScope := mc.Scope

			scope := nativeScope.Max(requestedScope)
			for _, s := range handledScopes {
				if scope == s {
					continue scopesLoop
				}
			}
			handledScopes = append(handledScopes, scope)

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
					continue scopesLoop
				}

				// Accelerator -> Accelerator (Use "accelerator" scope if requested scope is lower than node)
				if nativeScope == schema.MetricScopeAccelerator && scope.LT(schema.MetricScopeNode) {
					if scope != schema.MetricScopeAccelerator {
						// Skip all other catched cases
						continue
					}

					queries = append(queries, memorystore.APIQuery{
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

					queries = append(queries, memorystore.APIQuery{
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

				// HWThread -> HWThead
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
					queries = append(queries, memorystore.APIQuery{
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
						queries = append(queries, memorystore.APIQuery{
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
						queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
						queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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

				// MemoryDoman -> Node
				if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetMemoryDomainsFromHWThreads(topology.Node)
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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
					queries = append(queries, memorystore.APIQuery{
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

func intToStringSlice(is []int) []string {
	ss := make([]string, len(is))
	for i, x := range is {
		ss[i] = strconv.Itoa(x)
	}
	return ss
}
