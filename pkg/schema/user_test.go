// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"testing"
)

func TestHasValidRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user"}}

	exists, _ := u.HasValidRole("user")

	if !exists {
		t.Fatalf(`User{Roles: ["user"]} -> HasValidRole("user"): EXISTS = %v, expected 'true'.`, exists)
	}
}

func TestHasNotValidRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user"}}

	exists, _ := u.HasValidRole("manager")

	if exists {
		t.Fatalf(`User{Roles: ["user"]} -> HasValidRole("manager"): EXISTS = %v, expected 'false'.`, exists)
	}
}

func TestHasInvalidRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user"}}

	_, valid := u.HasValidRole("invalid")

	if valid {
		t.Fatalf(`User{Roles: ["user"]} -> HasValidRole("invalid"): VALID = %v, expected 'false'.`, valid)
	}
}

func TestHasNotInvalidRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user"}}

	_, valid := u.HasValidRole("user")

	if !valid {
		t.Fatalf(`User{Roles: ["user"]} -> HasValidRole("user"): VALID = %v, expected 'true'.`, valid)
	}
}

func TestHasRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user"}}

	exists := u.HasRole(RoleUser)

	if !exists {
		t.Fatalf(`User{Roles: ["user"]} -> HasRole(RoleUser): EXISTS = %v, expected 'true'.`, exists)
	}
}

func TestHasNotRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user"}}

	exists := u.HasRole(RoleManager)

	if exists {
		t.Fatalf(`User{Roles: ["user"]} -> HasRole(RoleManager): EXISTS = %v, expected 'false'.`, exists)
	}
}

func TestHasAnyRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user", "manager"}}

	result := u.HasAnyRole([]Role{RoleManager, RoleSupport, RoleAdmin})

	if !result {
		t.Fatalf(`User{Roles: ["user", "manager"]} -> HasAnyRole([]Role{RoleManager, RoleSupport, RoleAdmin}): RESULT = %v, expected 'true'.`, result)
	}
}

func TestHasNotAnyRole(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user", "manager"}}

	result := u.HasAnyRole([]Role{RoleSupport, RoleAdmin})

	if result {
		t.Fatalf(`User{Roles: ["user", "manager"]} -> HasAllRoles([]Role{RoleSupport, RoleAdmin}): RESULT = %v, expected 'false'.`, result)
	}
}

func TestHasAllRoles(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user", "manager", "support"}}

	result := u.HasAllRoles([]Role{RoleUser, RoleManager, RoleSupport})

	if !result {
		t.Fatalf(`User{Roles: ["user", "manager", "support"]} -> HasAllRoles([]Role{RoleUser, RoleManager, RoleSupport}): RESULT = %v, expected 'true'.`, result)
	}
}

func TestHasNotAllRoles(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user", "manager"}}

	result := u.HasAllRoles([]Role{RoleUser, RoleManager, RoleSupport})

	if result {
		t.Fatalf(`User{Roles: ["user", "manager"]} -> HasAllRoles([]Role{RoleUser, RoleManager, RoleSupport}): RESULT = %v, expected 'false'.`, result)
	}
}

func TestHasNotRoles(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user", "manager"}}

	result := u.HasNotRoles([]Role{RoleSupport, RoleAdmin})

	if !result {
		t.Fatalf(`User{Roles: ["user", "manager"]} -> HasNotRoles([]Role{RoleSupport, RoleAdmin}): RESULT = %v, expected 'true'.`, result)
	}
}

func TestHasAllNotRoles(t *testing.T) {
	u := User{Username: "testuser", Roles: []string{"user", "manager"}}

	result := u.HasNotRoles([]Role{RoleUser, RoleManager})

	if result {
		t.Fatalf(`User{Roles: ["user", "manager"]} -> HasNotRoles([]Role{RoleUser, RoleManager}): RESULT = %v, expected 'false'.`, result)
	}
}
