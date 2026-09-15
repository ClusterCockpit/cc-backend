// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/stretchr/testify/assert"
)

func TestMetricStoreTokenEnv(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		want  string
	}{
		{name: "cluster name", scope: "fritz", want: "METRICSTORE_TOKEN_FRITZ"},
		{name: "dashes become underscores", scope: "fritz-spr1tb", want: "METRICSTORE_TOKEN_FRITZ_SPR1TB"},
		{name: "dots become underscores", scope: "a.b", want: "METRICSTORE_TOKEN_A_B"},
		{name: "wildcard has no scope specific name", scope: "*", want: ""},
		{name: "empty scope has no scope specific name", scope: "", want: ""},
		// METRICSTORE_TOKEN_FILE is the generic name's own file variant, so a
		// scope that sanitizes to FILE must not claim it.
		{name: "a scope colliding with the file variant is refused", scope: "file", want: ""},
		{name: "collision check is case insensitive", scope: "FILE", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, metricStoreTokenEnv(tt.scope))
		})
	}
}

func TestResolveStoreToken(t *testing.T) {
	t.Run("config value is used when nothing is set", func(t *testing.T) {
		token, err := resolveStoreToken(CCMetricStoreConfig{Scope: "fritz", Token: "from-config"})
		assert.NoError(t, err)
		assert.Equal(t, "from-config", token)
	})

	t.Run("generic name wins over config", func(t *testing.T) {
		t.Setenv(config.EnvMetricStoreToken, "from-generic")

		token, err := resolveStoreToken(CCMetricStoreConfig{Scope: "fritz", Token: "from-config"})
		assert.NoError(t, err)
		assert.Equal(t, "from-generic", token)
	})

	t.Run("scope specific name wins over generic", func(t *testing.T) {
		t.Setenv(config.EnvMetricStoreToken, "from-generic")
		t.Setenv("METRICSTORE_TOKEN_FRITZ", "from-scope")

		token, err := resolveStoreToken(CCMetricStoreConfig{Scope: "fritz", Token: "from-config"})
		assert.NoError(t, err)
		assert.Equal(t, "from-scope", token)
	})

	t.Run("one scope does not read another scope's token", func(t *testing.T) {
		t.Setenv("METRICSTORE_TOKEN_FRITZ", "fritz-token")

		token, err := resolveStoreToken(CCMetricStoreConfig{Scope: "alex", Token: "alex-config"})
		assert.NoError(t, err)
		assert.Equal(t, "alex-config", token)
	})

	t.Run("secret file is read", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		assert.NoError(t, os.WriteFile(path, []byte("from-file\n"), 0o600))
		t.Setenv("METRICSTORE_TOKEN_FRITZ"+util.EnvFileSuffix, path)

		token, err := resolveStoreToken(CCMetricStoreConfig{Scope: "fritz", Token: "from-config"})
		assert.NoError(t, err)
		assert.Equal(t, "from-file", token)
	})

	t.Run("an unreadable secret file is an error", func(t *testing.T) {
		t.Setenv(config.EnvMetricStoreToken+util.EnvFileSuffix, filepath.Join(t.TempDir(), "absent"))

		_, err := resolveStoreToken(CCMetricStoreConfig{Scope: "fritz", Token: "from-config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), config.EnvMetricStoreToken)
	})
}

// TestConfigSchemaAcceptsMissingToken covers the schema change that makes an
// environment-only deployment possible: config.Validate aborts the process, so
// a still-required token would make it unusable.
func TestConfigSchemaAcceptsMissingToken(t *testing.T) {
	raw := json.RawMessage(`[{"scope":"*","url":"http://metricstore:8082"}]`)
	assert.NotPanics(t, func() { config.Validate(configSchema, raw) })
}
