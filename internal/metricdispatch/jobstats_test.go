// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

import (
	"context"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestLoadJobStatsRunning(t *testing.T) {
	repo := &fakeRepo{stats: map[string]map[string]schema.MetricStatistics{
		"flops_any": {
			"n1": {Avg: 20, Min: 12.5, Max: 30},
			"n2": {Avg: 10, Min: 8.25, Max: 15},
		},
	}}
	useRepo(t, "statscluster", repo)

	job := &schema.Job{
		Cluster:   "statscluster",
		State:     schema.JobStateRunning,
		NumNodes:  2,
		Resources: []*schema.Resource{{Hostname: "n1"}, {Hostname: "n2"}},
	}

	data, err := LoadJobStats(job, []string{"flops_any", "mem_bw"}, context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := data["flops_any"]
	if got.Min != 8.25 {
		t.Errorf("min = %v, want 8.25 (min must not fold against 0)", got.Min)
	}
	if got.Avg != 15 || got.Max != 30 {
		t.Errorf("avg/max = %v/%v, want 15/30", got.Avg, got.Max)
	}
	if missing, ok := data["mem_bw"]; !ok || missing != (schema.MetricStatistics{}) {
		t.Errorf("metric without data: got %+v (present %v), want zero entry", missing, ok)
	}
}
