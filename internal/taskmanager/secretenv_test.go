// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/stretchr/testify/assert"
)

func writeSecretFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "secret")
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestResolveRetentionCredentials(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		fileBody   string
		wantAccess string
	}{
		{name: "config value is used when nothing is set", wantAccess: "from-config"},
		{name: "environment wins over config", envValue: "from-env", wantAccess: "from-env"},
		{name: "secret file wins over config", fileBody: "from-file\n", wantAccess: "from-file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(config.EnvRetentionS3AccessKey, tt.envValue)
			}
			if tt.fileBody != "" {
				t.Setenv(config.EnvRetentionS3AccessKey+util.EnvFileSuffix,
					writeSecretFile(t, tt.fileBody))
			}

			cfg := Retention{
				TargetKind:      "s3",
				TargetAccessKey: "from-config",
				TargetSecretKey: "secret-from-config",
			}
			assert.NoError(t, resolveRetentionCredentials(&cfg))
			assert.Equal(t, tt.wantAccess, cfg.TargetAccessKey)
			assert.Equal(t, "secret-from-config", cfg.TargetSecretKey)
		})
	}
}

// TestResolveRetentionCredentialsSkipsNonS3 keeps the file target free of any
// credential handling it has no use for.
func TestResolveRetentionCredentialsSkipsNonS3(t *testing.T) {
	t.Setenv(config.EnvRetentionS3AccessKey, "from-env")

	cfg := Retention{TargetKind: "file", TargetPath: "/tmp/archive"}
	assert.NoError(t, resolveRetentionCredentials(&cfg))
	assert.Empty(t, cfg.TargetAccessKey)
}

func TestResolveRetentionCredentialsRejectsUnreadableSecretFile(t *testing.T) {
	t.Setenv(config.EnvRetentionS3SecretKey+util.EnvFileSuffix,
		filepath.Join(t.TempDir(), "absent"))

	cfg := Retention{
		TargetKind:      "s3",
		TargetAccessKey: "from-config",
		TargetSecretKey: "secret-from-config",
	}
	err := resolveRetentionCredentials(&cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvRetentionS3SecretKey)
}

// TestRetentionCredentialsDoNotCrossFromArchive is the regression test for the
// credential-crossing hazard: the job archive and the retention target share
// the same S3 config shape and the same backend code, so only distinct
// environment names keep one from overriding the other.
func TestRetentionCredentialsDoNotCrossFromArchive(t *testing.T) {
	t.Setenv(config.EnvArchiveS3AccessKey, "archive-from-env")
	t.Setenv(config.EnvArchiveS3SecretKey, "archive-secret-from-env")

	cfg := Retention{
		TargetKind:      "s3",
		TargetAccessKey: "retention-configured",
		TargetSecretKey: "retention-secret",
	}
	assert.NoError(t, resolveRetentionCredentials(&cfg))

	assert.Equal(t, "retention-configured", cfg.TargetAccessKey,
		"the job archive's environment credentials must not reach the retention target")
	assert.Equal(t, "retention-secret", cfg.TargetSecretKey)
}

func TestResolveNodeStateCredentials(t *testing.T) {
	t.Setenv(config.EnvNodeStateS3AccessKey, "from-env")
	t.Setenv(config.EnvNodeStateS3SecretKey+util.EnvFileSuffix,
		writeSecretFile(t, "secret-from-file\n"))

	cfg := config.NodeStateRetention{
		TargetKind:      "s3",
		TargetAccessKey: "from-config",
		TargetSecretKey: "secret-from-config",
	}
	assert.NoError(t, resolveNodeStateCredentials(&cfg))
	assert.Equal(t, "from-env", cfg.TargetAccessKey)
	assert.Equal(t, "secret-from-file", cfg.TargetSecretKey)
}

// TestResolveNodeStateCredentialsLeavesGlobalUntouched guards the copy that
// RegisterNodeStateRetentionMoveService makes: resolved secrets must never be
// written back into config.Keys.
func TestResolveNodeStateCredentialsLeavesGlobalUntouched(t *testing.T) {
	t.Setenv(config.EnvNodeStateS3AccessKey, "from-env")

	previous := config.Keys.NodeStateRetention
	config.Keys.NodeStateRetention = &config.NodeStateRetention{
		TargetKind:      "s3",
		TargetAccessKey: "from-config",
		TargetSecretKey: "secret-from-config",
	}
	t.Cleanup(func() { config.Keys.NodeStateRetention = previous })

	cfg := *config.Keys.NodeStateRetention
	assert.NoError(t, resolveNodeStateCredentials(&cfg))

	assert.Equal(t, "from-env", cfg.TargetAccessKey)
	assert.Equal(t, "from-config", config.Keys.NodeStateRetention.TargetAccessKey,
		"the shared configuration must not receive the resolved secret")
}
