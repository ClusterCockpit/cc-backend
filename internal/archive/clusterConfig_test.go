// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive_test

import (
	"encoding/json"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/archive"
)

func TestClusterConfig(t *testing.T) {
	if err := archive.Init(json.RawMessage("{\"kind\": \"file\",\"path\": \"testdata/archive\"}"), false); err != nil {
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
