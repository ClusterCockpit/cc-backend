// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	Connect("sqlite3", "../../test/test.db")
}

const testconfig = `{
	"addr":            "0.0.0.0:8080",
	"validate": false,
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	},
	"clusters": [
	{
	   "name": "taurus",
	   "metricDataRepository": {"kind": "test", "url": "bla:8081"},
		"filterRanges": {
		  "numNodes": { "from": 1, "to": 4000 },
		  "duration": { "from": 0, "to": 604800 },
		  "startTime": { "from": "2010-01-01T00:00:00Z", "to": null }
		}
	}
	]
}`
const testclusterJson = `{
		"name": "taurus",
		"SubClusters": [
		  {
			"name": "haswell",
			"processorType": "Intel Haswell",
			"socketsPerNode": 2,
			"coresPerSocket": 12,
			"threadsPerCore": 1,
			"flopRateScalar": 32,
			"flopRateSimd": 512,
			"memoryBandwidth": 60,
			"topology": {
			  "node": [ 0, 1 ],
			  "socket": [
				[ 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11 ],
				[ 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23 ]
			  ],
			  "memoryDomain": [
				[ 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11 ],
				[ 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23 ]
			  ],
			  "core": [ [ 0 ], [ 1 ], [ 2 ], [ 3 ], [ 4 ], [ 5 ], [ 6 ], [ 7 ], [ 8 ], [ 9 ], [ 10 ], [ 11 ], [ 12 ], [ 13 ], [ 14 ], [ 15 ], [ 16 ], [ 17 ], [ 18 ], [ 19 ], [ 20 ], [ 21 ], [ 22 ], [ 23 ] ]
			}
		  }
		],
		"metricConfig": [
		  {
			"name": "cpu_used",
			"scope": "core",
			"unit": "",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 1,
				"normal": 0.5,
				"caution": 2e-07,
				"alert": 1e-07
			  }
			]
		  },
		  {
			"name": "ipc",
			"scope": "core",
			"unit": "IPC",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 2,
				"normal": 1,
				"caution": 0.1,
				"alert": 0.5
			  }
			]
		  },
		  {
			"name": "flops_any",
			"scope": "core",
			"unit": "F/s",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 40000000000,
				"normal": 20000000000,
				"caution": 30000000000,
				"alert": 35000000000
			  }
			]
		  },
		  {
			"name": "mem_bw",
			"scope": "socket",
			"unit": "B/s",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 58800000000,
				"normal": 28800000000,
				"caution": 38800000000,
				"alert": 48800000000
			  }
			]
		  },
		  {
			"name": "file_bw",
			"scope": "node",
			"unit": "B/s",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 20000000000,
				"normal": 5000000000,
				"caution": 9000000000,
				"alert": 19000000000
			  }
			]
		  },
		  {
			"name": "net_bw",
			"scope": "node",
			"unit": "B/s",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 7000000000,
				"normal": 5000000000,
				"caution": 6000000000,
				"alert": 6500000000
			  }
			]
		  },
		  {
			"name": "mem_used",
			"scope": "node",
			"unit": "B",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 32000000000,
				"normal": 2000000000,
				"caution": 31000000000,
				"alert": 30000000000
			  }
			]
		  },
		  {
			"name": "cpu_power",
			"scope": "socket",
			"unit": "W",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 100,
				"normal": 80,
				"caution": 90,
				"alert": 90
			  }
			]
		  }
		]
	}`

func TestImportFlag(t *testing.T) {
	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "testcluster"), 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "testcluster", "cluster.json"), []byte(testclusterJson), 0666); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	f, err := os.Create(dbfilepath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0666); err != nil {
		t.Fatal(err)
	}

	config.Init(cfgFilePath)
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	Connect("sqlite3", dbfilepath)
	db := GetConnection()

	if err := archive.Init(json.RawMessage(archiveCfg)); err != nil {
		t.Fatal(err)
	}

	if err := metricdata.Init(config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	if _, err := db.DB.Exec(JobsDBSchema); err != nil {
		t.Fatal(err)
	}

	t.Run("Job 20639587", func(t *testing.T) {
		if err := HandleImportFlag("../../test/meta.json:../../test/data.json"); err != nil {
			t.Fatal(err)
		}

		repo := GetJobRepository()
		jobId := int64(20639587)
		cluster := "taurus"
		startTime := int64(1635856524)
		job, err := repo.Find(&jobId, &cluster, &startTime)
		if err != nil {
			t.Fatal(err)
		}

		if job.NumNodes != 2 {
			t.Errorf("NumNode: Received %d, expected 2", job.NumNodes)
		}

		ar := archive.GetHandle()
		data, err := ar.LoadJobData(job)
		if err != nil {
			t.Fatal(err)
		}

		if len(data) != 8 {
			t.Errorf("Job data length: Got %d, want 8", len(data))
		}
	})
}
