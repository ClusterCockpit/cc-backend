// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
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

func setupRepo(t *testing.T) *repository.JobRepository {
	const testconfig = `{
	"addr":            "0.0.0.0:8080",
	"validate": false,
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
