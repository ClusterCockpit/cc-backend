// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

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

func TestResolveS3Credentials(t *testing.T) {
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
				t.Setenv(config.EnvArchiveS3AccessKey, tt.envValue)
			}
			if tt.fileBody != "" {
				t.Setenv(config.EnvArchiveS3AccessKey+util.EnvFileSuffix,
					writeSecretFile(t, tt.fileBody))
			}

			cfg := S3ArchiveConfig{AccessKey: "from-config", SecretKey: "secret-from-config"}
			assert.NoError(t, resolveS3Credentials(&cfg,
				config.EnvArchiveS3AccessKey, config.EnvArchiveS3SecretKey))
			assert.Equal(t, tt.wantAccess, cfg.AccessKey)
			assert.Equal(t, "secret-from-config", cfg.SecretKey)
		})
	}
}

// TestResolveS3CredentialsEmptyEnvNamesAreNoOp covers the call shape used by
// callers that have already resolved their own credentials.
func TestResolveS3CredentialsEmptyEnvNamesAreNoOp(t *testing.T) {
	t.Setenv(config.EnvArchiveS3AccessKey, "from-env")

	cfg := S3ArchiveConfig{AccessKey: "from-config", SecretKey: "secret-from-config"}
	assert.NoError(t, resolveS3Credentials(&cfg, "", ""))
	assert.Equal(t, "from-config", cfg.AccessKey)
	assert.Equal(t, "secret-from-config", cfg.SecretKey)
}

func TestResolveS3CredentialsRejectsUnreadableSecretFile(t *testing.T) {
	t.Setenv(config.EnvArchiveS3SecretKey+util.EnvFileSuffix,
		filepath.Join(t.TempDir(), "absent"))

	cfg := S3ArchiveConfig{AccessKey: "from-config", SecretKey: "secret-from-config"}
	err := resolveS3Credentials(&cfg,
		config.EnvArchiveS3AccessKey, config.EnvArchiveS3SecretKey)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvArchiveS3SecretKey)
}

// TestResolveS3CredentialsScopesDoNotCross is the regression test for the
// reason resolution does not live inside S3Archive.Init: that backend serves
// the job archive, both retention targets and archive-manager, so a fixed name
// there would let the job archive's credentials replace another set's.
func TestResolveS3CredentialsScopesDoNotCross(t *testing.T) {
	t.Setenv(config.EnvArchiveS3AccessKey, "archive-from-env")
	t.Setenv(config.EnvArchiveS3SecretKey, "archive-secret-from-env")

	// A retention target resolves under its own names, which are unset here.
	cfg := S3ArchiveConfig{AccessKey: "retention-configured", SecretKey: "retention-secret"}
	assert.NoError(t, resolveS3Credentials(&cfg,
		config.EnvRetentionS3AccessKey, config.EnvRetentionS3SecretKey))

	assert.Equal(t, "retention-configured", cfg.AccessKey,
		"the job archive's environment credentials must not reach the retention target")
	assert.Equal(t, "retention-secret", cfg.SecretKey)
}
