// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdata

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-lib/schema"
)

var TestLoadDataCallback func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error) = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error) {
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
	cluster, subCluster, nodeFilter string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	page *model.PageRequest,
	ctx context.Context,
) (map[string]schema.JobData, int, bool, error) {
	panic("TODO")
}

func DeepCopy(jd_temp schema.JobData) schema.JobData {

	jd := make(schema.JobData, len(jd_temp))
	for k, v := range jd_temp {
		jd[k] = make(map[schema.MetricScope]*schema.JobMetric, len(jd_temp[k]))
		for k_, v_ := range v {
			jd[k][k_] = new(schema.JobMetric)
			jd[k][k_].Series = make([]schema.Series, len(v_.Series))
			for i := 0; i < len(v_.Series); i += 1 {
				jd[k][k_].Series[i].Data = make([]schema.Float, len(v_.Series[i].Data))
				copy(jd[k][k_].Series[i].Data, v_.Series[i].Data)
				jd[k][k_].Series[i].Hostname = v_.Series[i].Hostname
				jd[k][k_].Series[i].Id = v_.Series[i].Id
				jd[k][k_].Series[i].Statistics.Avg = v_.Series[i].Statistics.Avg
				jd[k][k_].Series[i].Statistics.Min = v_.Series[i].Statistics.Min
				jd[k][k_].Series[i].Statistics.Max = v_.Series[i].Statistics.Max
			}
			jd[k][k_].Timestep = v_.Timestep
			jd[k][k_].Unit.Base = v_.Unit.Base
			jd[k][k_].Unit.Prefix = v_.Unit.Prefix
			if v_.StatisticsSeries != nil {
				// Init Slices
				jd[k][k_].StatisticsSeries = new(schema.StatsSeries)
				jd[k][k_].StatisticsSeries.Max = make([]schema.Float, len(v_.StatisticsSeries.Max))
				jd[k][k_].StatisticsSeries.Min = make([]schema.Float, len(v_.StatisticsSeries.Min))
				jd[k][k_].StatisticsSeries.Median = make([]schema.Float, len(v_.StatisticsSeries.Median))
				jd[k][k_].StatisticsSeries.Mean = make([]schema.Float, len(v_.StatisticsSeries.Mean))
				// Copy Data
				copy(jd[k][k_].StatisticsSeries.Max, v_.StatisticsSeries.Max)
				copy(jd[k][k_].StatisticsSeries.Min, v_.StatisticsSeries.Min)
				copy(jd[k][k_].StatisticsSeries.Median, v_.StatisticsSeries.Median)
				copy(jd[k][k_].StatisticsSeries.Mean, v_.StatisticsSeries.Mean)
				// Handle Percentiles
				for k__, v__ := range v_.StatisticsSeries.Percentiles {
					jd[k][k_].StatisticsSeries.Percentiles[k__] = make([]schema.Float, len(v__))
					copy(jd[k][k_].StatisticsSeries.Percentiles[k__], v__)
				}
			} else {
				jd[k][k_].StatisticsSeries = v_.StatisticsSeries
			}
		}
	}
	return jd
}
