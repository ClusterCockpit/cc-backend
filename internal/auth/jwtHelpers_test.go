// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/golang-jwt/jwt/v5"
)

// TestExtractStringFromClaims tests extracting string values from JWT claims
func TestExtractStringFromClaims(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":   "testuser",
		"email": "test@example.com",
		"age":   25, // not a string
	}
	
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"Existing string", "sub", "testuser"},
		{"Another string", "email", "test@example.com"},
		{"Non-existent key", "missing", ""},
		{"Non-string value", "age", ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractStringFromClaims(claims, tt.key)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestExtractRolesFromClaims tests role extraction and validation
func TestExtractRolesFromClaims(t *testing.T) {
	tests := []struct {
		name          string
		claims        jwt.MapClaims
		validateRoles bool
		expected      []string
	}{
		{
			name: "Valid roles without validation",
			claims: jwt.MapClaims{
				"roles": []any{"admin", "user", "invalid_role"},
			},
			validateRoles: false,
			expected:      []string{"admin", "user", "invalid_role"},
		},
		{
			name: "Valid roles with validation",
			claims: jwt.MapClaims{
				"roles": []any{"admin", "user", "api"},
			},
			validateRoles: true,
			expected:      []string{"admin", "user", "api"},
		},
		{
			name: "Invalid roles with validation",
			claims: jwt.MapClaims{
				"roles": []any{"invalid_role", "fake_role"},
			},
			validateRoles: true,
			expected:      []string{}, // Should filter out invalid roles
		},
		{
			name:          "No roles claim",
			claims:        jwt.MapClaims{},
			validateRoles: false,
			expected:      []string{},
		},
		{
			name: "Non-array roles",
			claims: jwt.MapClaims{
				"roles": "admin",
			},
			validateRoles: false,
			expected:      []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRolesFromClaims(tt.claims, tt.validateRoles)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d roles, got %d", len(tt.expected), len(result))
				return
			}
			
			for i, role := range result {
				if i >= len(tt.expected) || role != tt.expected[i] {
					t.Errorf("Expected role %s at position %d, got %s", tt.expected[i], i, role)
				}
			}
		})
	}
}

// TestExtractProjectsFromClaims tests project extraction from claims
func TestExtractProjectsFromClaims(t *testing.T) {
	tests := []struct {
		name     string
		claims   jwt.MapClaims
		expected []string
	}{
		{
			name: "Projects as array of interfaces",
			claims: jwt.MapClaims{
				"projects": []any{"project1", "project2", "project3"},
			},
			expected: []string{"project1", "project2", "project3"},
		},
		{
			name: "Projects as string array",
			claims: jwt.MapClaims{
				"projects": []string{"projectA", "projectB"},
			},
			expected: []string{"projectA", "projectB"},
		},
		{
			name:     "No projects claim",
			claims:   jwt.MapClaims{},
			expected: []string{},
		},
		{
			name: "Mixed types in projects array",
			claims: jwt.MapClaims{
				"projects": []any{"project1", 123, "project2"},
			},
			expected: []string{"project1", "project2"}, // Should skip non-strings
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractProjectsFromClaims(tt.claims)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d projects, got %d", len(tt.expected), len(result))
				return
			}
			
			for i, project := range result {
				if i >= len(tt.expected) || project != tt.expected[i] {
					t.Errorf("Expected project %s at position %d, got %s", tt.expected[i], i, project)
				}
			}
		})
	}
}

// TestExtractNameFromClaims tests name extraction from various formats
func TestExtractNameFromClaims(t *testing.T) {
	tests := []struct {
		name     string
		claims   jwt.MapClaims
		expected string
	}{
		{
			name: "Simple string name",
			claims: jwt.MapClaims{
				"name": "John Doe",
			},
			expected: "John Doe",
		},
		{
			name: "Nested name structure",
			claims: jwt.MapClaims{
				"name": map[string]any{
					"values": []any{"John", "Doe"},
				},
			},
			expected: "John Doe",
		},
		{
			name: "Nested name with single value",
			claims: jwt.MapClaims{
				"name": map[string]any{
					"values": []any{"Alice"},
				},
			},
			expected: "Alice",
		},
		{
			name:     "No name claim",
			claims:   jwt.MapClaims{},
			expected: "",
		},
		{
			name: "Empty nested values",
			claims: jwt.MapClaims{
				"name": map[string]any{
					"values": []any{},
				},
			},
			expected: "",
		},
		{
			name: "Nested with non-string values",
			claims: jwt.MapClaims{
				"name": map[string]any{
					"values": []any{123, "Smith"},
				},
			},
			expected: "123 Smith", // Should convert to string
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractNameFromClaims(tt.claims)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestGetUserFromJWT_NoValidation tests getUserFromJWT without database validation
func TestGetUserFromJWT_NoValidation(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":      "testuser",
		"name":     "Test User",
		"roles":    []any{"user", "admin"},
		"projects": []any{"project1", "project2"},
	}
	
	user, err := getUserFromJWT(claims, false, schema.AuthToken, -1)
	
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}
	
	if user.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", user.Name)
	}
	
	if len(user.Roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(user.Roles))
	}
	
	if len(user.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(user.Projects))
	}
	
	if user.AuthType != schema.AuthToken {
		t.Errorf("Expected AuthType %v, got %v", schema.AuthToken, user.AuthType)
	}
}

// TestGetUserFromJWT_MissingSub tests error when sub claim is missing
func TestGetUserFromJWT_MissingSub(t *testing.T) {
	claims := jwt.MapClaims{
		"name": "Test User",
	}
	
	_, err := getUserFromJWT(claims, false, schema.AuthToken, -1)
	
	if err == nil {
		t.Error("Expected error for missing sub claim")
	}
	
	if err.Error() != "missing 'sub' claim in JWT" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}
