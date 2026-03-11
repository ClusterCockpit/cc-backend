// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestDeepCopy(t *testing.T) {
	nodeId := "0"
	original := schema.JobData{
		"cpu_load": {
			schema.MetricScopeNode: &schema.JobMetric{
				Timestep: 60,
				Unit:     schema.Unit{Base: "load", Prefix: ""},
				Series: []schema.Series{
					{
						Hostname: "node001",
						ID:       &nodeId,
						Data:     []schema.Float{1.0, 2.0, 3.0},
						Statistics: schema.MetricStatistics{
							Min: 1.0,
							Avg: 2.0,
							Max: 3.0,
						},
					},
				},
				StatisticsSeries: &schema.StatsSeries{
					Min:    []schema.Float{1.0, 1.5, 2.0},
					Mean:   []schema.Float{2.0, 2.5, 3.0},
					Median: []schema.Float{2.0, 2.5, 3.0},
					Max:    []schema.Float{3.0, 3.5, 4.0},
					Percentiles: map[int][]schema.Float{
						25: {1.5, 2.0, 2.5},
						75: {2.5, 3.0, 3.5},
					},
				},
			},
		},
	}

	copied := deepCopy(original)

	original["cpu_load"][schema.MetricScopeNode].Series[0].Data[0] = 999.0
	original["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Min[0] = 888.0
	original["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Percentiles[25][0] = 777.0

	if copied["cpu_load"][schema.MetricScopeNode].Series[0].Data[0] != 1.0 {
		t.Errorf("Series data was not deeply copied: got %v, want 1.0",
			copied["cpu_load"][schema.MetricScopeNode].Series[0].Data[0])
	}

	if copied["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Min[0] != 1.0 {
		t.Errorf("StatisticsSeries was not deeply copied: got %v, want 1.0",
			copied["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Min[0])
	}

	if copied["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Percentiles[25][0] != 1.5 {
		t.Errorf("Percentiles was not deeply copied: got %v, want 1.5",
			copied["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Percentiles[25][0])
	}

	if copied["cpu_load"][schema.MetricScopeNode].Timestep != 60 {
		t.Errorf("Timestep not copied correctly: got %v, want 60",
			copied["cpu_load"][schema.MetricScopeNode].Timestep)
	}

	if copied["cpu_load"][schema.MetricScopeNode].Series[0].Hostname != "node001" {
		t.Errorf("Hostname not copied correctly: got %v, want node001",
			copied["cpu_load"][schema.MetricScopeNode].Series[0].Hostname)
	}
}

func TestDeepCopyNilStatisticsSeries(t *testing.T) {
	original := schema.JobData{
		"mem_used": {
			schema.MetricScopeNode: &schema.JobMetric{
				Timestep: 60,
				Series: []schema.Series{
					{
						Hostname: "node001",
						Data:     []schema.Float{1.0, 2.0},
					},
				},
				StatisticsSeries: nil,
			},
		},
	}

	copied := deepCopy(original)

	if copied["mem_used"][schema.MetricScopeNode].StatisticsSeries != nil {
		t.Errorf("StatisticsSeries should be nil, got %v",
			copied["mem_used"][schema.MetricScopeNode].StatisticsSeries)
	}
}

func TestDeepCopyEmptyPercentiles(t *testing.T) {
	original := schema.JobData{
		"cpu_load": {
			schema.MetricScopeNode: &schema.JobMetric{
				Timestep: 60,
				Series:   []schema.Series{},
				StatisticsSeries: &schema.StatsSeries{
					Min:         []schema.Float{1.0},
					Mean:        []schema.Float{2.0},
					Median:      []schema.Float{2.0},
					Max:         []schema.Float{3.0},
					Percentiles: nil,
				},
			},
		},
	}

	copied := deepCopy(original)

	if copied["cpu_load"][schema.MetricScopeNode].StatisticsSeries.Percentiles != nil {
		t.Errorf("Percentiles should be nil when source is nil/empty")
	}
}
