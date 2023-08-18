// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
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
	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

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

func setup(t *testing.T) *repository.JobRepository {
	const testconfig = `{
	"addr":            "0.0.0.0:8080",
	"validate": false,
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	},
    "jwts": {
        "max-age": "2m"
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

	log.Init("info", true)
	tmpdir := t.TempDir()

	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobarchive, "version.txt"), []byte(fmt.Sprintf("%d", 1)), 0666); err != nil {
		t.Fatal(err)
	}
	fritzArchive := filepath.Join(tmpdir, "job-archive", "fritz")
	if err := os.Mkdir(fritzArchive, 0777); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join("testdata", "cluster-fritz.json"),
		filepath.Join(fritzArchive, "cluster.json")); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	err := repository.MigrateDB("sqlite3", dbfilepath)
	if err != nil {
		t.Fatal(err)
	}

	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0666); err != nil {
		t.Fatal(err)
	}

	config.Init(cfgFilePath)
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	if err := archive.Init(json.RawMessage(archiveCfg), config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	repository.Connect("sqlite3", dbfilepath)
	return repository.GetJobRepository()
}

type Result struct {
	JobId     int64
	Cluster   string
	StartTime int64
	Duration  int32
}

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
			job, err := r.Find(&result.JobId, &result.Cluster, &result.StartTime)
			if err != nil {
				t.Fatal(err)
			}
			if job.Duration != result.Duration {
				t.Errorf("wrong duration for job\ngot: %d \nwant: %d", job.Duration, result.Duration)
			}
		})
	}
}
