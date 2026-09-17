// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/stretchr/testify/assert"
)

// b64 encodes a marker string so it survives the base64 decoding the JWT
// authenticators perform, letting a test assert which source won.
func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// writeSecretFile writes content to a temporary file and returns its path.
func writeSecretFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "secret")
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// setJwtConfig installs a JWT configuration for the duration of the test.
func setJwtConfig(t *testing.T, cfg *JWTAuthConfig) {
	t.Helper()

	previous := Keys.JwtConfig
	Keys.JwtConfig = cfg
	t.Cleanup(func() { Keys.JwtConfig = previous })
}

func TestJWTAuthenticatorInitSecretSources(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fileBody string
		want     string
	}{
		{name: "config value is used when nothing is set", want: "from-config"},
		{name: "environment wins over config", envValue: b64("from-env"), want: "from-env"},
		{name: "secret file wins over config", fileBody: b64("from-file") + "\n", want: "from-file"},
		{
			name:     "environment wins over secret file",
			envValue: b64("from-env"), fileBody: b64("from-file"),
			want: "from-env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setJwtConfig(t, &JWTAuthConfig{
				PublicKey:  b64("from-config"),
				PrivateKey: b64("private-from-config"),
			})

			if tt.envValue != "" {
				t.Setenv(config.EnvJWTPublicKey, tt.envValue)
			}
			if tt.fileBody != "" {
				t.Setenv(config.EnvJWTPublicKey+util.EnvFileSuffix, writeSecretFile(t, tt.fileBody))
			}

			ja := &JWTAuthenticator{}
			assert.NoError(t, ja.Init())
			assert.Equal(t, tt.want, string(ja.publicKey))
		})
	}
}

func TestJWTAuthenticatorInitRejectsUnreadableSecretFile(t *testing.T) {
	setJwtConfig(t, &JWTAuthConfig{
		PublicKey:  b64("from-config"),
		PrivateKey: b64("private-from-config"),
	})
	t.Setenv(config.EnvJWTPrivateKey+util.EnvFileSuffix, filepath.Join(t.TempDir(), "absent"))

	ja := &JWTAuthenticator{}
	err := ja.Init()

	// Falling back to the configured key here would start the server with a
	// credential the operator has already replaced.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvJWTPrivateKey)
}

func TestJWTSessionAuthenticatorInitSecretSources(t *testing.T) {
	setJwtConfig(t, &JWTAuthConfig{CrossLoginHS512Key: b64("from-config")})
	t.Setenv(config.EnvCrossLoginJWTHS512Key+util.EnvFileSuffix,
		writeSecretFile(t, b64("from-file")+"\n"))

	ja := &JWTSessionAuthenticator{}
	assert.NoError(t, ja.Init())
	assert.Equal(t, "from-file", string(ja.loginTokenKey))
}

func TestJWTCookieSessionAuthenticatorInitSecretSources(t *testing.T) {
	setJwtConfig(t, &JWTAuthConfig{
		// Init requires a cookie name and a trusted issuer before it accepts
		// the configuration at all; neither is what this test is about.
		CookieName:          "jwt",
		TrustedIssuer:       "https://issuer.example.com/",
		PublicKey:           b64("pub-from-config"),
		PrivateKey:          b64("priv-from-config"),
		CrossLoginPublicKey: b64("cross-from-config"),
	})
	t.Setenv(config.EnvCrossLoginJWTPublicKey, b64("cross-from-env"))

	ja := &JWTCookieSessionAuthenticator{}
	assert.NoError(t, ja.Init())
	assert.Equal(t, "pub-from-config", string(ja.publicKey))
	assert.Equal(t, "cross-from-env", string(ja.publicKeyCrossLogin))
}

func TestLdapAuthenticatorInitSecretSources(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fileBody string
		want     string
	}{
		{name: "config value is used when nothing is set", want: "from-config"},
		{name: "environment wins over config", envValue: "from-env", want: "from-env"},
		{name: "secret file wins over config", fileBody: "from-file\n", want: "from-file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := Keys.LdapConfig
			Keys.LdapConfig = &LdapConfig{SyncPassword: "from-config"}
			t.Cleanup(func() { Keys.LdapConfig = previous })

			if tt.envValue != "" {
				t.Setenv(config.EnvLdapAdminPassword, tt.envValue)
			}
			if tt.fileBody != "" {
				t.Setenv(config.EnvLdapAdminPassword+util.EnvFileSuffix, writeSecretFile(t, tt.fileBody))
			}

			la := &LdapAuthenticator{}
			assert.NoError(t, la.Init())
			assert.Equal(t, tt.want, la.syncPassword)
		})
	}
}

func TestLdapAuthenticatorInitRejectsUnreadableSecretFile(t *testing.T) {
	previous := Keys.LdapConfig
	Keys.LdapConfig = &LdapConfig{SyncPassword: "from-config"}
	t.Cleanup(func() { Keys.LdapConfig = previous })

	t.Setenv(config.EnvLdapAdminPassword+util.EnvFileSuffix, filepath.Join(t.TempDir(), "absent"))

	la := &LdapAuthenticator{}
	err := la.Init()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvLdapAdminPassword)
	assert.Empty(t, la.syncPassword, "the configured password must not be used as a fallback")
}
