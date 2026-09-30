// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

const (
	fsFrom  = int64(1000)
	fsFreq  = int64(10)
	fsCount = 20
)

// newFetchStatsStore installs a store with one node-native metric ("load") on
// cl/n1 and two core-level metrics ("flops" summed, "temp" averaged) on
// cl/n1/core0 and cl/n1/core1. core1 of "flops" is written so that the per
// timestep sum is constant (19) while the per-core minimum is 0: this tells
// per-timestep aggregation apart from combining per-buffer extremes.
func newFetchStatsStore(t *testing.T, core1Offset int) *MemoryStore {
	t.Helper()
	ms := &MemoryStore{
		Metrics: map[string]MetricConfig{
			"load":  {Frequency: fsFreq, Aggregation: AvgAggregation, offset: 0},
			"flops": {Frequency: fsFreq, Aggregation: SumAggregation, offset: 1},
			"temp":  {Frequency: fsFreq, Aggregation: AvgAggregation, offset: 2},
		},
		root: Level{
			metrics:  make([]*buffer, 3),
			children: make(map[string]*Level),
		},
	}

	for i := range fsCount {
		ts := fsFrom + int64(i)*fsFreq
		if err := ms.Write([]string{"cl", "n1"}, ts, []Metric{
			{Name: "load", Value: schema.Float(float64(i%5) + 1)},
		}); err != nil {
			t.Fatal(err)
		}
		if err := ms.Write([]string{"cl", "n1", "core0"}, ts, []Metric{
			{Name: "flops", Value: schema.Float(i)},
			{Name: "temp", Value: schema.Float(40 + i)},
		}); err != nil {
			t.Fatal(err)
		}
		if i >= core1Offset {
			if err := ms.Write([]string{"cl", "n1", "core1"}, ts, []Metric{
				{Name: "flops", Value: schema.Float(fsCount - 1 - i)},
				{Name: "temp", Value: schema.Float(60 - i)},
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	old := msInstance
	msInstance = ms
	t.Cleanup(func() { msInstance = old })
	return ms
}

func coreQuery(metric string, avgOnly bool) APIQuery {
	core := "core"
	return APIQuery{
		Metric: metric, Hostname: "n1", Type: &core,
		TypeIds: []string{"0", "1"}, Aggregate: true, AvgOnly: avgOnly,
	}
}

// fetchBoth runs the same request through FetchData (full read) and FetchStats.
func fetchBoth(t *testing.T, q APIQuery) (full, fast APIMetricData) {
	t.Helper()
	req := APIQueryRequest{
		Cluster: "cl", From: fsFrom, To: fsFrom + fsCount*fsFreq,
		Queries: []APIQuery{q}, WithStats: true,
	}
	fd, err := FetchData(req)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := FetchStats(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.Results) != 1 || len(fd.Results[0]) != 1 || len(fs.Results) != 1 || len(fs.Results[0]) != 1 {
		t.Fatalf("unexpected result shapes: full %v, fast %v", fd.Results, fs.Results)
	}
	full, fast = fd.Results[0][0], fs.Results[0][0]
	if full.Error != nil || fast.Error != nil {
		t.Fatalf("errors: full %v, fast %v", full.Error, fast.Error)
	}
	if fast.Data != nil {
		t.Errorf("FetchStats returned data points")
	}
	return full, fast
}

func TestFetchStatsNodeNativeMatchesRead(t *testing.T) {
	ms := newFetchStatsStore(t, 0)
	q := APIQuery{Metric: "load", Hostname: "n1"}

	if _, _, _, ok := ms.fastStats(querySelectors("cl", q)[0], "load", fsFrom, fsFrom+fsCount*fsFreq, false); !ok {
		t.Fatal("node-native metric did not use the fast path")
	}

	full, fast := fetchBoth(t, q)
	if full.Avg != fast.Avg || full.Min != fast.Min || full.Max != fast.Max {
		t.Errorf("fast %v/%v/%v != full %v/%v/%v (avg/min/max)",
			fast.Avg, fast.Min, fast.Max, full.Avg, full.Min, full.Max)
	}
}

func TestFetchStatsAggregatedAvgOnlyMatchesRead(t *testing.T) {
	ms := newFetchStatsStore(t, 0)
	for _, metric := range []string{"flops", "temp"} {
		t.Run(metric, func(t *testing.T) {
			q := coreQuery(metric, true)
			if _, _, _, ok := ms.fastStats(querySelectors("cl", q)[0], metric, fsFrom, fsFrom+fsCount*fsFreq, true); !ok {
				t.Fatal("avg-only aggregated query did not use the fast path")
			}
			full, fast := fetchBoth(t, q)
			if full.Avg != fast.Avg {
				t.Errorf("avg: fast %v != full %v", fast.Avg, full.Avg)
			}
		})
	}
}

func TestFetchStatsAggregatedMinMaxUsesPerTimestepSemantics(t *testing.T) {
	ms := newFetchStatsStore(t, 0)
	q := coreQuery("flops", false)

	if _, _, _, ok := ms.fastStats(querySelectors("cl", q)[0], "flops", fsFrom, fsFrom+fsCount*fsFreq, false); ok {
		t.Fatal("aggregated query with min/max must not use the fast path")
	}

	full, fast := fetchBoth(t, q)
	// Sum of both cores is 19 at every timestep.
	if fast.Min != 19 || fast.Max != 19 || fast.Avg != 19 {
		t.Errorf("fast avg/min/max = %v/%v/%v, want 19/19/19", fast.Avg, fast.Min, fast.Max)
	}
	if full.Min != fast.Min || full.Max != fast.Max {
		t.Errorf("fast min/max %v/%v != full %v/%v", fast.Min, fast.Max, full.Min, full.Max)
	}
}

func TestFetchStatsMisalignedFallsBackToRead(t *testing.T) {
	ms := newFetchStatsStore(t, 3) // core1 starts three samples late
	q := coreQuery("temp", true)

	if _, _, _, ok := ms.fastStats(querySelectors("cl", q)[0], "temp", fsFrom, fsFrom+fsCount*fsFreq, true); ok {
		t.Fatal("misaligned buffers must not use the fast path")
	}

	full, fast := fetchBoth(t, q)
	if full.Avg != fast.Avg || full.Min != fast.Min || full.Max != fast.Max {
		t.Errorf("fallback fast %v/%v/%v != full %v/%v/%v",
			fast.Avg, fast.Min, fast.Max, full.Avg, full.Min, full.Max)
	}
}

func TestFetchStatsScaleFactor(t *testing.T) {
	newFetchStatsStore(t, 0)
	q := APIQuery{Metric: "load", Hostname: "n1"}
	full, _ := fetchBoth(t, q)

	q.ScaleFactor = 2
	_, scaled := fetchBoth(t, q)
	if scaled.Avg != 2*full.Avg || scaled.Max != 2*full.Max {
		t.Errorf("scaled avg/max = %v/%v, want %v/%v", scaled.Avg, scaled.Max, 2*full.Avg, 2*full.Max)
	}
}
