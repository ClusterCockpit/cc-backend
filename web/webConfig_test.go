// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package web

import (
	"encoding/json"
	"fmt"
	"testing"

	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
)

func TestInit(t *testing.T) {
	fp := "../../configs/config.json"
	ccconf.Init(fp)
	cfg := ccconf.GetPackageConfig("ui")

	Init(cfg)

	if UIDefaultsMap["nodelist_usePaging"] == false {
		t.Errorf("wrong option\ngot: %v \nwant: true", UIDefaultsMap["NodeList_UsePaging"])
	}
}

func TestSimpleDefaults(t *testing.T) {
	const s = `{
		"joblist": {
		    "showFootprint": false
		}
	}`

	Init(json.RawMessage(s))

	if UIDefaultsMap["joblist_usePaging"] == true {
		t.Errorf("wrong option\ngot: %v \nwant: false", UIDefaultsMap["NodeList_UsePaging"])
	}
}

func TestOverwrite(t *testing.T) {
	const s = `{
  "metricConfig": {
    "jobListMetrics": ["flops_sp", "flops_dp"],
    "clusters": [
      {
        "name": "fritz",
        "jobListMetrics": ["flops_any", "mem_bw", "load"],
        "subClusters": [
          {
            "name": "icelake",
            "jobListMetrics": ["flops_any", "mem_bw", "power", "load"],
            "jobViewPlotMetrics": ["load"]
          }
        ]
      }
    ]
  }
}`

	Init(json.RawMessage(s))

	fmt.Printf("%+v", UIDefaultsMap)
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
