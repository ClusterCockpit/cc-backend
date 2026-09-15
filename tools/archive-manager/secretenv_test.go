// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/stretchr/testify/assert"
)

// TestSourceConfigAcceptsBothKeySpellings covers the spelling split this tool
// used to have: --convert parsed camelCase while --import went through the
// archive backend and read kebab-case.
func TestSourceConfigAcceptsBothKeySpellings(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "kebab case",
			raw:  `{"kind":"s3","bucket":"b","access-key":"AK","secret-key":"SK","use-path-style":true}`,
		},
		{
			name: "camel case",
			raw:  `{"kind":"s3","bucket":"b","accessKey":"AK","secretKey":"SK","usePathStyle":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg sourceConfig
			assert.NoError(t, json.Unmarshal([]byte(tt.raw), &cfg))
			assert.Equal(t, "AK", cfg.AccessKey)
			assert.Equal(t, "SK", cfg.SecretKey)
			assert.True(t, cfg.UsePathStyle)
			assert.Equal(t, "b", cfg.Bucket)
		})
	}
}

func TestResolveSourceCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	assert.NoError(t, os.WriteFile(path, []byte("dst-secret-from-file\n"), 0o600))

	t.Setenv(config.EnvArchiveMgrSrcAccessKey, "src-from-env")
	t.Setenv(config.EnvArchiveMgrDstSecretKey+util.EnvFileSuffix, path)

	src := sourceConfig{Kind: "s3", AccessKey: "src-configured", SecretKey: "src-secret"}
	assert.NoError(t, resolveSourceCredentials(&src,
		config.EnvArchiveMgrSrcAccessKey, config.EnvArchiveMgrSrcSecretKey))
	assert.Equal(t, "src-from-env", src.AccessKey)
	assert.Equal(t, "src-secret", src.SecretKey)

	dst := sourceConfig{Kind: "s3", AccessKey: "dst-configured", SecretKey: "dst-secret"}
	assert.NoError(t, resolveSourceCredentials(&dst,
		config.EnvArchiveMgrDstAccessKey, config.EnvArchiveMgrDstSecretKey))
	assert.Equal(t, "dst-secret-from-file", dst.SecretKey)

	// The source's environment credentials must not reach the destination.
	assert.Equal(t, "dst-configured", dst.AccessKey)
}

func TestResolveSourceCredentialsSkipsNonS3(t *testing.T) {
	t.Setenv(config.EnvArchiveMgrSrcAccessKey, "src-from-env")

	cfg := sourceConfig{Kind: "file", Path: "./var/job-archive"}
	assert.NoError(t, resolveSourceCredentials(&cfg,
		config.EnvArchiveMgrSrcAccessKey, config.EnvArchiveMgrSrcSecretKey))
	assert.Empty(t, cfg.AccessKey)
}

func TestResolveSourceCredentialsRejectsUnreadableSecretFile(t *testing.T) {
	t.Setenv(config.EnvArchiveMgrSrcSecretKey+util.EnvFileSuffix,
		filepath.Join(t.TempDir(), "absent"))

	cfg := sourceConfig{Kind: "s3", AccessKey: "configured", SecretKey: "secret"}
	err := resolveSourceCredentials(&cfg,
		config.EnvArchiveMgrSrcAccessKey, config.EnvArchiveMgrSrcSecretKey)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvArchiveMgrSrcSecretKey)
}
