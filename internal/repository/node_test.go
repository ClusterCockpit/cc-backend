// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	_ "github.com/mattn/go-sqlite3"
)

func nodeTestSetup(t *testing.T) {
	const testconfig = `{
		"main": {
	"addr":            "0.0.0.0:8080",
	"validate": false,
  "apiAllowedIPs": [
    "*"
  ]
	},
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	},
	"auth": {
  "jwts": {
      "max-age": "2m"
  }
	}
}`
	const testclusterJSON = `{
        "name": "testcluster",
		"subClusters": [
			{
				"name": "sc1",
				"nodes": "host123,host124,host125",
				"processorType": "Intel Core i7-4770",
				"socketsPerNode": 1,
				"coresPerSocket": 4,
				"threadsPerCore": 2,
                "flopRateScalar": {
                  "unit": {
                    "prefix": "G",
                    "base": "F/s"
                  },
                  "value": 14
                },
                "flopRateSimd": {
                  "unit": {
                    "prefix": "G",
                    "base": "F/s"
                  },
                  "value": 112
                },
                "memoryBandwidth": {
                  "unit": {
                    "prefix": "G",
                    "base": "B/s"
                  },
                  "value": 24
                },
                "numberOfNodes": 70,
				"topology": {
					"node": [0, 1, 2, 3, 4, 5, 6, 7],
					"socket": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"memoryDomain": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"die": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"core": [[0], [1], [2], [3], [4], [5], [6], [7]]
				}
			}
		],
		"metricConfig": [
			{
				"name": "load_one",
			    "unit": { "base": ""},
				"scope": "node",
				"timestep": 60,
        "aggregation": "avg",
				"peak": 8,
				"normal": 0,
				"caution": 0,
				"alert": 0
			}
		]
	}`

	cclog.Init("debug", true)
	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0o777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "version.txt"),
		fmt.Appendf(nil, "%d", 3), 0o666); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "testcluster"),
		0o777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "testcluster", "cluster.json"),
		[]byte(testclusterJSON), 0o666); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	err := MigrateDB(dbfilepath)
	if err != nil {
		t.Fatal(err)
	}

	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0o666); err != nil {
		t.Fatal(err)
	}

	ccconf.Init(cfgFilePath)

	// Load and check main configuration
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		config.Init(cfg)
	} else {
		cclog.Abort("Main configuration must be present")
	}
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	Connect("sqlite3", dbfilepath)

	if err := archive.Init(json.RawMessage(archiveCfg)); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateNodeState(t *testing.T) {
	nodeTestSetup(t)

	nodeState := schema.NodeStateDB{
		TimeStamp: time.Now().Unix(), NodeState: "allocated",
		CpusAllocated:   72,
		MemoryAllocated: 480,
		GpusAllocated:   0,
		HealthState:     schema.MonitoringStateFull,
		JobsRunning:     1,
	}

	repo := GetNodeRepository()
	err := repo.UpdateNodeState("host124", "testcluster", &nodeState)
	if err != nil {
		return
	}

	node, err := repo.GetNode("host124", "testcluster", false)
	if err != nil {
		return
	}

	if node.NodeState != "allocated" {
		t.Errorf("wrong node state\ngot: %s \nwant: allocated ", node.NodeState)
	}
}
