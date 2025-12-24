// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/golang-jwt/jwt/v5"
)

// extractStringFromClaims extracts a string value from JWT claims
func extractStringFromClaims(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key].(string); ok {
		return val
	}
	return ""
}

// extractRolesFromClaims extracts roles from JWT claims
// If validateRoles is true, only valid roles are returned
func extractRolesFromClaims(claims jwt.MapClaims, validateRoles bool) []string {
	var roles []string
	
	if rawroles, ok := claims["roles"].([]any); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				if validateRoles {
					if schema.IsValidRole(r) {
						roles = append(roles, r)
					}
				} else {
					roles = append(roles, r)
				}
			}
		}
	}
	
	return roles
}

// extractProjectsFromClaims extracts projects from JWT claims
func extractProjectsFromClaims(claims jwt.MapClaims) []string {
	projects := make([]string, 0)
	
	if rawprojs, ok := claims["projects"].([]any); ok {
		for _, pp := range rawprojs {
			if p, ok := pp.(string); ok {
				projects = append(projects, p)
			}
		}
	} else if rawprojs, ok := claims["projects"]; ok {
		if projSlice, ok := rawprojs.([]string); ok {
			projects = append(projects, projSlice...)
		}
	}
	
	return projects
}

// extractNameFromClaims extracts name from JWT claims
// Handles both simple string and complex nested structure
func extractNameFromClaims(claims jwt.MapClaims) string {
	// Try simple string first
	if name, ok := claims["name"].(string); ok {
		return name
	}
	
	// Try nested structure: {name: {values: [...]}}
	if wrap, ok := claims["name"].(map[string]any); ok {
		if vals, ok := wrap["values"].([]any); ok {
			if len(vals) == 0 {
				return ""
			}
			
			name := fmt.Sprintf("%v", vals[0])
			for i := 1; i < len(vals); i++ {
				name += fmt.Sprintf(" %v", vals[i])
			}
			return name
		}
	}
	
	return ""
}

// getUserFromJWT creates or retrieves a user based on JWT claims
// If validateUser is true, the user must exist in the database
// Otherwise, a new user object is created from claims
// authSource should be a schema.AuthSource constant (like schema.AuthViaToken)
func getUserFromJWT(claims jwt.MapClaims, validateUser bool, authType schema.AuthType, authSource schema.AuthSource) (*schema.User, error) {
	sub := extractStringFromClaims(claims, "sub")
	if sub == "" {
		return nil, errors.New("missing 'sub' claim in JWT")
	}
	
	if validateUser {
		// Validate user against database
		ur := repository.GetUserRepository()
		user, err := ur.GetUser(sub)
		if err != nil && err != sql.ErrNoRows {
			cclog.Errorf("Error while loading user '%v': %v", sub, err)
			return nil, fmt.Errorf("database error: %w", err)
		}
		
		// Deny any logins for unknown usernames
		if user == nil || err == sql.ErrNoRows {
			cclog.Warn("Could not find user from JWT in internal database.")
			return nil, errors.New("unknown user")
		}
		
		// Return database user (with database roles)
		return user, nil
	}
	
	// Create user from JWT claims
	name := extractNameFromClaims(claims)
	roles := extractRolesFromClaims(claims, true) // Validate roles
	projects := extractProjectsFromClaims(claims)
	
	return &schema.User{
		Username:   sub,
		Name:       name,
		Roles:      roles,
		Projects:   projects,
		AuthType:   authType,
		AuthSource: authSource,
	}, nil
}
