// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdata

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var TestLoadDataCallback func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
	panic("TODO")
}

// Only a mock for unit-testing.
type TestMetricDataRepository struct{}

func (tmdr *TestMetricDataRepository) Init(_ json.RawMessage) error {
	return nil
}

func (tmdr *TestMetricDataRepository) LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context) (schema.JobData, error) {

	return TestLoadDataCallback(job, metrics, scopes, ctx)
}

func (tmdr *TestMetricDataRepository) LoadStats(
	job *schema.Job,
	metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {

	panic("TODO")
}

func (tmdr *TestMetricDataRepository) LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {

	panic("TODO")
}
