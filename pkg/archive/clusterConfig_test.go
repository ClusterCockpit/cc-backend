// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive_test

import (
	"encoding/json"
	"testing"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
)

func TestClusterConfig(t *testing.T) {
	if err := archive.Init(json.RawMessage("{\"kind\": \"file\",\"path\": \"testdata/archive\"}")); err != nil {
		t.Fatal(err)
	}

	sc, err := archive.GetSubCluster("fritz", "spr1tb")
	if err != nil {
		t.Fatal(err)
	}
	// spew.Dump(sc.MetricConfig)
	if len(sc.Footprint) != 3 {
		t.Fail()
	}
	if len(sc.MetricConfig) != 15 {
		t.Fail()
	}

	for _, metric := range sc.MetricConfig {
		if metric.LowerIsBetter && metric.Name != "mem_used" {
			t.Fail()
		}
	}

	// spew.Dump(archive.GlobalMetricList)
	// t.Fail()
}

func TestGetMetricConfigSubClusterRespectsRemovedMetrics(t *testing.T) {
	if err := archive.Init(json.RawMessage(`{"kind": "file","path": "testdata/archive"}`)); err != nil {
		t.Fatal(err)
	}

	sc, err := archive.GetSubCluster("fritz", "spr2tb")
	if err != nil {
		t.Fatal(err)
	}

	metrics := archive.GetMetricConfigSubCluster("fritz", "spr2tb")
	if len(metrics) != len(sc.MetricConfig) {
		t.Fatalf("GetMetricConfigSubCluster() returned %d metrics, want %d", len(metrics), len(sc.MetricConfig))
	}

	if _, ok := metrics["flops_any"]; ok {
		t.Fatalf("GetMetricConfigSubCluster() returned removed metric flops_any for subcluster spr2tb")
	}

	if _, ok := metrics["cpu_power"]; !ok {
		t.Fatalf("GetMetricConfigSubCluster() missing active metric cpu_power for subcluster spr2tb")
	}
}
