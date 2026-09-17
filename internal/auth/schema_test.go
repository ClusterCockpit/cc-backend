// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// compileConfigSchema guards against the schema silently degrading into a
// document with no constraints (which is what happens when the "type"/
// "properties" wrapper is missing: every instance validates).
func compileConfigSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	sch, err := jsonschema.CompileString("schema.json", configSchema)
	if err != nil {
		t.Fatalf("compiling auth config schema: %v", err)
	}
	return sch
}

func validateAuthConfig(t *testing.T, sch *jsonschema.Schema, raw string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("invalid test json: %v", err)
	}
	return sch.Validate(v)
}

func TestConfigSchemaAccepts(t *testing.T) {
	sch := compileConfigSchema(t)

	tests := map[string]string{
		"jwts only":    `{"jwts":{"max-age":"2000h"}}`,
		"oidc only":    `{"oidc":{"provider":"http://localhost:8080/realms/cc"}}`,
		"empty":        `{}`,
		"ldap":         `{"ldap":{"url":"ldap://127.0.0.1:389","user-base":"ou=users,dc=example,dc=com","search-dn":"cn=admin,dc=example,dc=com","user-bind":"uid={username},ou=users,dc=example,dc=com","user-filter":"(objectclass=posixAccount)","role-filters":{"admin":"(memberOf=cn=cc-admin,ou=groups,dc=example,dc=com)"}}}`,
		"role-mapping": `{"oidc":{"provider":"http://p","role-mapping":{"cc-admin":"admin"}}}`,
	}

	for name, raw := range tests {
		if err := validateAuthConfig(t, sch, raw); err != nil {
			t.Errorf("%s: unexpected validation error: %v", name, err)
		}
	}
}

func TestConfigSchemaRejects(t *testing.T) {
	sch := compileConfigSchema(t)

	tests := map[string]string{
		"jwts without max-age":      `{"jwts":{"public-key":"abc"}}`,
		"jwts max-age not a string": `{"jwts":{"max-age":2000}}`,
		"oidc without provider":     `{"oidc":{"client-id":"cc-backend"}}`,
		"ldap without user-bind":    `{"ldap":{"url":"ldap://127.0.0.1:389","user-base":"ou=users","search-dn":"cn=admin","user-filter":"(objectclass=posixAccount)"}}`,
		"role-filters not strings":  `{"ldap":{"url":"u","user-base":"b","search-dn":"d","user-bind":"ub","user-filter":"f","role-filters":{"admin":true}}}`,
		"jwts not an object":        `{"jwts":"2000h"}`,
	}

	for name, raw := range tests {
		if err := validateAuthConfig(t, sch, raw); err == nil {
			t.Errorf("%s: expected validation error, got none", name)
		}
	}
}

// TestConfigSchemaExampleConfigs validates the auth sections of the shipped
// example configurations.
func TestConfigSchemaExampleConfigs(t *testing.T) {
	sch := compileConfigSchema(t)

	for _, path := range []string{
		"../../configs/config.json",
		"../../configs/config-demo.json",
		"../../configs/config-large.json",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var sections map[string]json.RawMessage
		if err := json.Unmarshal(raw, &sections); err != nil {
			t.Fatal(err)
		}
		authCfg, ok := sections["auth"]
		if !ok {
			continue
		}
		var v any
		if err := json.Unmarshal(authCfg, &v); err != nil {
			t.Fatal(err)
		}
		if err := sch.Validate(v); err != nil {
			t.Errorf("%s: auth section does not validate: %v", path, err)
		}
	}
}
