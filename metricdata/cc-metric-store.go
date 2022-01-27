package metricdata

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/schema"
)

type CCMetricStore struct {
	jwt           string
	url           string
	queryEndpoint string
	client        http.Client
	here2there    map[string]string
	there2here    map[string]string
}

type ApiQueryRequest struct {
	Cluster     string     `json:"cluster"`
	From        int64      `json:"from"`
	To          int64      `json:"to"`
	WithStats   bool       `json:"with-stats"`
	WithData    bool       `json:"with-data"`
	Queries     []ApiQuery `json:"queries"`
	ForAllNodes []string   `json:"for-all-nodes"`
}

type ApiQuery struct {
	Metric     string  `json:"metric"`
	Hostname   string  `json:"host"`
	Aggregate  bool    `json:"aggreg"`
	Type       *string `json:"type,omitempty"`
	TypeIds    []int   `json:"type-ids,omitempty"`
	SubType    *string `json:"subtype,omitempty"`
	SubTypeIds []int   `json:"subtype-ids,omitempty"`
}

type ApiMetricData struct {
	Error *string        `json:"error"`
	From  int64          `json:"from"`
	To    int64          `json:"to"`
	Data  []schema.Float `json:"data"`
	Avg   schema.Float   `json:"avg"`
	Min   schema.Float   `json:"min"`
	Max   schema.Float   `json:"max"`
}

func (ccms *CCMetricStore) Init(url, token string, renamings map[string]string) error {
	ccms.url = url
	ccms.queryEndpoint = fmt.Sprintf("%s/api/query", url)
	ccms.jwt = token
	ccms.client = http.Client{
		Timeout: 5 * time.Second,
	}

	if renamings != nil {
		ccms.here2there = renamings
		ccms.there2here = make(map[string]string, len(renamings))
		for k, v := range ccms.here2there {
			ccms.there2here[v] = k
		}
	} else {
		ccms.here2there = make(map[string]string)
		ccms.there2here = make(map[string]string)
	}

	return nil
}

func (ccms *CCMetricStore) toRemoteName(metric string) string {
	if renamed, ok := ccms.here2there[metric]; ok {
		return renamed
	}

	return metric
}

func (ccms *CCMetricStore) toLocalName(metric string) string {
	if renamed, ok := ccms.there2here[metric]; ok {
		return renamed
	}

	return metric
}

func (ccms *CCMetricStore) doRequest(ctx context.Context, body *ApiQueryRequest) ([][]ApiMetricData, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ccms.queryEndpoint, buf)
	if err != nil {
		return nil, err
	}
	if ccms.jwt != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", ccms.jwt))
	}

	res, err := ccms.client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("'%s': HTTP Status: %s", ccms.queryEndpoint, res.Status)
	}

	var resBody [][]ApiMetricData
	if err := json.NewDecoder(bufio.NewReader(res.Body)).Decode(&resBody); err != nil {
		return nil, err
	}

	return resBody, nil
}

func (ccms *CCMetricStore) LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
	queries, assignedScope, err := ccms.buildQueries(job, metrics, scopes)
	if err != nil {
		return nil, err
	}

	req := ApiQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime.Unix(),
		To:        job.StartTime.Add(time.Duration(job.Duration) * time.Second).Unix(),
		Queries:   queries,
		WithStats: true,
		WithData:  true,
	}

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		return nil, err
	}

	var jobData schema.JobData = make(schema.JobData)
	for i, row := range resBody {
		query := req.Queries[i]
		metric := ccms.toLocalName(query.Metric)
		scope := assignedScope[i]
		mc := config.GetMetricConfig(job.Cluster, metric)
		if _, ok := jobData[metric]; !ok {
			jobData[metric] = make(map[schema.MetricScope]*schema.JobMetric)
		}

		jobMetric, ok := jobData[metric][scope]
		if !ok {
			jobMetric = &schema.JobMetric{
				Unit:     mc.Unit,
				Scope:    scope,
				Timestep: mc.Timestep,
				Series:   make([]schema.Series, 0),
			}
			jobData[metric][scope] = jobMetric
		}

		for _, res := range row {
			if res.Error != nil {
				return nil, fmt.Errorf("cc-metric-store error while fetching %s: %s", metric, *res.Error)
			}

			id := (*int)(nil)
			if query.Type != nil {
				id = new(int)
				*id = query.TypeIds[0]
			}

			if res.Avg.IsNaN() || res.Min.IsNaN() || res.Max.IsNaN() {
				// TODO: use schema.Float instead of float64?
				// This is done because regular float64 can not be JSONed when NaN.
				res.Avg = schema.Float(0)
				res.Min = schema.Float(0)
				res.Max = schema.Float(0)
			}

			jobMetric.Series = append(jobMetric.Series, schema.Series{
				Hostname: query.Hostname,
				Id:       id,
				Statistics: &schema.MetricStatistics{
					Avg: float64(res.Avg),
					Min: float64(res.Min),
					Max: float64(res.Max),
				},
				Data: res.Data,
			})
		}
	}

	return jobData, nil
}

var (
	hwthreadString    = string("cpu") // TODO/FIXME: inconsistency between cc-metric-collector and ClusterCockpit
	coreString        = string(schema.MetricScopeCore)
	socketString      = string(schema.MetricScopeSocket)
	acceleratorString = string(schema.MetricScopeAccelerator)
)

func (ccms *CCMetricStore) buildQueries(job *schema.Job, metrics []string, scopes []schema.MetricScope) ([]ApiQuery, []schema.MetricScope, error) {
	queries := make([]ApiQuery, 0, len(metrics)*len(scopes)*len(job.Resources))
	topology := config.GetPartition(job.Cluster, job.Partition).Topology
	assignedScope := []schema.MetricScope{}

	for _, metric := range metrics {
		remoteName := ccms.toRemoteName(metric)
		mc := config.GetMetricConfig(job.Cluster, metric)
		if mc == nil {
			// return nil, fmt.Errorf("metric '%s' is not specified for cluster '%s'", metric, job.Cluster)
			// log.Printf("metric '%s' is not specified for cluster '%s'", metric, job.Cluster)
			continue
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

			for _, host := range job.Resources {
				hwthreads := host.HWThreads
				if hwthreads == nil {
					hwthreads = topology.Node
				}

				// Accelerator -> Accelerator (Use "accelerator" scope if requested scope is lower than node)
				if nativeScope == schema.MetricScopeAccelerator && scope.LT(schema.MetricScopeNode) {
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: false,
						Type:      &acceleratorString,
						TypeIds:   host.Accelerators,
					})
					assignedScope = append(assignedScope, schema.MetricScopeAccelerator)
					continue
				}

				// Accelerator -> Node
				if nativeScope == schema.MetricScopeAccelerator && scope == schema.MetricScopeNode {
					if len(host.Accelerators) == 0 {
						continue
					}

					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: true,
						Type:      &acceleratorString,
						TypeIds:   host.Accelerators,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// HWThread -> HWThead
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: false,
						Type:      &hwthreadString,
						TypeIds:   hwthreads,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// HWThread -> Core
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					for _, core := range cores {
						queries = append(queries, ApiQuery{
							Metric:    remoteName,
							Hostname:  host.Hostname,
							Aggregate: true,
							Type:      &hwthreadString,
							TypeIds:   topology.Core[core],
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// HWThread -> Socket
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					for _, socket := range sockets {
						queries = append(queries, ApiQuery{
							Metric:    remoteName,
							Hostname:  host.Hostname,
							Aggregate: true,
							Type:      &hwthreadString,
							TypeIds:   topology.Socket[socket],
						})
						assignedScope = append(assignedScope, scope)
					}
					continue
				}

				// HWThread -> Node
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeNode {
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: true,
						Type:      &hwthreadString,
						TypeIds:   hwthreads,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Core -> Core
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: false,
						Type:      &coreString,
						TypeIds:   cores,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Core -> Node
				if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeNode {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: true,
						Type:      &coreString,
						TypeIds:   cores,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Socket -> Socket
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: false,
						Type:      &socketString,
						TypeIds:   sockets,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Socket -> Node
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					queries = append(queries, ApiQuery{
						Metric:    remoteName,
						Hostname:  host.Hostname,
						Aggregate: true,
						Type:      &socketString,
						TypeIds:   sockets,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				// Node -> Node
				if nativeScope == schema.MetricScopeNode && scope == schema.MetricScopeNode {
					queries = append(queries, ApiQuery{
						Metric:   remoteName,
						Hostname: host.Hostname,
					})
					assignedScope = append(assignedScope, scope)
					continue
				}

				return nil, nil, fmt.Errorf("TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
			}
		}
	}

	return queries, assignedScope, nil
}

func (ccms *CCMetricStore) LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {
	queries, _, err := ccms.buildQueries(job, metrics, []schema.MetricScope{schema.MetricScopeNode})
	if err != nil {
		return nil, err
	}

	req := ApiQueryRequest{
		Cluster:   job.Cluster,
		From:      job.StartTime.Unix(),
		To:        job.StartTime.Add(time.Duration(job.Duration) * time.Second).Unix(),
		Queries:   queries,
		WithStats: true,
		WithData:  false,
	}

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]map[string]schema.MetricStatistics, len(metrics))
	for i, res := range resBody {
		query := req.Queries[i]
		metric := ccms.toLocalName(query.Metric)
		data := res[0]
		if data.Error != nil {
			return nil, fmt.Errorf("fetching %s for node %s failed: %s", metric, query.Hostname, *data.Error)
		}

		metricdata, ok := stats[metric]
		if !ok {
			metricdata = make(map[string]schema.MetricStatistics, job.NumNodes)
			stats[metric] = metricdata
		}

		if data.Avg.IsNaN() || data.Min.IsNaN() || data.Max.IsNaN() {
			return nil, fmt.Errorf("fetching %s for node %s failed: %s", metric, query.Hostname, "avg/min/max is NaN")
		}

		metricdata[query.Hostname] = schema.MetricStatistics{
			Avg: float64(data.Avg),
			Min: float64(data.Min),
			Max: float64(data.Max),
		}
	}

	return stats, nil
}

func (ccms *CCMetricStore) LoadNodeData(clusterId string, metrics, nodes []string, from, to int64, ctx context.Context) (map[string]map[string][]schema.Float, error) {
	req := ApiQueryRequest{
		Cluster:   clusterId,
		From:      from,
		To:        to,
		WithStats: false,
		WithData:  true,
	}

	if nodes == nil {
		req.ForAllNodes = metrics
	} else {
		for _, node := range nodes {
			for _, metric := range metrics {
				req.Queries = append(req.Queries, ApiQuery{
					Hostname: node,
					Metric:   ccms.toRemoteName(metric),
				})
			}
		}
	}

	resBody, err := ccms.doRequest(ctx, &req)
	if err != nil {
		return nil, err
	}

	data := make(map[string]map[string][]schema.Float)
	for i, res := range resBody {
		query := req.Queries[i]
		qdata := res[0]
		if qdata.Error != nil {
			return nil, fmt.Errorf("fetching %s for node %s failed: %s", query.Metric, query.Hostname, *qdata.Error)
		}

		nodedata, ok := data[query.Hostname]
		if !ok {
			nodedata = make(map[string][]schema.Float)
			data[query.Hostname] = nodedata
		}

		nodedata[ccms.toLocalName(query.Metric)] = qdata.Data
	}

	return data, nil
}
