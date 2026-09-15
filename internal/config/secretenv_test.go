// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/stretchr/testify/assert"
)

// repoRoot is this package's directory relative to the repository root.
const repoRoot = "../.."

// legacyUnprefixedNames are the seven names that predate the CC_ convention.
// They are listed explicitly so that adding an eighth unprefixed cc-lib name
// fails the naming test rather than slipping through.
var legacyUnprefixedNames = map[string]bool{
	EnvJWTPublicKey:           true,
	EnvJWTPrivateKey:          true,
	EnvCrossLoginJWTPublicKey: true,
	EnvCrossLoginJWTHS512Key:  true,
	EnvLdapAdminPassword:      true,
	EnvOIDClientID:            true,
	EnvOIDClientSecret:        true,
}

func TestSecretEnvNamesUnique(t *testing.T) {
	seen := make(map[string]bool, len(SecretEnvs))
	for _, se := range SecretEnvs {
		assert.False(t, seen[se.Env], "duplicate registry entry for %s", se.Env)
		seen[se.Env] = true
	}
}

func TestSecretEnvNamingConvention(t *testing.T) {
	valid := regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

	for _, se := range SecretEnvs {
		assert.True(t, valid.MatchString(se.Env),
			"%s is not an upper snake case environment variable name", se.Env)

		// A name ending in _FILE would shadow another name's file variant.
		assert.False(t, strings.HasSuffix(se.Env, util.EnvFileSuffix),
			"%s must not end in %s", se.Env, util.EnvFileSuffix)

		assert.NotEmpty(t, se.Purpose, "%s has no purpose documented", se.Env)
		assert.NotEmpty(t, se.Docs, "%s lists no documentation files", se.Env)

		// The prefix says who resolves the name. cc-lib is linked into several
		// applications, so its names are namespaced; ours are not.
		if se.External {
			assert.True(t, strings.HasPrefix(se.Env, "CC_"),
				"%s is resolved in cc-lib and must carry the CC_ prefix", se.Env)
			continue
		}
		if legacyUnprefixedNames[se.Env] {
			continue
		}
		assert.False(t, strings.HasPrefix(se.Env, "CC_"),
			"%s is resolved here and must not use cc-lib's CC_ namespace", se.Env)
	}
}

// goSources returns every non-test Go file in the repository, keyed by
// repository-relative path.
func goSources(t *testing.T) map[string]string {
	t.Helper()

	sources := make(map[string]string)
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "var", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		// The registry itself is where the literals are allowed to live.
		if rel == filepath.Join("internal", "config", "secretenv.go") {
			return nil
		}

		buf, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources[rel] = string(buf)
		return nil
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, sources, "found no Go sources to scan")

	return sources
}

// constIdentifiers maps each locally resolved registry entry to the Go
// identifier that must appear at its call site.
func constIdentifiers() map[string]string {
	return map[string]string{
		EnvJWTPublicKey:           "EnvJWTPublicKey",
		EnvJWTPrivateKey:          "EnvJWTPrivateKey",
		EnvCrossLoginJWTPublicKey: "EnvCrossLoginJWTPublicKey",
		EnvCrossLoginJWTHS512Key:  "EnvCrossLoginJWTHS512Key",
		EnvLdapAdminPassword:      "EnvLdapAdminPassword",
		EnvOIDClientID:            "EnvOIDClientID",
		EnvOIDClientSecret:        "EnvOIDClientSecret",
		EnvArchiveS3AccessKey:     "EnvArchiveS3AccessKey",
		EnvArchiveS3SecretKey:     "EnvArchiveS3SecretKey",
		EnvRetentionS3AccessKey:   "EnvRetentionS3AccessKey",
		EnvRetentionS3SecretKey:   "EnvRetentionS3SecretKey",
		EnvNodeStateS3AccessKey:   "EnvNodeStateS3AccessKey",
		EnvNodeStateS3SecretKey:   "EnvNodeStateS3SecretKey",
		EnvMetricStoreToken:       "EnvMetricStoreToken",
		EnvArchiveMgrSrcAccessKey: "EnvArchiveMgrSrcAccessKey",
		EnvArchiveMgrSrcSecretKey: "EnvArchiveMgrSrcSecretKey",
		EnvArchiveMgrDstAccessKey: "EnvArchiveMgrDstAccessKey",
		EnvArchiveMgrDstSecretKey: "EnvArchiveMgrDstSecretKey",
	}
}

// TestSecretEnvConstantsAreUsed catches a name that is documented but never
// actually honored, which would be worse than not offering it at all.
func TestSecretEnvConstantsAreUsed(t *testing.T) {
	sources := goSources(t)
	idents := constIdentifiers()

	for _, se := range SecretEnvs {
		if se.External {
			continue
		}

		ident, ok := idents[se.Env]
		assert.True(t, ok, "%s has no constant identifier listed in this test", se.Env)
		if !ok {
			continue
		}

		used := false
		for _, src := range sources {
			if strings.Contains(src, "config."+ident) || strings.Contains(src, ident) {
				used = true
				break
			}
		}
		assert.True(t, used, "%s (config.%s) is registered but never referenced in code", se.Env, ident)
	}
}

// TestSecretEnvNoRawStringLiterals forces every call site through the
// constants, so a rename cannot leave a stale literal behind.
func TestSecretEnvNoRawStringLiterals(t *testing.T) {
	sources := goSources(t)

	for _, se := range SecretEnvs {
		literal := `"` + se.Env + `"`
		for path, src := range sources {
			assert.NotContains(t, src, literal,
				"%s contains the literal %s; use the constant from internal/config instead", path, literal)
		}
	}
}

// TestSecretEnvDocumented pairs with the test above: a name must be both wired
// and written down, in every file that claims to list it.
func TestSecretEnvDocumented(t *testing.T) {
	cache := make(map[string]string)

	for _, se := range SecretEnvs {
		for _, doc := range se.Docs {
			content, ok := cache[doc]
			if !ok {
				buf, err := os.ReadFile(filepath.Join(repoRoot, doc))
				assert.NoError(t, err, "cannot read documentation file %s", doc)
				if err != nil {
					continue
				}
				content = string(buf)
				cache[doc] = content
			}
			assert.Contains(t, content, se.Env, "%s does not document %s", doc, se.Env)
		}
	}
}
