// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstoreclient provides a client for querying the cc-metric-store time series database.
//
// The cc-metric-store is a high-performance time series database optimized for HPC metric data.
// This client handles HTTP communication, query construction, scope transformations, and data retrieval
// for job and node metrics across different metric scopes (node, socket, core, hwthread, accelerator).
//
// # Architecture
//
// The package is split into two main components:
//   - Client Operations (cc-metric-store.go): HTTP client, request handling, data loading methods
//   - Query Building (cc-metric-store-queries.go): Query construction and scope transformation logic
//
// # Basic Usage
//
//	store := NewCCMetricStore("http://localhost:8080", "jwt-token")
//
//	// Load job data
//	jobData, err := store.LoadData(job, metrics, scopes, ctx, resolution)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Metric Scopes
//
// The client supports hierarchical metric scopes that map to HPC hardware topology:
//   - MetricScopeAccelerator: GPU/accelerator level metrics
//   - MetricScopeHWThread: Hardware thread (SMT) level metrics
//   - MetricScopeCore: CPU core level metrics
//   - MetricScopeSocket: CPU socket level metrics
//   - MetricScopeMemoryDomain: NUMA domain level metrics
//   - MetricScopeNode: Full node level metrics
//
// The client automatically handles scope transformations, aggregating finer-grained metrics
// to coarser scopes when needed (e.g., aggregating core metrics to socket level).
//
// # Error Handling
//
// The client supports partial errors - if some queries fail, it returns both the successful
// data and an error listing the failed queries. This allows processing partial results
// when some nodes or metrics are temporarily unavailable.
//
// # API Versioning
//
// The client uses cc-metric-store API v2, which includes support for:
//   - Data resampling for bandwidth optimization
//   - Multi-scope queries in a single request
//   - Aggregation across hardware topology levels
package metricstoreclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// CCMetricStore is the HTTP client for communicating with cc-metric-store.
// It manages connection details, authentication, and provides methods for querying metrics.
type CCMetricStore struct {
	client        http.Client // HTTP client with 10-second timeout
	jwt           string      // JWT Bearer token for authentication
	url           string      // Base URL of cc-metric-store instance
	queryEndpoint string      // Full URL to query API endpoint
}

// APIQueryRequest represents a request to the cc-metric-store query API.
// It supports both explicit queries and "for-all-nodes" bulk queries.
type APIQueryRequest struct {
	Cluster     string     `json:"cluster"`       // Target cluster name
	Queries     []APIQuery `json:"queries"`       // Explicit list of metric queries
	ForAllNodes []string   `json:"for-all-nodes"` // Metrics to query for all nodes
	From        int64      `json:"from"`          // Start time (Unix timestamp)
	To          int64      `json:"to"`            // End time (Unix timestamp)
	WithStats   bool       `json:"with-stats"`    // Include min/avg/max statistics
	WithData    bool       `json:"with-data"`     // Include time series data points
}

// APIQuery specifies a single metric query with optional scope filtering.
// Type and TypeIds define the hardware scope (core, socket, accelerator, etc.).
type APIQuery struct {
	Type       *string  `json:"type,omitempty"`        // Scope type (e.g., "core", "socket")
	SubType    *string  `json:"subtype,omitempty"`     // Sub-scope type (reserved for future use)
	Metric     string   `json:"metric"`                // Metric name
	Hostname   string   `json:"host"`                  // Target hostname
	Resolution int      `json:"resolution"`            // Data resolution in seconds (0 = native)
	TypeIds    []string `json:"type-ids,omitempty"`    // IDs for the scope type (e.g., core IDs)
	SubTypeIds []string `json:"subtype-ids,omitempty"` // IDs for sub-scope (reserved)
	Aggregate  bool     `json:"aggreg"`                // Aggregate across TypeIds
}

// APIQueryResponse contains the results from a cc-metric-store query.
// Results align with the Queries slice by index.
type APIQueryResponse struct {
	Queries []APIQuery        `json:"queries,omitempty"` // Echoed queries (for bulk requests)
	Results [][]APIMetricData `json:"results"`           // Result data, indexed by query
}

// APIMetricData represents time series data and statistics for a single metric series.
// Error is set if this particular series failed to load.
type APIMetricData struct {
	Error      *string        `json:"error"`      // Error message if query failed
	Data       []schema.Float `json:"data"`       // Time series data points
	From       int64          `json:"from"`       // Actual start time of data
	To         int64          `json:"to"`         // Actual end time of data
	Resolution int            `json:"resolution"` // Actual resolution of data in seconds
	Avg        schema.Float   `json:"avg"`        // Average value across time range
	Min        schema.Float   `json:"min"`        // Minimum value in time range
	Max        schema.Float   `json:"max"`        // Maximum value in time range
}

// NewCCMetricStore creates and initializes a new CCMetricStore client.
// The url parameter should include the protocol and port (e.g., "http://localhost:8080").
// The token parameter is a JWT used for Bearer authentication; pass empty string if auth is disabled.
func NewCCMetricStore(url string, token string) *CCMetricStore {
	return &CCMetricStore{
		url:           url,
		queryEndpoint: fmt.Sprintf("%s/api/query", url),
		jwt:           token,
		client: http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// doRequest executes an HTTP POST request to the cc-metric-store query API.
// It handles JSON encoding/decoding, authentication, and API versioning.
// The request body is automatically closed to prevent resource leaks.
func (ccms *CCMetricStore) doRequest(
	ctx context.Context,
	body *APIQueryRequest,
) (*APIQueryResponse, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		cclog.Errorf("Error while encoding request body: %s", err.Error())
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ccms.queryEndpoint, buf)
	if err != nil {
		cclog.Errorf("Error while building request body: %s", err.Error())
		return nil, err
	}
	if ccms.jwt != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", ccms.jwt))
	}

	// versioning the cc-metric-store query API.
	// v2 = data with resampling
	// v1 = data without resampling
	q := req.URL.Query()
	q.Add("version", "v2")
	req.URL.RawQuery = q.Encode()

	res, err := ccms.client.Do(req)
	if err != nil {
		cclog.Errorf("Error while performing request: %s", err.Error())
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("'%s': HTTP Status: %s", ccms.queryEndpoint, res.Status)
	}

	var resBody APIQueryResponse
	if err := json.NewDecoder(bufio.NewReader(res.Body)).Decode(&resBody); err != nil {
		cclog.Errorf("Error while decoding result body: %s", err.Error())
		return nil, err
	}

	return &resBody, nil
}

// LoadData retrieves time series data and statistics for the specified job and metrics.
// It queries data for the job's time range and resources, handling scope transformations automatically.
//
// Parameters:
//   - job: Job metadata including cluster, time range, and allocated resources
//   - metrics: List of metric names to retrieve
//   - scopes: Requested metric scopes (node, socket, core, etc.)
//   - ctx: Context for cancellation and timeouts
//   - resolution: Data resolution in seconds (0 for native resolution)
//
// Returns JobData organized as: metric -> scope -> series list.
// Supports partial errors: returns available data even if some queries fail.
func (ccms *CCMetricStore) LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int,
) (schema.JobData, error) {
	queries, assignedScope, err := ccms.buildQueries(job, metrics, scopes, resolution)
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

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		cclog.Errorf("Error while performing request for job %d: %s", job.JobID, err.Error())
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
			res = row[0].Resolution
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

			sanitizeStats(&res.Avg, &res.Min, &res.Max)

			jobMetric.Series = append(jobMetric.Series, schema.Series{
				Hostname: query.Hostname,
				ID:       id,
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

// LoadStats retrieves min/avg/max statistics for job metrics at node scope.
// This is faster than LoadData when only statistical summaries are needed (no time series data).
//
// Returns statistics organized as: metric -> hostname -> statistics.
func (ccms *CCMetricStore) LoadStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context,
) (map[string]map[string]schema.MetricStatistics, error) {
	queries, _, err := ccms.buildQueries(job, metrics, []schema.MetricScope{schema.MetricScopeNode}, 0) // #166 Add scope shere for analysis view accelerator normalization?
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

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		cclog.Errorf("Error while performing request for job %d: %s", job.JobID, err.Error())
		return nil, err
	}

	stats := make(map[string]map[string]schema.MetricStatistics, len(metrics))
	for i, res := range resBody.Results {
		query := req.Queries[i]
		metric := query.Metric
		data := res[0]
		if data.Error != nil {
			cclog.Warnf("fetching %s for node %s failed: %s", metric, query.Hostname, *data.Error)
			continue
		}

		metricdata, ok := stats[metric]
		if !ok {
			metricdata = make(map[string]schema.MetricStatistics, job.NumNodes)
			stats[metric] = metricdata
		}

		if hasNaNStats(data.Avg, data.Min, data.Max) {
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

// LoadScopedStats retrieves statistics for job metrics across multiple scopes.
// Used for the Job-View Statistics Table to display per-scope breakdowns.
//
// Returns statistics organized as: metric -> scope -> list of scoped statistics.
// Each scoped statistic includes hostname, hardware ID (if applicable), and min/avg/max values.
func (ccms *CCMetricStore) LoadScopedStats(
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

	req := APIQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime,
		To:        job.StartTime + int64(job.Duration),
		Queries:   queries,
		WithStats: true,
		WithData:  false,
	}

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		cclog.Errorf("Error while performing request for job %d: %s", job.JobID, err.Error())
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

			sanitizeStats(&res.Avg, &res.Min, &res.Max)

			scopedJobStats[metric][scope] = append(scopedJobStats[metric][scope], &schema.ScopedStats{
				Hostname: query.Hostname,
				ID:       id,
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

// LoadNodeData retrieves current metric data for specified nodes in a cluster.
// Used for the Systems-View Node-Overview to display real-time node status.
//
// If nodes is nil, queries all metrics for all nodes in the cluster (bulk query).
// Returns data organized as: hostname -> metric -> list of JobMetric (with time series and stats).
func (ccms *CCMetricStore) LoadNodeData(
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

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		cclog.Errorf("Error while performing request for cluster %s: %s", cluster, err.Error())
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
			/* Build list for "partial errors", if any */
			errors = append(errors, fmt.Sprintf("fetching %s for node %s failed: %s", metric, query.Hostname, *qdata.Error))
		}

		sanitizeStats(&qdata.Avg, &qdata.Min, &qdata.Max)

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

// LoadNodeListData retrieves paginated node metrics for the Systems-View Node-List.
//
// Supports filtering by subcluster and node name pattern. The nodeFilter performs
// substring matching on hostnames.
//
// Returns:
//   - Node data organized as: hostname -> JobData (metric -> scope -> series)
//   - Total node count (before pagination)
//   - HasNextPage flag indicating if more pages are available
//   - Error (may be partial error with some data returned)
func (ccms *CCMetricStore) LoadNodeListData(
	cluster, subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	ctx context.Context,
) (map[string]schema.JobData, error) {
	queries, assignedScope, err := ccms.buildNodeQueries(cluster, subCluster, nodes, metrics, scopes, resolution)
	if err != nil {
		cclog.Errorf("Error while building node queries for Cluster %s, SubCluster %s, Metrics %v, Scopes %v: %s", cluster, subCluster, metrics, scopes, err.Error())
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

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		cclog.Errorf("Error while performing request for cluster %s: %s", cluster, err.Error())
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
		// qdata := res[0]
		metric := query.Metric
		scope := assignedScope[i]
		mc := archive.GetMetricConfig(cluster, metric)

		res := mc.Timestep
		if len(row) > 0 {
			res = row[0].Resolution
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

			sanitizeStats(&res.Avg, &res.Min, &res.Max)

			scopeData.Series = append(scopeData.Series, schema.Series{
				Hostname: query.Hostname,
				ID:       id,
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

// HealthCheck queries the external cc-metric-store's health check endpoint.
// It sends a HealthCheckReq as the request body to /api/healthcheck and
// returns the per-node health check results.
func (ccms *CCMetricStore) HealthCheck(cluster string,
	nodes []string, metrics []string,
) (map[string]metricstore.HealthCheckResult, error) {
	req := metricstore.HealthCheckReq{
		Cluster:     cluster,
		Nodes:       nodes,
		MetricNames: metrics,
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(req); err != nil {
		cclog.Errorf("Error while encoding health check request body: %s", err.Error())
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/healthcheck", ccms.url)
	httpReq, err := http.NewRequest(http.MethodGet, endpoint, buf)
	if err != nil {
		cclog.Errorf("Error while building health check request: %s", err.Error())
		return nil, err
	}
	if ccms.jwt != "" {
		httpReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", ccms.jwt))
	}

	res, err := ccms.client.Do(httpReq)
	if err != nil {
		cclog.Errorf("Error while performing health check request: %s", err.Error())
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("'%s': HTTP Status: %s", endpoint, res.Status)
	}

	var results map[string]metricstore.HealthCheckResult
	if err := json.NewDecoder(bufio.NewReader(res.Body)).Decode(&results); err != nil {
		cclog.Errorf("Error while decoding health check response: %s", err.Error())
		return nil, err
	}

	return results, nil
}

// sanitizeStats replaces NaN values in statistics with 0 to enable JSON marshaling.
// Regular float64 values cannot be JSONed when NaN.
func sanitizeStats(avg, min, max *schema.Float) {
	if avg.IsNaN() || min.IsNaN() || max.IsNaN() {
		*avg = schema.Float(0)
		*min = schema.Float(0)
		*max = schema.Float(0)
	}
}

// hasNaNStats returns true if any of the statistics contain NaN values.
func hasNaNStats(avg, min, max schema.Float) bool {
	return avg.IsNaN() || min.IsNaN() || max.IsNaN()
}
