// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package web

import (
	"encoding/json"
	"testing"
)

func TestInitDefaults(t *testing.T) {
	// Test Init with nil config uses defaults
	err := Init(nil)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Check default values are set
	if UIDefaultsMap["jobList_usePaging"] != false {
		t.Errorf("wrong option\ngot: %v \nwant: false", UIDefaultsMap["jobList_usePaging"])
	}
	if UIDefaultsMap["nodeList_usePaging"] != false {
		t.Errorf("wrong option\ngot: %v \nwant: false", UIDefaultsMap["nodeList_usePaging"])
	}
	if UIDefaultsMap["jobView_showPolarPlot"] != true {
		t.Errorf("wrong option\ngot: %v \nwant: true", UIDefaultsMap["jobView_showPolarPlot"])
	}
}

func TestSimpleDefaults(t *testing.T) {
	const s = `{
		"job-list": {
		    "show-footprint": true
		}
	}`

	err := Init(json.RawMessage(s))
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify show-footprint was set
	if UIDefaultsMap["jobList_showFootprint"] != true {
		t.Errorf("wrong option\ngot: %v \nwant: true", UIDefaultsMap["jobList_showFootprint"])
	}

	// Verify other defaults remain unchanged
	if UIDefaultsMap["jobList_usePaging"] != false {
		t.Errorf("wrong option\ngot: %v \nwant: false", UIDefaultsMap["jobList_usePaging"])
	}
}

func TestOverwrite(t *testing.T) {
	const s = `{
  "metric-config": {
    "job-list-metrics": ["flops_sp", "flops_dp"],
    "clusters": [
      {
        "name": "fritz",
        "job-list-metrics": ["flops_any", "mem_bw", "load"],
        "sub-clusters": [
          {
            "name": "icelake",
            "job-list-metrics": ["flops_any", "mem_bw", "power", "load"],
            "job-view-plot-metrics": ["load"]
          }
        ]
      }
    ]
  }
}`

	err := Init(json.RawMessage(s))
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	v, ok := UIDefaultsMap["metricConfig_jobListMetrics"].([]string)
	if ok {
		if v[0] != "flops_sp" {
			t.Errorf("wrong metric\ngot: %s \nwant: flops_sp", v[0])
		}
	} else {
		t.Errorf("missing Key\nkey: metricConfig_jobListMetrics")
	}
	v, ok = UIDefaultsMap["metricConfig_jobListMetrics:fritz"].([]string)
	if ok {
		if v[0] != "flops_any" {
			t.Errorf("wrong metric\ngot: %s \nwant: flops_any", v[0])
		}
	} else {
		t.Errorf("missing Key\nkey: metricConfig_jobListMetrics:fritz")
	}
	v, ok = UIDefaultsMap["metricConfig_jobListMetrics:fritz:icelake"].([]string)
	if ok {
		if v[3] != "load" {
			t.Errorf("wrong metric\ngot: %s \nwant: load", v[3])
		}
	} else {
		t.Errorf("missing Key\nkey: metricConfig_jobListMetrics:fritz:icelake")
	}
}
