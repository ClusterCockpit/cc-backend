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
  "api-allowed-ips": [
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

	if err := ResetConnection(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ResetConnection()
	})

	Connect(dbfilepath)

	if err := archive.Init(json.RawMessage(archiveCfg)); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateNodeState(t *testing.T) {
	nodeTestSetup(t)

	repo := GetNodeRepository()
	now := time.Now().Unix()

	nodeState := schema.NodeStateDB{
		TimeStamp:       now,
		NodeState:       "allocated",
		CpusAllocated:   72,
		MemoryAllocated: 480,
		GpusAllocated:   0,
		HealthState:     schema.MonitoringStateFull,
		JobsRunning:     1,
	}

	err := repo.UpdateNodeState("host124", "testcluster", &nodeState)
	if err != nil {
		t.Fatal(err)
	}

	node, err := repo.GetNode("host124", "testcluster", false)
	if err != nil {
		t.Fatal(err)
	}

	if node.NodeState != "allocated" {
		t.Errorf("wrong node state\ngot: %s \nwant: allocated ", node.NodeState)
	}

	t.Run("FindBeforeEmpty", func(t *testing.T) {
		// Only the current-timestamp row exists, so nothing should be found before now
		rows, err := repo.FindNodeStatesBefore(now)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("expected 0 rows, got %d", len(rows))
		}
	})

	t.Run("DeleteOldRows", func(t *testing.T) {
		// Insert 2 more old rows for host124
		for i, ts := range []int64{now - 7200, now - 3600} {
			ns := schema.NodeStateDB{
				TimeStamp:       ts,
				NodeState:       "allocated",
				HealthState:     schema.MonitoringStateFull,
				CpusAllocated:   72,
				MemoryAllocated: 480,
				JobsRunning:     i,
			}
			if err := repo.UpdateNodeState("host124", "testcluster", &ns); err != nil {
				t.Fatal(err)
			}
		}

		// Delete rows older than 30 minutes
		cutoff := now - 1800
		cnt, err := repo.DeleteNodeStatesBefore(cutoff)
		if err != nil {
			t.Fatal(err)
		}

		// Should delete the 2 old rows
		if cnt != 2 {
			t.Errorf("expected 2 deleted rows, got %d", cnt)
		}

		// Latest row should still exist
		node, err := repo.GetNode("host124", "testcluster", false)
		if err != nil {
			t.Fatal(err)
		}
		if node.NodeState != "allocated" {
			t.Errorf("expected node state 'allocated', got %s", node.NodeState)
		}
	})

	t.Run("PreservesLatestPerNode", func(t *testing.T) {
		// Insert a single old row for host125 — it's the latest per node so it must survive
		ns := schema.NodeStateDB{
			TimeStamp:       now - 7200,
			NodeState:       "idle",
			HealthState:     schema.MonitoringStateFull,
			CpusAllocated:   0,
			MemoryAllocated: 0,
			JobsRunning:     0,
		}
		if err := repo.UpdateNodeState("host125", "testcluster", &ns); err != nil {
			t.Fatal(err)
		}

		// Delete everything older than now — the latest per node should be preserved
		_, err := repo.DeleteNodeStatesBefore(now)
		if err != nil {
			t.Fatal(err)
		}

		// The latest row for host125 must still exist
		node, err := repo.GetNode("host125", "testcluster", false)
		if err != nil {
			t.Fatal(err)
		}
		if node.NodeState != "idle" {
			t.Errorf("expected node state 'idle', got %s", node.NodeState)
		}

		// Verify exactly 1 row remains for host125
		var countAfter int
		if err := repo.DB.QueryRow(
			"SELECT COUNT(*) FROM node_state WHERE node_id = (SELECT id FROM node WHERE hostname = 'host125')").
			Scan(&countAfter); err != nil {
			t.Fatal(err)
		}
		if countAfter != 1 {
			t.Errorf("expected 1 row remaining for host125, got %d", countAfter)
		}
	})

	t.Run("FindBeforeWithJoin", func(t *testing.T) {
		// Insert old and current rows for host123
		for _, ts := range []int64{now - 7200, now} {
			ns := schema.NodeStateDB{
				TimeStamp:       ts,
				NodeState:       "allocated",
				HealthState:     schema.MonitoringStateFull,
				CpusAllocated:   8,
				MemoryAllocated: 1024,
				GpusAllocated:   1,
				JobsRunning:     1,
			}
			if err := repo.UpdateNodeState("host123", "testcluster", &ns); err != nil {
				t.Fatal(err)
			}
		}

		// Find rows older than 30 minutes, excluding latest per node
		cutoff := now - 1800
		rows, err := repo.FindNodeStatesBefore(cutoff)
		if err != nil {
			t.Fatal(err)
		}

		// Should find the old host123 row
		found := false
		for _, row := range rows {
			if row.Hostname == "host123" && row.TimeStamp == now-7200 {
				found = true
				if row.Cluster != "testcluster" {
					t.Errorf("expected cluster 'testcluster', got %s", row.Cluster)
				}
				if row.SubCluster != "sc1" {
					t.Errorf("expected subcluster 'sc1', got %s", row.SubCluster)
				}
				if row.CpusAllocated != 8 {
					t.Errorf("expected cpus_allocated 8, got %d", row.CpusAllocated)
				}
			}
		}
		if !found {
			t.Errorf("expected to find old host123 row among %d results", len(rows))
		}
	})
}
