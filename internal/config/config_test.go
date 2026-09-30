// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"testing"
	"time"

	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/resampler"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestInit(t *testing.T) {
	fp := "../../configs/config.json"
	ccconf.Init(fp)
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		Init(cfg)
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if Keys.Addr != "0.0.0.0:443" {
		t.Errorf("wrong addr\ngot: %s \nwant: 0.0.0.0:443", Keys.Addr)
	}
}

func TestInitMinimal(t *testing.T) {
	fp := "../../configs/config-demo.json"
	ccconf.Init(fp)
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		Init(cfg)
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if Keys.Addr != "127.0.0.1:8080" {
		t.Errorf("wrong addr\ngot: %s \nwant: 127.0.0.1:8080", Keys.Addr)
	}
}

// config-large.json carried the removed resampling keys (minimum-points,
// trigger, resolutions), which DisallowUnknownFields rejects.
func TestInitLarge(t *testing.T) {
	fp := "../../configs/config-large.json"
	ccconf.Init(fp)
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		Init(cfg)
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if Keys.EnableResampling == nil {
		t.Fatal("resampling config missing")
	}
	if Keys.EnableResampling.DefaultAlgo != "average" {
		t.Errorf("wrong default algo\ngot: %s \nwant: average", Keys.EnableResampling.DefaultAlgo)
	}
}

func TestTargetPointsForPolicy(t *testing.T) {
	tests := []struct {
		policy string
		want   int
	}{
		{"low", 200},
		{"medium", 500},
		{"high", 1000},
		{"unknown", 0},
		{"", 0},
	}

	for _, tt := range tests {
		if got := TargetPointsForPolicy(tt.policy); got != tt.want {
			t.Errorf("TargetPointsForPolicy(%q) = %d, want %d", tt.policy, got, tt.want)
		}
	}
}

// The resampler must be allowed to act exactly when a series exceeds the target
// point count. A mismatch here silently drops resample requests for a band of
// job durations.
func TestInitSyncsResamplerThreshold(t *testing.T) {
	for _, policy := range []string{"low", "medium", "high"} {
		Keys.EnableResampling = &ResampleConfig{DefaultPolicy: policy}
		initResampler()

		want := TargetPointsForPolicy(policy)
		if resampler.MinimumRequiredPoints != want {
			t.Errorf("policy %q: MinimumRequiredPoints = %d, want %d",
				policy, resampler.MinimumRequiredPoints, want)
		}
	}

	// Empty policy falls back to the documented default.
	Keys.EnableResampling = &ResampleConfig{}
	initResampler()
	if want := TargetPointsForPolicy(DefaultResamplePolicy); resampler.MinimumRequiredPoints != want {
		t.Errorf("empty policy: MinimumRequiredPoints = %d, want %d",
			resampler.MinimumRequiredPoints, want)
	}
}

// TestFleetConfigFromExample asserts that the shipped example config parses into
// the fleet block, which also proves the JSON schema accepts every key.
func TestFleetConfigFromExample(t *testing.T) {
	fp := "../../configs/config.json"
	ccconf.Init(fp)
	cfg := ccconf.GetPackageConfig("main")
	if cfg == nil {
		cclog.Abort("Main configuration must be present")
	}
	Init(cfg)

	if Keys.Fleet == nil {
		t.Fatal("fleet config missing")
	}
	if Keys.Fleet.ConfigDir != "./var/fleet-config" {
		t.Errorf("wrong config-dir\ngot: %s \nwant: ./var/fleet-config", Keys.Fleet.ConfigDir)
	}
	if Keys.Fleet.HeartbeatSubject != "cc.fleet.event" {
		t.Errorf("wrong heartbeat-subject\ngot: %s \nwant: cc.fleet.event", Keys.Fleet.HeartbeatSubject)
	}
	if got := Keys.Fleet.StaleAfterDuration(); got != 90*time.Second {
		t.Errorf("wrong stale-after\ngot: %s \nwant: 1m30s", got)
	}
	if got := Keys.Fleet.DiscoveryIntervalDuration(); got != time.Minute {
		t.Errorf("wrong discovery-interval\ngot: %s \nwant: 1m0s", got)
	}
	if got := Keys.Fleet.HeartbeatWorkers(); got != 2 {
		t.Errorf("wrong heartbeat-concurrency\ngot: %d \nwant: 2", got)
	}
}

func TestFleetDurationDefaults(t *testing.T) {
	// Empty and malformed values must fall back instead of yielding a zero
	// interval, which would spin a ticker.
	for _, raw := range []string{"", "90 seconds", "nonsense"} {
		c := &FleetConfig{
			StaleAfter:           raw,
			SweepInterval:        raw,
			ConfigReloadInterval: raw,
			DiscoveryInterval:    raw,
		}
		if got := c.StaleAfterDuration(); got != DefaultFleetStaleAfter {
			t.Errorf("stale-after %q: got %s, want %s", raw, got, DefaultFleetStaleAfter)
		}
		if got := c.SweepIntervalDuration(); got != DefaultFleetSweepInterval {
			t.Errorf("sweep-interval %q: got %s, want %s", raw, got, DefaultFleetSweepInterval)
		}
		if got := c.ConfigReloadIntervalDuration(); got != DefaultFleetConfigReloadInterval {
			t.Errorf("config-reload-interval %q: got %s, want %s", raw, got, DefaultFleetConfigReloadInterval)
		}
		if got := c.DiscoveryIntervalDuration(); got != DefaultFleetDiscoveryInterval {
			t.Errorf("discovery-interval %q: got %s, want %s", raw, got, DefaultFleetDiscoveryInterval)
		}
	}

	c := &FleetConfig{}
	if got := c.HeartbeatWorkers(); got != DefaultFleetHeartbeatConcurrency {
		t.Errorf("heartbeat-concurrency: got %d, want %d", got, DefaultFleetHeartbeatConcurrency)
	}
	if got := (&FleetConfig{HeartbeatConcurrency: -1}).HeartbeatWorkers(); got != DefaultFleetHeartbeatConcurrency {
		t.Errorf("negative heartbeat-concurrency: got %d, want %d", got, DefaultFleetHeartbeatConcurrency)
	}
}

// TestFleetSchemaRejectsInvalidValues compiles the schema directly: Validate
// aborts the process on failure, so rejection cannot be exercised via Init.
func TestFleetSchemaRejectsInvalidValues(t *testing.T) {
	sch, err := jsonschema.CompileString("schema.json", configSchema)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		config string
		valid  bool
	}{
		{"complete block", `{"fleet":{"config-dir":"./var/fleet-config","stale-after":"90s","discovery-interval":"1m30s","heartbeat-concurrency":4}}`, true},
		{"only the required key", `{"fleet":{"config-dir":"./var/fleet-config"}}`, true},
		{"missing config-dir", `{"fleet":{"stale-after":"90s"}}`, false},
		{"duration as a number", `{"fleet":{"config-dir":"./x","stale-after":90}}`, false},
		{"unparseable duration", `{"fleet":{"config-dir":"./x","stale-after":"90 seconds"}}`, false},
		{"concurrency as a string", `{"fleet":{"config-dir":"./x","heartbeat-concurrency":"two"}}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tt.config), &v); err != nil {
				t.Fatal(err)
			}
			err := sch.Validate(v)
			if tt.valid && err != nil {
				t.Fatalf("expected the config to validate, got: %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("expected the config to be rejected")
			}
		})
	}
}
