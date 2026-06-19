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

func TestMergeLdapRoles(t *testing.T) {
	var (
		user    = schema.GetRoleString(schema.RoleUser)
		admin   = schema.GetRoleString(schema.RoleAdmin)
		support = schema.GetRoleString(schema.RoleSupport)
		api     = schema.GetRoleString(schema.RoleAPI)
		manager = schema.GetRoleString(schema.RoleManager)
	)

	tests := []struct {
		name     string
		current  []string
		managed  []string
		matched  []string
		projects []string
		want     []string
	}{
		{
			name:    "no managed roles is a no-op keeping user baseline",
			current: []string{user},
			managed: nil,
			matched: nil,
			want:    []string{user},
		},
		{
			name:    "add matched elevated role to plain user",
			current: []string{user},
			managed: []string{admin, support},
			matched: []string{admin},
			want:    []string{admin, user},
		},
		{
			name:    "remove managed role when no longer matched",
			current: []string{admin, user},
			managed: []string{admin},
			matched: nil,
			want:    []string{user},
		},
		{
			name:    "preserve non-managed roles (manager, api)",
			current: []string{api, manager, user},
			managed: []string{admin, support},
			matched: []string{support},
			want:    []string{api, manager, support, user},
		},
		{
			name:     "managed manager with projects is not removed",
			current:  []string{manager, user},
			managed:  []string{manager},
			matched:  nil,
			projects: []string{"projA"},
			want:     []string{manager, user},
		},
		{
			name:    "managed manager without projects is removed",
			current: []string{manager, user},
			managed: []string{manager},
			matched: nil,
			want:    []string{user},
		},
		{
			name:    "user baseline always present even if absent in current",
			current: []string{admin},
			managed: []string{admin},
			matched: []string{admin},
			want:    []string{admin, user},
		},
		{
			name:    "new user (nil current) gets matched roles plus baseline",
			current: nil,
			managed: []string{admin, support, api},
			matched: []string{admin, api},
			want:    []string{admin, api, user},
		},
		{
			name:    "result is deduplicated",
			current: []string{admin, admin, user},
			managed: []string{admin},
			matched: []string{admin},
			want:    []string{admin, user},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeLdapRoles(tt.current, tt.managed, tt.matched, tt.projects)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeLdapRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
