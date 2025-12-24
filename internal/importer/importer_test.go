// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// copyFile copies a file from source path to destination path.
// Used by tests to set up test fixtures.
func copyFile(s string, d string) error {
	r, err := os.Open(s)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(d)
	if err != nil {
		return err
	}
	defer w.Close()
	w.ReadFrom(r)
	return nil
}

// setup initializes a test environment for importer tests.
//
// Creates a temporary directory with:
//   - A test job archive with cluster configuration
//   - A SQLite database initialized with schema
//   - Configuration files loaded
//
// Returns a JobRepository instance for test assertions.
func setup(t *testing.T) *repository.JobRepository {
	const testconfig = `{
		"main": {
	"addr":            "0.0.0.0:8080",
	"validate": false,
  "apiAllowedIPs": [
    "*"
  ]},
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
	},
    {
	   "name": "fritz",
	   "metricDataRepository": {"kind": "test", "url": "bla:8081"},
	   "filterRanges": {
		"numNodes": { "from": 1, "to": 944 },
		"duration": { "from": 0, "to": 86400 },
		"startTime": { "from": "2022-01-01T00:00:00Z", "to": null }
	   }
	},
    {
		"name": "taurus",
		"metricDataRepository": {"kind": "test", "url": "bla:8081"},
		 "filterRanges": {
		   "numNodes": { "from": 1, "to": 4000 },
		   "duration": { "from": 0, "to": 604800 },
		   "startTime": { "from": "2010-01-01T00:00:00Z", "to": null }
		 }
	 }
	]}`

	cclog.Init("info", true)
	tmpdir := t.TempDir()

	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobarchive, "version.txt"), fmt.Appendf(nil, "%d", 3), 0o666); err != nil {
		t.Fatal(err)
	}
	fritzArchive := filepath.Join(tmpdir, "job-archive", "fritz")
	if err := os.Mkdir(fritzArchive, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join("testdata", "cluster-fritz.json"),
		filepath.Join(fritzArchive, "cluster.json")); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	err := repository.MigrateDB(dbfilepath)
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
		if clustercfg := ccconf.GetPackageConfig("clusters"); clustercfg != nil {
			config.Init(cfg, clustercfg)
		} else {
			t.Fatal("Cluster configuration must be present")
		}
	} else {
		t.Fatal("Main configuration must be present")
	}

	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	if err := archive.Init(json.RawMessage(archiveCfg), config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	repository.Connect("sqlite3", dbfilepath)
	return repository.GetJobRepository()
}

// Result represents the expected test result for job import verification.
type Result struct {
	JobId     int64
	Cluster   string
	StartTime int64
	Duration  int32
}

// readResult reads the expected test result from a golden file.
// Golden files contain the expected job attributes after import.
func readResult(t *testing.T, testname string) Result {
	var r Result

	content, err := os.ReadFile(filepath.Join("testdata",
		fmt.Sprintf("%s-golden.json", testname)))
	if err != nil {
		t.Fatal("Error when opening file: ", err)
	}

	err = json.Unmarshal(content, &r)
	if err != nil {
		t.Fatal("Error during Unmarshal(): ", err)
	}

	return r
}

// TestHandleImportFlag tests the HandleImportFlag function with various job import scenarios.
//
// The test uses golden files in testdata/ to verify that jobs are correctly:
//   - Parsed from metadata and data JSON files
//   - Enriched with footprints and energy metrics
//   - Inserted into the database
//   - Retrievable with correct attributes
func TestHandleImportFlag(t *testing.T) {
	r := setup(t)

	tests, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range tests {
		_, filename := filepath.Split(path)
		str := strings.Split(strings.TrimSuffix(filename, ".input"), "-")
		testname := str[1]

		t.Run(testname, func(t *testing.T) {
			s := fmt.Sprintf("%s:%s", filepath.Join("testdata",
				fmt.Sprintf("meta-%s.input", testname)),
				filepath.Join("testdata", fmt.Sprintf("data-%s.json", testname)))
			err := importer.HandleImportFlag(s)
			if err != nil {
				t.Fatal(err)
			}

			result := readResult(t, testname)
			job, err := r.FindCached(&result.JobId, &result.Cluster, &result.StartTime)
			if err != nil {
				t.Fatal(err)
			}
			if job.Duration != result.Duration {
				t.Errorf("wrong duration for job\ngot: %d \nwant: %d", job.Duration, result.Duration)
			}
		})
	}
}
