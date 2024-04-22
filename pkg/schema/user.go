// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"fmt"
	"strings"
)

type Role int

const (
	RoleAnonymous Role = iota
	RoleApi
	RoleUser
	RoleManager
	RoleSupport
	RoleAdmin
	RoleError
)

type AuthSource int

const (
	AuthViaLocalPassword AuthSource = iota
	AuthViaLDAP
	AuthViaToken
	AuthViaOIDC
	AuthViaAll
)

type AuthType int

const (
	AuthToken AuthType = iota
	AuthSession
)

type User struct {
	Username   string     `json:"username"`
	Password   string     `json:"-"`
	Name       string     `json:"name"`
	Roles      []string   `json:"roles"`
	AuthType   AuthType   `json:"authType"`
	AuthSource AuthSource `json:"authSource"`
	Email      string     `json:"email"`
	Projects   []string   `json:"projects"`
}

func (u *User) HasProject(project string) bool {
	for _, p := range u.Projects {
		if p == project {
			return true
		}
	}
	return false
}

func GetRoleString(roleInt Role) string {
	return [6]string{"anonymous", "api", "user", "manager", "support", "admin"}[roleInt]
}

func getRoleEnum(roleStr string) Role {
	switch strings.ToLower(roleStr) {
	case "admin":
		return RoleAdmin
	case "support":
		return RoleSupport
	case "manager":
		return RoleManager
	case "user":
		return RoleUser
	case "api":
		return RoleApi
	case "anonymous":
		return RoleAnonymous
	default:
		return RoleError
	}
}

func IsValidRole(role string) bool {
	return getRoleEnum(role) != RoleError
}

func (u *User) HasValidRole(role string) (hasRole bool, isValid bool) {
	if IsValidRole(role) {
		for _, r := range u.Roles {
			if r == role {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

func (u *User) HasRole(role Role) bool {
	for _, r := range u.Roles {
		if r == GetRoleString(role) {
			return true
		}
	}
	return false
}

// Role-Arrays are short: performance not impacted by nested loop
func (u *User) HasAnyRole(queryroles []Role) bool {
	for _, ur := range u.Roles {
		for _, qr := range queryroles {
			if ur == GetRoleString(qr) {
				return true
			}
		}
	}
	return false
}

// Role-Arrays are short: performance not impacted by nested loop
func (u *User) HasAllRoles(queryroles []Role) bool {
	target := len(queryroles)
	matches := 0
	for _, ur := range u.Roles {
		for _, qr := range queryroles {
			if ur == GetRoleString(qr) {
				matches += 1
				break
			}
		}
	}

	if matches == target {
		return true
	} else {
		return false
	}
}

// Role-Arrays are short: performance not impacted by nested loop
func (u *User) HasNotRoles(queryroles []Role) bool {
	matches := 0
	for _, ur := range u.Roles {
		for _, qr := range queryroles {
			if ur == GetRoleString(qr) {
				matches += 1
				break
			}
		}
	}

	if matches == 0 {
		return true
	} else {
		return false
	}
}

// Called by API endpoint '/roles/' from frontend: Only required for admin config -> Check Admin Role
func GetValidRoles(user *User) ([]string, error) {
	var vals []string
	if user.HasRole(RoleAdmin) {
		for i := RoleApi; i < RoleError; i++ {
			vals = append(vals, GetRoleString(i))
		}
		return vals, nil
	}

	return vals, fmt.Errorf("%s: only admins are allowed to fetch a list of roles", user.Username)
}

// Called by routerConfig web.page setup in backend: Only requires known user
func GetValidRolesMap(user *User) (map[string]Role, error) {
	named := make(map[string]Role)
	if user.HasNotRoles([]Role{RoleAnonymous}) {
		for i := RoleApi; i < RoleError; i++ {
			named[GetRoleString(i)] = i
		}
		return named, nil
	}
	return named, fmt.Errorf("only known users are allowed to fetch a list of roles")
}

// Find highest role
func (u *User) GetAuthLevel() Role {
	if u.HasRole(RoleAdmin) {
		return RoleAdmin
	} else if u.HasRole(RoleSupport) {
		return RoleSupport
	} else if u.HasRole(RoleManager) {
		return RoleManager
	} else if u.HasRole(RoleUser) {
		return RoleUser
	} else if u.HasRole(RoleApi) {
		return RoleApi
	} else if u.HasRole(RoleAnonymous) {
		return RoleAnonymous
	} else {
		return RoleError
	}
}
