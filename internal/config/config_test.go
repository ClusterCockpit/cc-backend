// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"

	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/resampler"
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
