package metricdata

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

type CCMetricStore struct {
	jwt    string
	url    string
	client http.Client
}

type ApiRequestBody struct {
	Metrics   []string   `json:"metrics"`
	Selectors [][]string `json:"selectors"`
}

type ApiQuery struct {
	Metric     string   `json:"metric"`
	Hostname   string   `json:"hostname"`
	Type       *string  `json:"type,omitempty"`
	TypeIds    []string `json:"type-ids,omitempty"`
	SubType    *string  `json:"subtype,omitempty"`
	SubTypeIds []string `json:"subtype-ids,omitempty"`
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

type ApiStatsData struct {
	Error   *string      `json:"error"`
	From    int64        `json:"from"`
	To      int64        `json:"to"`
	Samples int          `json:"samples"`
	Avg     schema.Float `json:"avg"`
	Min     schema.Float `json:"min"`
	Max     schema.Float `json:"max"`
}

func (ccms *CCMetricStore) Init(url, token string) error {
	ccms.url = url
	ccms.jwt = token
	ccms.client = http.Client{
		Timeout: 5 * time.Second,
	}
	return nil
}

func (ccms *CCMetricStore) doRequest(job *schema.Job, suffix string, metrics []string, ctx context.Context) (*http.Response, error) {
	from, to := job.StartTime.Unix(), job.StartTime.Add(time.Duration(job.Duration)*time.Second).Unix()
	reqBody := ApiRequestBody{}
	reqBody.Metrics = metrics
	for _, node := range job.Resources {
		if node.Accelerators != nil || node.HWThreads != nil {
			// TODO/FIXME:
			return nil, errors.New("todo: cc-metric-store resources: Accelerator/HWThreads")
		}

		reqBody.Selectors = append(reqBody.Selectors, []string{job.Cluster, node.Hostname})
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/%d/%d/%s", ccms.url, from, to, suffix), bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, err
	}
	if ccms.jwt != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", ccms.jwt))
	}
	return ccms.client.Do(req)
}

func (ccms *CCMetricStore) LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
	type ApiQueryRequest struct {
		Cluster string     `json:"cluster"`
		From    int64      `json:"from"`
		To      int64      `json:"to"`
		Queries []ApiQuery `json:"queries"`
	}

	type ApiQueryResponse struct {
		ApiMetricData
		Query *ApiQuery `json:"query"`
	}

	queries, assignedScope, err := ccms.buildQueries(job, metrics, scopes)
	if err != nil {
		return nil, err
	}

	reqBody := ApiQueryRequest{
		Cluster: job.Cluster,
		From:    job.StartTime.Unix(),
		To:      job.StartTime.Add(time.Duration(job.Duration) * time.Second).Unix(),
		Queries: queries,
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(reqBody); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ccms.url+"/api/query", buf)
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
		return nil, fmt.Errorf("cc-metric-store replied with: %s", res.Status)
	}

	var resBody []ApiQueryResponse
	if err := json.NewDecoder(bufio.NewReader(res.Body)).Decode(&resBody); err != nil {
		return nil, err
	}

	// log.Printf("response: %#v", resBody)

	var jobData schema.JobData = make(schema.JobData)
	for i, res := range resBody {
		metric := res.Query.Metric
		if _, ok := jobData[metric]; !ok {
			jobData[metric] = make(map[schema.MetricScope]*schema.JobMetric)
		}

		if res.Error != nil {
			return nil, fmt.Errorf("cc-metric-store error while fetching %s: %s", metric, *res.Error)
		}

		scope := assignedScope[i]
		mc := config.GetMetricConfig(job.Cluster, metric)
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

		id := (*int)(nil)
		if res.Query.Type != nil {
			id = new(int)
			*id, _ = strconv.Atoi(res.Query.TypeIds[0])
		}

		if res.Avg.IsNaN() || res.Min.IsNaN() || res.Max.IsNaN() {
			// TODO: use schema.Float instead of float64?
			// This is done because regular float64 can not be JSONed when NaN.
			res.Avg = schema.Float(0)
			res.Min = schema.Float(0)
			res.Max = schema.Float(0)
		}

		jobMetric.Series = append(jobMetric.Series, schema.Series{
			Hostname: res.Query.Hostname,
			Id:       id,
			Statistics: &schema.MetricStatistics{
				Avg: float64(res.Avg),
				Min: float64(res.Min),
				Max: float64(res.Max),
			},
			Data: res.Data,
		})
	}

	return jobData, nil
}

var (
	hwthreadString = string("cpu") // TODO/FIXME: inconsistency between cc-metric-collector and ClusterCockpit
	// coreString        = string(schema.MetricScopeCore)
	socketString      = string(schema.MetricScopeSocket)
	acceleratorString = string(schema.MetricScopeAccelerator)
)

func (ccms *CCMetricStore) buildQueries(job *schema.Job, metrics []string, scopes []schema.MetricScope) ([]ApiQuery, []schema.MetricScope, error) {
	queries := make([]ApiQuery, 0, len(metrics)*len(scopes)*len(job.Resources))
	topology := config.GetPartition(job.Cluster, job.Partition).Topology
	assignedScope := []schema.MetricScope{}

	for _, metric := range metrics {
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
					for _, accel := range host.Accelerators {
						queries = append(queries, ApiQuery{
							Metric:   metric,
							Hostname: host.Hostname,
							Type:     &acceleratorString,
							TypeIds:  []string{strconv.Itoa(accel)},
						})
						assignedScope = append(assignedScope, schema.MetricScopeAccelerator)
					}
					continue
				}

				// Accelerator -> Node
				if nativeScope == schema.MetricScopeAccelerator && scope == schema.MetricScopeNode {
					if len(host.Accelerators) == 0 {
						continue
					}

					queries = append(queries, ApiQuery{
						Metric:   metric,
						Hostname: host.Hostname,
						Type:     &acceleratorString,
						TypeIds:  toStringSlice(host.Accelerators),
					})
					assignedScope = append(assignedScope, schema.MetricScopeNode)
					continue
				}

				// HWThread -> HWThead
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
					for _, hwthread := range hwthreads {
						queries = append(queries, ApiQuery{
							Metric:   metric,
							Hostname: host.Hostname,
							Type:     &hwthreadString,
							TypeIds:  []string{strconv.Itoa(hwthread)},
						})
						assignedScope = append(assignedScope, schema.MetricScopeHWThread)
					}
					continue
				}

				// HWThread -> Core
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeCore {
					cores, _ := topology.GetCoresFromHWThreads(hwthreads)
					for _, core := range cores {
						queries = append(queries, ApiQuery{
							Metric:   metric,
							Hostname: host.Hostname,
							Type:     &hwthreadString,
							TypeIds:  toStringSlice(topology.Core[core]),
						})
						assignedScope = append(assignedScope, schema.MetricScopeCore)
					}
					continue
				}

				// HWThread -> Socket
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					for _, socket := range sockets {
						queries = append(queries, ApiQuery{
							Metric:   metric,
							Hostname: host.Hostname,
							Type:     &hwthreadString,
							TypeIds:  toStringSlice(topology.Socket[socket]),
						})
						assignedScope = append(assignedScope, schema.MetricScopeSocket)
					}
					continue
				}

				// HWThread -> Node
				if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeNode {
					queries = append(queries, ApiQuery{
						Metric:   metric,
						Hostname: host.Hostname,
						Type:     &hwthreadString,
						TypeIds:  toStringSlice(hwthreads),
					})
					assignedScope = append(assignedScope, schema.MetricScopeNode)
					continue
				}

				// Socket -> Socket
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeSocket {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					for _, socket := range sockets {
						queries = append(queries, ApiQuery{
							Metric:   metric,
							Hostname: host.Hostname,
							Type:     &acceleratorString,
							TypeIds:  []string{strconv.Itoa(socket)},
						})
						assignedScope = append(assignedScope, schema.MetricScopeSocket)
					}
					continue
				}

				// Socket -> Node
				if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeNode {
					sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
					queries = append(queries, ApiQuery{
						Metric:   metric,
						Hostname: host.Hostname,
						Type:     &socketString,
						TypeIds:  toStringSlice(sockets),
					})
					assignedScope = append(assignedScope, schema.MetricScopeNode)
					continue
				}

				// Node -> Node
				if nativeScope == schema.MetricScopeNode && scope == schema.MetricScopeNode {
					queries = append(queries, ApiQuery{
						Metric:   metric,
						Hostname: host.Hostname,
					})
					assignedScope = append(assignedScope, schema.MetricScopeNode)
					continue
				}

				return nil, nil, fmt.Errorf("TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
			}
		}
	}

	return queries, assignedScope, nil
}

func toStringSlice(s []int) []string {
	ret := make([]string, len(s))
	for i, val := range s {
		ret[i] = strconv.Itoa(val)
	}
	return ret
}

func (ccms *CCMetricStore) LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {
	res, err := ccms.doRequest(job, "stats", metrics, ctx)
	if err != nil {
		return nil, err
	}

	resdata := make([]map[string]ApiStatsData, 0, len(job.Resources))
	if err := json.NewDecoder(res.Body).Decode(&resdata); err != nil {
		return nil, err
	}

	stats := map[string]map[string]schema.MetricStatistics{}
	for _, metric := range metrics {
		nodestats := map[string]schema.MetricStatistics{}
		for i, node := range job.Resources {
			if node.Accelerators != nil || node.HWThreads != nil {
				// TODO/FIXME:
				return nil, errors.New("todo: cc-metric-store resources: Accelerator/HWThreads")
			}

			data := resdata[i][metric]
			if data.Error != nil {
				return nil, errors.New(*data.Error)
			}

			if data.Samples == 0 {
				return nil, fmt.Errorf("no data for node '%s' and metric '%s'", node.Hostname, metric)
			}

			nodestats[node.Hostname] = schema.MetricStatistics{
				Avg: float64(data.Avg),
				Min: float64(data.Min),
				Max: float64(data.Max),
			}
		}

		stats[metric] = nodestats
	}

	return stats, nil
}

func (ccms *CCMetricStore) LoadNodeData(clusterId string, metrics, nodes []string, from, to int64, ctx context.Context) (map[string]map[string][]schema.Float, error) {
	reqBody := ApiRequestBody{}
	reqBody.Metrics = metrics
	for _, node := range nodes {
		reqBody.Selectors = append(reqBody.Selectors, []string{clusterId, node})
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	var req *http.Request
	if nodes == nil {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/%s/%d/%d/all-nodes", ccms.url, clusterId, from, to), bytes.NewReader(reqBodyBytes))
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/%d/%d/timeseries", ccms.url, from, to), bytes.NewReader(reqBodyBytes))
	}
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

	data := map[string]map[string][]schema.Float{}
	if nodes == nil {
		resdata := map[string]map[string]ApiMetricData{}
		if err := json.NewDecoder(res.Body).Decode(&resdata); err != nil {
			return nil, err
		}

		for node, metrics := range resdata {
			nodedata := map[string][]schema.Float{}
			for metric, data := range metrics {
				if data.Error != nil {
					return nil, errors.New(*data.Error)
				}

				nodedata[metric] = data.Data
			}
			data[node] = nodedata
		}
	} else {
		resdata := make([]map[string]ApiMetricData, 0, len(nodes))
		if err := json.NewDecoder(res.Body).Decode(&resdata); err != nil {
			return nil, err
		}

		for i, node := range nodes {
			metricsData := map[string][]schema.Float{}
			for metric, data := range resdata[i] {
				if data.Error != nil {
					return nil, errors.New(*data.Error)
				}

				metricsData[metric] = data.Data
			}

			data[node] = metricsData
		}
	}

	return data, nil
}
