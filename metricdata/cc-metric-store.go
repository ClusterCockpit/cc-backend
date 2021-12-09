package metricdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
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

type ApiMetricData struct {
	Error *string        `json:"error"`
	From  int64          `json:"from"`
	To    int64          `json:"to"`
	Data  []schema.Float `json:"data"`
	Avg   *float64       `json:"avg"`
	Min   *float64       `json:"min"`
	Max   *float64       `json:"max"`
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

func (ccms *CCMetricStore) Init(url string) error {
	ccms.url = url // os.Getenv("CCMETRICSTORE_URL")
	ccms.jwt = os.Getenv("CCMETRICSTORE_JWT")
	if ccms.jwt == "" {
		log.Println("warning: environment variable 'CCMETRICSTORE_JWT' not set")
	}

	return nil
}

func (ccms *CCMetricStore) doRequest(job *model.Job, suffix string, metrics []string, ctx context.Context) (*http.Response, error) {
	from, to := job.StartTime.Unix(), job.StartTime.Add(time.Duration(job.Duration)*time.Second).Unix()
	reqBody := ApiRequestBody{}
	reqBody.Metrics = metrics
	for _, node := range job.Nodes {
		reqBody.Selectors = append(reqBody.Selectors, []string{job.ClusterID, node})
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

func (ccms *CCMetricStore) LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error) {
	res, err := ccms.doRequest(job, "timeseries?with-stats=true", metrics, ctx)
	if err != nil {
		return nil, err
	}

	resdata := make([]map[string]ApiMetricData, 0, len(job.Nodes))
	if err := json.NewDecoder(res.Body).Decode(&resdata); err != nil {
		return nil, err
	}

	var jobData schema.JobData = make(schema.JobData)
	for _, metric := range metrics {
		mc := config.GetMetricConfig(job.ClusterID, metric)
		metricData := &schema.JobMetric{
			Scope:    "node", // TODO: FIXME: Whatever...
			Unit:     mc.Unit,
			Timestep: mc.Sampletime,
			Series:   make([]*schema.MetricSeries, 0, len(job.Nodes)),
		}
		for i, node := range job.Nodes {
			data := resdata[i][metric]
			if data.Error != nil {
				return nil, errors.New(*data.Error)
			}

			if data.Avg == nil || data.Min == nil || data.Max == nil {
				return nil, fmt.Errorf("no data for node '%s' and metric '%s'", node, metric)
			}

			metricData.Series = append(metricData.Series, &schema.MetricSeries{
				NodeID: node,
				Data:   data.Data,
				Statistics: &schema.MetricStatistics{
					Avg: *data.Avg,
					Min: *data.Min,
					Max: *data.Max,
				},
			})
		}
		jobData[metric] = metricData
	}

	return jobData, nil
}

func (ccms *CCMetricStore) LoadStats(job *model.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {
	res, err := ccms.doRequest(job, "stats", metrics, ctx)
	if err != nil {
		return nil, err
	}

	resdata := make([]map[string]ApiStatsData, 0, len(job.Nodes))
	if err := json.NewDecoder(res.Body).Decode(&resdata); err != nil {
		return nil, err
	}

	stats := map[string]map[string]schema.MetricStatistics{}
	for _, metric := range metrics {
		nodestats := map[string]schema.MetricStatistics{}
		for i, node := range job.Nodes {
			data := resdata[i][metric]
			if data.Error != nil {
				return nil, errors.New(*data.Error)
			}

			if data.Samples == 0 {
				return nil, fmt.Errorf("no data for node '%s' and metric '%s'", node, metric)
			}

			nodestats[node] = schema.MetricStatistics{
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
