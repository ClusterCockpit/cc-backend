// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package configexample guards the shipped example configuration files in
// ./configs against drift: every section must still be accepted by the code
// that parses it at startup. The test lives in its own package because it has
// to import every config-consuming package at once.
package configexample

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/taskmanager"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	"github.com/ClusterCockpit/cc-backend/web"
	ccschema "github.com/ClusterCockpit/cc-lib/v2/schema"
)

const configDir = "../../configs"

// knownSections mirrors the list in cmd/cc-backend/main.go.
var knownSections = map[string]bool{
	"main": true, "auth": true, "nats": true, "archive": true,
	"metric-store": true, "metric-store-external": true, "cron": true, "ui": true,
}

// archiveConfig covers the union of all archive backend options plus the
// retention/compression keys consumed by the task manager.
type archiveConfig struct {
	Kind         string                `json:"kind"`
	Path         string                `json:"path"`
	DBPath       string                `json:"db-path"`
	Endpoint     string                `json:"endpoint"`
	Bucket       string                `json:"bucket"`
	Region       string                `json:"region"`
	UsePathStyle bool                  `json:"use-path-style"`
	AccessKey    string                `json:"access-key"`
	SecretKey    string                `json:"secret-key"`
	Retention    taskmanager.Retention `json:"retention"`
	Compression  int                   `json:"compression"`
}

func decodeStrict(t *testing.T, name string, raw json.RawMessage, target any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		t.Errorf("%s: %v", name, err)
	}
}

func TestExampleConfigs(t *testing.T) {
	for _, name := range []string{"config.json", "config-demo.json", "config-large.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(configDir, name))
			if err != nil {
				t.Fatal(err)
			}
			var sections map[string]json.RawMessage
			if err := json.Unmarshal(raw, &sections); err != nil {
				t.Fatal(err)
			}

			for key, value := range sections {
				// ccConfig resolves a "<name>-file" key by loading the
				// referenced file and storing it under "<name>".
				section := key
				if base, ok := strings.CutSuffix(key, "-file"); ok {
					section = base
					var path string
					if err := json.Unmarshal(value, &path); err != nil {
						t.Errorf("%s: value of %q is not a file path: %v", name, key, err)
						continue
					}
					b, err := os.ReadFile(filepath.Join(configDir, path))
					if err != nil {
						t.Errorf("%s: %q references %s: %v", name, key, path, err)
						continue
					}
					value = json.RawMessage(b)
				}

				if !knownSections[section] {
					t.Errorf("%s: unknown top-level section %q", name, key)
					continue
				}

				switch section {
				case "main":
					var cfg config.ProgramConfig
					decodeStrict(t, key, value, &cfg)
				case "cron":
					var cfg taskmanager.CronFrequency
					decodeStrict(t, key, value, &cfg)
				case "archive":
					var cfg archiveConfig
					decodeStrict(t, key, value, &cfg)
				case "metric-store":
					var cfg metricstore.MetricStoreConfig
					decodeStrict(t, key, value, &cfg)
				case "metric-store-external":
					var cfg []metricdispatch.CCMetricStoreConfig
					decodeStrict(t, key, value, &cfg)
				case "ui":
					var cfg web.WebConfig
					decodeStrict(t, key, value, &cfg)
				}
			}
		})
	}
}

func TestExampleClusterConfig(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(configDir, "cluster.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ccschema.Validate(ccschema.ClusterCfg, bytes.NewReader(raw)); err != nil {
		t.Errorf("cluster.json does not validate: %v", err)
	}
	var cluster ccschema.Cluster
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cluster); err != nil {
		t.Errorf("cluster.json: %v", err)
	}
}

// TestArchiveTestdataClusterConfigs keeps the cluster.json fixtures valid
// against the cc-lib schema. They are copied into real deployments, and an
// invalid one only fails at runtime with main.validate enabled.
func TestArchiveTestdataClusterConfigs(t *testing.T) {
	for _, path := range []string{
		"../../pkg/archive/testdata/archive/alex/cluster.json",
		"../../pkg/archive/testdata/archive/fritz/cluster.json",
		"../../pkg/archive/testdata/archive/emmy/cluster.json",
		"../../internal/importer/testdata/cluster-fritz.json",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ccschema.Validate(ccschema.ClusterCfg, bytes.NewReader(raw)); err != nil {
			t.Errorf("%s does not validate: %v", path, err)
		}
	}
}
