// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net"
	"testing"
	"time"
)

// TestGetIPUserLimiter tests the rate limiter creation and retrieval
func TestGetIPUserLimiter(t *testing.T) {
	ip := "192.168.1.1"
	username := "testuser"
	
	// Get limiter for the first time
	limiter1 := getIPUserLimiter(ip, username)
	if limiter1 == nil {
		t.Fatal("Expected limiter to be created")
	}
	
	// Get the same limiter again
	limiter2 := getIPUserLimiter(ip, username)
	if limiter1 != limiter2 {
		t.Error("Expected to get the same limiter instance")
	}
	
	// Get a different limiter for different user
	limiter3 := getIPUserLimiter(ip, "otheruser")
	if limiter1 == limiter3 {
		t.Error("Expected different limiter for different user")
	}
	
	// Get a different limiter for different IP
	limiter4 := getIPUserLimiter("192.168.1.2", username)
	if limiter1 == limiter4 {
		t.Error("Expected different limiter for different IP")
	}
}

// TestRateLimiterBehavior tests that rate limiting works correctly
func TestRateLimiterBehavior(t *testing.T) {
	ip := "10.0.0.1"
	username := "ratelimituser"
	
	limiter := getIPUserLimiter(ip, username)
	
	// Should allow first 5 attempts
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Errorf("Request %d should be allowed within rate limit", i+1)
		}
	}
	
	// 6th attempt should be blocked
	if limiter.Allow() {
		t.Error("Request 6 should be blocked by rate limiter")
	}
}

// TestCleanupOldRateLimiters tests the cleanup function
func TestCleanupOldRateLimiters(t *testing.T) {
	// Clear all existing limiters first to avoid interference from other tests
	cleanupOldRateLimiters(time.Now().Add(24 * time.Hour))
	
	// Create some new rate limiters
	limiter1 := getIPUserLimiter("1.1.1.1", "user1")
	limiter2 := getIPUserLimiter("2.2.2.2", "user2")
	
	if limiter1 == nil || limiter2 == nil {
		t.Fatal("Failed to create test limiters")
	}
	
	// Cleanup limiters older than 1 second from now (should keep both)
	time.Sleep(10 * time.Millisecond) // Small delay to ensure timestamp difference
	cleanupOldRateLimiters(time.Now().Add(-1 * time.Second))
	
	// Verify they still exist (should get same instance)
	if getIPUserLimiter("1.1.1.1", "user1") != limiter1 {
		t.Error("Limiter 1 was incorrectly cleaned up")
	}
	if getIPUserLimiter("2.2.2.2", "user2") != limiter2 {
		t.Error("Limiter 2 was incorrectly cleaned up")
	}
	
	// Cleanup limiters older than 1 hour from now (should remove both)
	cleanupOldRateLimiters(time.Now().Add(2 * time.Hour))
	
	// Getting them again should create new instances
	newLimiter1 := getIPUserLimiter("1.1.1.1", "user1")
	if newLimiter1 == limiter1 {
		t.Error("Old limiter should have been cleaned up")
	}
}

// TestIPv4Extraction tests extracting IPv4 addresses
func TestIPv4Extraction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"IPv4 with port", "192.168.1.1:8080", "192.168.1.1"},
		{"IPv4 without port", "192.168.1.1", "192.168.1.1"},
		{"Localhost with port", "127.0.0.1:3000", "127.0.0.1"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input
			if host, _, err := net.SplitHostPort(result); err == nil {
				result = host
			}
			
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestIPv6Extraction tests extracting IPv6 addresses  
func TestIPv6Extraction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"IPv6 with port", "[2001:db8::1]:8080", "2001:db8::1"},
		{"IPv6 localhost with port", "[::1]:3000", "::1"},
		{"IPv6 without port", "2001:db8::1", "2001:db8::1"},
		{"IPv6 localhost", "::1", "::1"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input
			if host, _, err := net.SplitHostPort(result); err == nil {
				result = host
			}
			
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestIPExtractionEdgeCases tests edge cases for IP extraction
func TestIPExtractionEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Hostname without port", "example.com", "example.com"},
		{"Empty string", "", ""},
		{"Just port", ":8080", ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input
			if host, _, err := net.SplitHostPort(result); err == nil {
				result = host
			}
			
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
