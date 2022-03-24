package metricdata

import (
	"context"
	"time"

	"github.com/ClusterCockpit/cc-backend/schema"
)

var TestLoadDataCallback func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
	panic("TODO")
}

// Only a mock for unit-testing.
type TestMetricDataRepository struct {
	url, token string
	renamings  map[string]string
}

func (tmdr *TestMetricDataRepository) Init(url, token string, renamings map[string]string) error {
	tmdr.url = url
	tmdr.token = token
	tmdr.renamings = renamings
	return nil
}

func (tmdr *TestMetricDataRepository) LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
	return TestLoadDataCallback(job, metrics, scopes, ctx)
}

func (tmdr *TestMetricDataRepository) LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {
	panic("TODO")
}

func (tmdr *TestMetricDataRepository) LoadNodeData(cluster string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {
	panic("TODO")
}
