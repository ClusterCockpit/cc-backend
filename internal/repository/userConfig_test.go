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
	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	_ "github.com/mattn/go-sqlite3"
)

func setupUserTest(t *testing.T) *UserCfgRepo {
	const testconfig = `{
	"main": {
	 "addr":   "0.0.0.0:8080",
   "api-allowed-ips": [
     "*"
   ]
  },
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	}
}`

	cclog.Init("info", true)

	// Copy test DB to a temp file for test isolation
	srcData, err := os.ReadFile("testdata/job.db")
	if err != nil {
		t.Fatal(err)
	}
	dbfilepath := filepath.Join(t.TempDir(), "job.db")
	if err := os.WriteFile(dbfilepath, srcData, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ResetConnection(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ResetConnection()
	})

	err = MigrateDB(dbfilepath)
	if err != nil {
		t.Fatal(err)
	}
	Connect(dbfilepath)

	tmpdir := t.TempDir()
	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0o666); err != nil {
		t.Fatal(err)
	}

	ccconf.Init(cfgFilePath)

	// Load and check main configuration
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		config.Init(cfg)
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

	_, exists := cfg["metricConfig_jobListMetrics:fritz"]
	if !exists {
		t.Fatal("Key metricConfig_jobListMetrics is missing")
	}
}
