// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestDecodeJobDataRejectsFilesystemsArray(t *testing.T) {
	raw := `{
		"cpu_load": {"node": {"unit": {"base": ""}, "timestep": 60, "series": []}},
		"filesystems": [{"key": "/scratch", "instances": []}]
	}`

	if _, err := DecodeJobData(strings.NewReader(raw), t.Name()); err == nil {
		t.Fatal("expected a decode error for a top-level filesystems array")
	}
	if _, err := DecodeJobStats(strings.NewReader(raw), t.Name()+"-stats"); err == nil {
		t.Fatal("expected a decode error for a top-level filesystems array")
	}
}

func deviceScopedJobData() schema.JobData {
	home, scratch := "/home", "/scratch"
	return schema.JobData{Metrics: map[string]schema.ScopedMetrics{
		"fs_read_bw": {
			schema.MetricScopeNode: &schema.JobMetric{
				Unit:     schema.Unit{Base: "B/s"},
				Timestep: 60,
				Series: []schema.Series{{
					Hostname:   "node001",
					Data:       []schema.Float{3, 3},
					Statistics: schema.MetricStatistics{Min: 3, Avg: 3, Max: 3},
				}},
			},
			schema.MetricScopeFilesystem: &schema.JobMetric{
				Unit:     schema.Unit{Base: "B/s"},
				Timestep: 60,
				Series: []schema.Series{
					{
						Hostname:   "node001",
						ID:         &home,
						Data:       []schema.Float{1, 1},
						Statistics: schema.MetricStatistics{Min: 1, Avg: 1, Max: 1},
					},
					{
						Hostname:   "node001",
						ID:         &scratch,
						Data:       []schema.Float{2, 2},
						Statistics: schema.MetricStatistics{Min: 2, Avg: 2, Max: 2},
					},
				},
			},
		},
	}}
}

func TestDecodeDeviceScopedJobData(t *testing.T) {
	raw, err := json.Marshal(deviceScopedJobData())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	jd, err := DecodeJobData(bytes.NewReader(raw), t.Name())
	if err != nil {
		t.Fatalf("DecodeJobData: %v", err)
	}
	fs := jd.Metrics["fs_read_bw"][schema.MetricScopeFilesystem]
	if fs == nil || len(fs.Series) != 2 {
		t.Fatalf("filesystem scope not decoded: %+v", jd.Metrics["fs_read_bw"])
	}
	if *fs.Series[0].ID != "/home" || *fs.Series[1].ID != "/scratch" {
		t.Errorf("series ids = %q, %q; want /home, /scratch", *fs.Series[0].ID, *fs.Series[1].ID)
	}
	if jd.Metrics["fs_read_bw"][schema.MetricScopeNode] == nil {
		t.Error("node scope not decoded")
	}

	stats, err := DecodeJobStats(bytes.NewReader(raw), t.Name()+"-stats")
	if err != nil {
		t.Fatalf("DecodeJobStats: %v", err)
	}
	fsStats := stats.Metrics["fs_read_bw"][schema.MetricScopeFilesystem]
	if len(fsStats) != 2 {
		t.Fatalf("filesystem stats = %d series, want 2", len(fsStats))
	}
	if *fsStats[1].ID != "/scratch" || fsStats[1].Data.Avg != 2 {
		t.Errorf("filesystem stats[1] = %s avg %v, want /scratch avg 2", *fsStats[1].ID, fsStats[1].Data.Avg)
	}
	if nodeStats := stats.Metrics["fs_read_bw"][schema.MetricScopeNode]; len(nodeStats) != 1 || nodeStats[0].Data.Avg != 3 {
		t.Errorf("node stats = %+v, want one series with avg 3", nodeStats)
	}
}
