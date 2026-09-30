// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package footprint

import (
	"testing"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func jobOn(hosts ...string) *schema.Job {
	job := &schema.Job{NumNodes: int32(len(hosts))}
	for _, h := range hosts {
		job.Resources = append(job.Resources, &schema.Resource{Hostname: h})
	}
	return job
}

func TestFold(t *testing.T) {
	tests := []struct {
		name  string
		job   *schema.Job
		stats map[string]map[string]schema.MetricStatistics
		want  map[string]schema.MetricStatistics
	}{
		{
			name: "positive minimum is not clamped to zero",
			job:  jobOn("n1", "n2"),
			stats: map[string]map[string]schema.MetricStatistics{
				"flops_any": {
					"n1": {Avg: 20, Min: 12.5, Max: 30},
					"n2": {Avg: 10, Min: 8.25, Max: 15},
				},
			},
			want: map[string]schema.MetricStatistics{
				"flops_any": {Avg: 15, Min: 8.25, Max: 30},
			},
		},
		{
			name: "average is the mean over nodes",
			job:  jobOn("n1", "n2"),
			stats: map[string]map[string]schema.MetricStatistics{
				"mem_bw": {
					"n1": {Avg: 100, Min: 90, Max: 110},
					"n2": {Avg: 50, Min: 40, Max: 60},
				},
			},
			want: map[string]schema.MetricStatistics{
				"mem_bw": {Avg: 75, Min: 40, Max: 110},
			},
		},
		{
			name: "hosts without data are skipped",
			job:  jobOn("n1", "n2"),
			stats: map[string]map[string]schema.MetricStatistics{
				"cpu_load": {"n1": {Avg: 64, Min: 60, Max: 72}},
			},
			want: map[string]schema.MetricStatistics{
				"cpu_load": {Avg: 64, Min: 60, Max: 72},
			},
		},
		{
			name: "hosts outside the job are ignored and empty metrics omitted",
			job:  jobOn("n1"),
			stats: map[string]map[string]schema.MetricStatistics{
				"cpu_load": {"n9": {Avg: 1, Min: 1, Max: 1}},
			},
			want: map[string]schema.MetricStatistics{},
		},
		{
			name: "values are rounded to two digits",
			job:  jobOn("n1", "n2", "n3"),
			stats: map[string]map[string]schema.MetricStatistics{
				"m": {
					"n1": {Avg: 1, Min: 0.123, Max: 1.005},
					"n2": {Avg: 1, Min: 0.5, Max: 1},
					"n3": {Avg: 2, Min: 0.5, Max: 1},
				},
			},
			want: map[string]schema.MetricStatistics{
				"m": {Avg: 1.33, Min: 0.12, Max: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fold(tt.job, tt.stats)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d metrics %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for m, w := range tt.want {
				if got[m] != w {
					t.Errorf("metric %s: got %+v, want %+v", m, got[m], w)
				}
			}
		})
	}
}

func testSubCluster() *schema.SubCluster {
	mc := func(name, fp, energy string) *schema.MetricConfig {
		return &schema.MetricConfig{Metric: schema.Metric{Name: name}, Footprint: fp, Energy: energy}
	}
	return &schema.SubCluster{
		Name: "main",
		MetricConfig: []*schema.MetricConfig{
			mc("flops_any", "max", ""),
			mc("mem_bw", "avg", ""),
			mc("cpu_power", "", "power"),
			mc("acc_energy", "", "energy"),
		},
		Footprint:       []string{"flops_any", "mem_bw"},
		EnergyFootprint: []string{"cpu_power", "acc_energy"},
	}
}

func TestStatTypeSubClusterOverride(t *testing.T) {
	saved := archive.GlobalMetricList
	t.Cleanup(func() { archive.GlobalMetricList = saved })
	archive.GlobalMetricList = []*schema.GlobalMetricListItem{
		{Name: "flops_any", Footprint: "avg"},
		{Name: "mem_bw", Footprint: "avg"},
		{Name: "global_only", Footprint: "min"},
	}

	sc := testSubCluster()
	fp, err := Build(sc, map[string]schema.MetricStatistics{
		"flops_any": {Avg: 10, Min: 5, Max: 20},
		"mem_bw":    {Avg: 7, Min: 1, Max: 9},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fp["flops_any_avg"]; ok {
		t.Errorf("global statistic used instead of subcluster override: %v", fp)
	}
	if fp["flops_any_max"] != 20 {
		t.Errorf("flops_any_max = %v, want 20 (%v)", fp["flops_any_max"], fp)
	}
	if fp["mem_bw_avg"] != 7 {
		t.Errorf("mem_bw_avg = %v, want 7", fp["mem_bw_avg"])
	}

	st, err := StatType(sc, "global_only")
	if err != nil || st != "min" {
		t.Errorf("StatType fallback = %q, %v; want min", st, err)
	}
	if _, err := StatType(sc, "unknown"); err == nil {
		t.Error("expected error for metric without footprint statistic")
	}
}

func TestBuildMissingMetrics(t *testing.T) {
	sc := testSubCluster()
	stats := map[string]schema.MetricStatistics{"mem_bw": {Avg: 3}}

	omit, err := Build(sc, stats, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(omit) != 1 || omit["mem_bw_avg"] != 3 {
		t.Errorf("omit missing: got %v", omit)
	}

	zero, err := Build(sc, stats, true)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := zero["flops_any_max"]; !ok || v != 0 {
		t.Errorf("zero missing: got %v", zero)
	}
}

func TestBuildEnergy(t *testing.T) {
	sc := testSubCluster()
	stats := map[string]schema.MetricStatistics{
		"cpu_power":  {Avg: 250},
		"acc_energy": {Avg: 1000},
	}

	// 250 W per node * 4 nodes * 2 h / 1000 = 2 kWh
	efp, total := BuildEnergy(sc, stats, 4, 7200)
	if efp["cpu_power"] != 2 {
		t.Errorf("cpu_power = %v kWh, want 2", efp["cpu_power"])
	}
	if v, ok := efp["acc_energy"]; !ok || v != 0 {
		t.Errorf("acc_energy = %v (present %v), want 0", v, ok)
	}
	if total != 2 {
		t.Errorf("total = %v, want 2", total)
	}

	// 333 W * 1 node * 1 h / 1000 = 0.333 -> 0.33
	_, total = BuildEnergy(sc, map[string]schema.MetricStatistics{"cpu_power": {Avg: 333}}, 1, 3600)
	if total != 0.33 {
		t.Errorf("rounded total = %v, want 0.33", total)
	}
}
