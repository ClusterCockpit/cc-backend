// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	ccconf "github.com/ClusterCockpit/cc-lib/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	_ "github.com/mattn/go-sqlite3"
)

func setupUserTest(t *testing.T) *UserCfgRepo {
	const testconfig = `{
	"main": {
	 "addr":   "0.0.0.0:8080",
   "apiAllowedIPs": [
     "*"
   ]
  },
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	},
	"clusters": [
	{
	   "name": "testcluster",
	   "metricDataRepository": {"kind": "test", "url": "bla:8081"},
	   "filterRanges": {
		"numNodes": { "from": 1, "to": 64 },
		"duration": { "from": 0, "to": 86400 },
		"startTime": { "from": "2022-01-01T00:00:00Z", "to": null }
	}
	}]
}`

	cclog.Init("info", true)
	dbfilepath := "testdata/job.db"
	err := MigrateDB("sqlite3", dbfilepath)
	if err != nil {
		t.Fatal(err)
	}
	Connect("sqlite3", dbfilepath)

	tmpdir := t.TempDir()
	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0666); err != nil {
		t.Fatal(err)
	}

	ccconf.Init(cfgFilePath)

	// Load and check main configuration
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		if clustercfg := ccconf.GetPackageConfig("clusters"); clustercfg != nil {
			config.Init(cfg, clustercfg)
		} else {
			t.Fatal("Cluster configuration must be present")
		}
	} else {
		t.Fatal("Main configuration must be present")
	}

	return GetUserCfgRepo()
}

func TestGetUIConfig(t *testing.T) {
	r := setupUserTest(t)
	u := schema.User{Username: "demo"}

	cfg, err := r.GetUIConfig(&u)
	if err != nil {
		t.Fatal("No config")
	}

	tmp := cfg["plot_list_selectedMetrics"]
	metrics := tmp.([]string)
	str := metrics[2]
	if str != "flops_any" {
		t.Errorf("wrong config\ngot: %s \nwant: flops_any", str)
	}
}
