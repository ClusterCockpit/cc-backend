package metricdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (ccms *CCMetricStore) Init() error {
	ccms.url = os.Getenv("CCMETRICSTORE_URL")
	ccms.jwt = os.Getenv("CCMETRICSTORE_JWT")
	if ccms.url == "" || ccms.jwt == "" {
		return errors.New("environment variables 'CCMETRICSTORE_URL' or 'CCMETRICSTORE_JWT' not set")
	}

	return nil
}

func (ccms *CCMetricStore) LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error) {
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

	authHeader := fmt.Sprintf("Bearer %s", ccms.jwt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/%d/%d/timeseries?with-stats=true", ccms.url, from, to), bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", authHeader)
	res, err := ccms.client.Do(req)
	if err != nil {
		return nil, err
	}

	resdata := make([]map[string]ApiMetricData, 0, len(reqBody.Selectors))
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
				return nil, errors.New("no data")
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
