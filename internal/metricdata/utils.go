// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdata

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ClusterCockpit/cc-lib/schema"
)

var TestLoadDataCallback func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error) = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error) {
	panic("TODO")
}

// TestMetricDataRepository is only a mock for unit-testing.
type TestMetricDataRepository struct{}

func (tmdr *TestMetricDataRepository) Init(_ json.RawMessage) error {
	return nil
}

func (tmdr *TestMetricDataRepository) LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int,
) (schema.JobData, error) {
	return TestLoadDataCallback(job, metrics, scopes, ctx, resolution)
}

func (tmdr *TestMetricDataRepository) LoadStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context,
) (map[string]map[string]schema.MetricStatistics, error) {
	panic("TODO")
}

func (tmdr *TestMetricDataRepository) LoadScopedStats(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
) (schema.ScopedJobStats, error) {
	panic("TODO")
}

func (tmdr *TestMetricDataRepository) LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context,
) (map[string]map[string][]*schema.JobMetric, error) {
	panic("TODO")
}

func (tmdr *TestMetricDataRepository) LoadNodeListData(
	cluster, subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	ctx context.Context,
) (map[string]schema.JobData, error) {
	panic("TODO")
}
