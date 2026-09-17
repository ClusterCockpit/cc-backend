// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"reflect"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestMapOIDCRoles(t *testing.T) {
	var (
		user    = schema.GetRoleString(schema.RoleUser)
		admin   = schema.GetRoleString(schema.RoleAdmin)
		support = schema.GetRoleString(schema.RoleSupport)
		api     = schema.GetRoleString(schema.RoleAPI)
	)

	mapping := map[string]string{
		"cc-admins":  admin,
		"cc-support": support,
		"cc-api":     api,
		"staff":      support, // second name mapping to the same role
	}

	tests := []struct {
		name      string
		oidcRoles []string
		mapping   map[string]string
		want      []string
	}{
		{
			name:      "explicit mapping to elevated roles",
			oidcRoles: []string{"cc-admins", "cc-api"},
			mapping:   mapping,
			want:      []string{admin, api},
		},
		{
			name:      "unmapped names are ignored (no identity fallback)",
			oidcRoles: []string{"admin", "support", "unknown"},
			mapping:   mapping,
			want:      []string{user},
		},
		{
			name:      "mix of mapped and unmapped keeps only mapped",
			oidcRoles: []string{"cc-admins", "admin", "noise"},
			mapping:   mapping,
			want:      []string{admin},
		},
		{
			name:      "empty token roles default to user",
			oidcRoles: nil,
			mapping:   mapping,
			want:      []string{user},
		},
		{
			name:      "no mapping configured defaults to user",
			oidcRoles: []string{"cc-admins", "admin"},
			mapping:   map[string]string{},
			want:      []string{user},
		},
		{
			name:      "duplicate target roles are deduplicated and sorted",
			oidcRoles: []string{"cc-support", "staff", "cc-admins"},
			mapping:   mapping,
			want:      []string{admin, support},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapOIDCRoles(tt.oidcRoles, tt.mapping)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mapOIDCRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
