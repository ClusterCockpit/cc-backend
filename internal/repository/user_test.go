// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
)

func init() {
	Connect("sqlite3", "../../test/test.db")
}

func setupUserTest(t *testing.T) *UserCfgRepo {
	const testconfig = `{
	"addr":            "0.0.0.0:8080",
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
	} } ]
}`
	tmpdir := t.TempDir()
	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0666); err != nil {
		t.Fatal(err)
	}

	config.Init(cfgFilePath)
	return GetUserCfgRepo()
}
func TestGetUIConfig(t *testing.T) {
	r := setupUserTest(t)
	u := auth.User{Username: "jan"}

	cfg, err := r.GetUIConfig(&u)
	if err != nil {
		t.Fatal("No config")
	}

	tmp := cfg["plot_list_selectedMetrics"]
	metrics := tmp.([]interface{})

	str := metrics[2].(string)
	if str != "mem_bw" {
		t.Errorf("wrong config\ngot: %s \nwant: mem_bw", str)
	}
}
