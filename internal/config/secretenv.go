// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import "github.com/ClusterCockpit/cc-lib/v2/nats"

// Environment variables that override the corresponding secret in
// config.json. Each also accepts a "_FILE" variant naming a file that holds
// the value, so a secret can be supplied from a Docker or Kubernetes secret
// mount or from systemd LoadCredential without ever being written to
// config.json; see util.SecretFromEnv for the precedence rules.
//
// These are the names cc-backend itself resolves, and they are unprefixed for
// consistency with the seven auth names that already existed. Names resolved
// inside cc-lib carry a CC_ prefix instead, because cc-lib is linked into
// several applications whose environments it must not silently claim names in;
// those are referenced from their owning cc-lib package rather than redefined
// here, so that each variable has exactly one definition.
const (
	EnvJWTPublicKey           = "JWT_PUBLIC_KEY"
	EnvJWTPrivateKey          = "JWT_PRIVATE_KEY"
	EnvCrossLoginJWTPublicKey = "CROSS_LOGIN_JWT_PUBLIC_KEY"
	EnvCrossLoginJWTHS512Key  = "CROSS_LOGIN_JWT_HS512_KEY"
	EnvLdapAdminPassword      = "LDAP_ADMIN_PASSWORD"
	EnvOIDClientID            = "OID_CLIENT_ID"
	EnvOIDClientSecret        = "OID_CLIENT_SECRET"

	EnvArchiveS3AccessKey   = "ARCHIVE_S3_ACCESS_KEY"
	EnvArchiveS3SecretKey   = "ARCHIVE_S3_SECRET_KEY"
	EnvRetentionS3AccessKey = "RETENTION_S3_ACCESS_KEY"
	EnvRetentionS3SecretKey = "RETENTION_S3_SECRET_KEY"
	EnvNodeStateS3AccessKey = "NODESTATE_S3_ACCESS_KEY"
	EnvNodeStateS3SecretKey = "NODESTATE_S3_SECRET_KEY"

	// EnvMetricStoreToken is the fallback for every entry of the
	// metric-store-external array. Because that section is an array, one entry
	// per scope, a single name cannot address one particular entry: the
	// scope-specific name returned by metricdispatch takes precedence over
	// this one.
	EnvMetricStoreToken = "METRICSTORE_TOKEN"

	EnvArchiveMgrSrcAccessKey = "ARCHIVE_MANAGER_SRC_S3_ACCESS_KEY"
	EnvArchiveMgrSrcSecretKey = "ARCHIVE_MANAGER_SRC_S3_SECRET_KEY"
	EnvArchiveMgrDstAccessKey = "ARCHIVE_MANAGER_DST_S3_ACCESS_KEY"
	EnvArchiveMgrDstSecretKey = "ARCHIVE_MANAGER_DST_S3_SECRET_KEY"
)

// SecretEnv documents one env-overridable secret.
//
// Docs lists the files that must mention Env; External marks a name that is
// resolved inside cc-lib rather than here. The guard tests in
// secretenv_test.go enforce both, so a name cannot be documented without being
// wired, nor wired without being documented.
type SecretEnv struct {
	// Env is the environment variable name.
	Env string
	// ConfigPath is the dotted path of the config.json key it overrides, or
	// empty for a secret that only a command line tool reads.
	ConfigPath string
	// Purpose is a short description used in error messages and docs review.
	Purpose string
	// Docs lists files that must mention Env.
	Docs []string
	// External is set when the name is resolved inside cc-lib, which exempts
	// it from the "constant is referenced in cc-backend code" check.
	External bool
}

// docsCommon are the files that must document every secret.
var docsCommon = []string{"README.md", "CLAUDE.md"}

func docs(extra ...string) []string {
	return append(append([]string{}, docsCommon...), extra...)
}

// SecretEnvs is the registry of every secret cc-backend can take from the
// environment, including the ones cc-lib resolves on its behalf.
var SecretEnvs = []SecretEnv{
	{
		Env: EnvJWTPublicKey, ConfigPath: "auth.jwts.public-key",
		Purpose: "Ed25519 public key used to validate JWTs",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvJWTPrivateKey, ConfigPath: "auth.jwts.private-key",
		Purpose: "Ed25519 private key used to sign JWTs",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvCrossLoginJWTPublicKey, ConfigPath: "auth.jwts.cross-login-public-key",
		Purpose: "Ed25519 public key for externally generated JWTs",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvCrossLoginJWTHS512Key, ConfigPath: "auth.jwts.cross-login-hs512-key",
		Purpose: "HS512 key for cross-login JWT sessions",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvLdapAdminPassword, ConfigPath: "auth.ldap.sync-password",
		Purpose: "password of the LDAP account used for user sync",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvOIDClientID, ConfigPath: "auth.oidc.client-id",
		Purpose: "OIDC client id",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvOIDClientSecret, ConfigPath: "auth.oidc.client-secret",
		Purpose: "OIDC client secret",
		Docs:    docs("internal/auth/schema.go"),
	},
	{
		Env: EnvArchiveS3AccessKey, ConfigPath: "archive.access-key",
		Purpose: "S3 access key of the job archive",
		Docs:    docs("pkg/archive/ConfigSchema.go"),
	},
	{
		Env: EnvArchiveS3SecretKey, ConfigPath: "archive.secret-key",
		Purpose: "S3 secret key of the job archive",
		Docs:    docs("pkg/archive/ConfigSchema.go"),
	},
	{
		Env: EnvRetentionS3AccessKey, ConfigPath: "archive.retention.target-access-key",
		Purpose: "S3 access key of the job retention target",
		Docs:    docs("pkg/archive/ConfigSchema.go"),
	},
	{
		Env: EnvRetentionS3SecretKey, ConfigPath: "archive.retention.target-secret-key",
		Purpose: "S3 secret key of the job retention target",
		Docs:    docs("pkg/archive/ConfigSchema.go"),
	},
	{
		Env: EnvNodeStateS3AccessKey, ConfigPath: "main.nodestate-retention.target-access-key",
		Purpose: "S3 access key of the node state retention target",
		Docs:    docs("internal/config/schema.go"),
	},
	{
		Env: EnvNodeStateS3SecretKey, ConfigPath: "main.nodestate-retention.target-secret-key",
		Purpose: "S3 secret key of the node state retention target",
		Docs:    docs("internal/config/schema.go"),
	},
	{
		Env: EnvMetricStoreToken, ConfigPath: "metric-store-external[].token",
		Purpose: "authentication token of an external metric store",
		Docs:    docs("internal/metricdispatch/configSchema.go"),
	},
	{
		Env:     EnvArchiveMgrSrcAccessKey,
		Purpose: "S3 access key of the archive-manager source archive",
		Docs:    docs("tools/archive-manager/README.md"),
	},
	{
		Env:     EnvArchiveMgrSrcSecretKey,
		Purpose: "S3 secret key of the archive-manager source archive",
		Docs:    docs("tools/archive-manager/README.md"),
	},
	{
		Env:     EnvArchiveMgrDstAccessKey,
		Purpose: "S3 access key of the archive-manager target archive",
		Docs:    docs("tools/archive-manager/README.md"),
	},
	{
		Env:     EnvArchiveMgrDstSecretKey,
		Purpose: "S3 secret key of the archive-manager target archive",
		Docs:    docs("tools/archive-manager/README.md"),
	},
	{
		Env: nats.EnvUsername, ConfigPath: "nats.username",
		Purpose:  "NATS username",
		Docs:     docsCommon,
		External: true,
	},
	{
		Env: nats.EnvPassword, ConfigPath: "nats.password",
		Purpose:  "NATS password",
		Docs:     docsCommon,
		External: true,
	},
}
