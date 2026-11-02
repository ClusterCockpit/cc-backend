// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"slices"
	"testing"
)

func TestOIDCCallbackIgnoresForeignClientRoles(t *testing.T) {
	for _, tt := range []struct {
		name        string
		clients     map[string][]string
		realm, want []string
	}{
		{"foreign admin", map[string][]string{"audit-other-client": {"admin"}}, nil, []string{"user"}},
		{"own admin", map[string][]string{"audit-client": {"admin"}}, nil, []string{"admin"}},
		{"own support ignores foreign admin", map[string][]string{"audit-client": {"support"}, "audit-other-client": {"admin"}}, nil, []string{"support"}},
		{"realm manager retained", nil, []string{"manager"}, []string{"manager"}},
		{"unknown own role defaults to user", map[string][]string{"audit-client": {"unrecognized"}}, nil, []string{"user"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, response := runSecurityOIDCCallback(t, "audit-signed-subject", false, tt.clients, tt.realm)
			user := securitySessionUser(t, a, response)
			if len(user.Roles) != len(tt.want) {
				t.Fatalf("roles %v, want %v", user.Roles, tt.want)
			}
			for _, role := range tt.want {
				if !slices.Contains(user.Roles, role) {
					t.Fatalf("roles %v, missing %q", user.Roles, role)
				}
			}
		})
	}
}
