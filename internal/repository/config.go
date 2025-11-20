// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import "time"

// RepositoryConfig holds configuration for repository operations.
// All fields have sensible defaults, so this configuration is optional.
type RepositoryConfig struct {
	// CacheSize is the LRU cache size in bytes for job metadata and energy footprints.
	// Default: 1MB (1024 * 1024 bytes)
	CacheSize int

	// MaxOpenConnections is the maximum number of open database connections.
	// Default: 4
	MaxOpenConnections int

	// MaxIdleConnections is the maximum number of idle database connections.
	// Default: 4
	MaxIdleConnections int

	// ConnectionMaxLifetime is the maximum amount of time a connection may be reused.
	// Default: 1 hour
	ConnectionMaxLifetime time.Duration

	// ConnectionMaxIdleTime is the maximum amount of time a connection may be idle.
	// Default: 1 hour
	ConnectionMaxIdleTime time.Duration

	// MinRunningJobDuration is the minimum duration in seconds for a job to be
	// considered in "running jobs" queries. This filters out very short jobs.
	// Default: 600 seconds (10 minutes)
	MinRunningJobDuration int
}

// DefaultConfig returns the default repository configuration.
// These values are optimized for typical deployments.
func DefaultConfig() *RepositoryConfig {
	return &RepositoryConfig{
		CacheSize:              1 * 1024 * 1024, // 1MB
		MaxOpenConnections:     4,
		MaxIdleConnections:     4,
		ConnectionMaxLifetime:  time.Hour,
		ConnectionMaxIdleTime:  time.Hour,
		MinRunningJobDuration:  600, // 10 minutes
	}
}

// repoConfig is the package-level configuration instance.
// It is initialized with defaults and can be overridden via SetConfig.
var repoConfig *RepositoryConfig = DefaultConfig()

// SetConfig sets the repository configuration.
// This must be called before any repository initialization (Connect, GetJobRepository, etc.).
// If not called, default values from DefaultConfig() are used.
func SetConfig(cfg *RepositoryConfig) {
	if cfg != nil {
		repoConfig = cfg
	}
}

// GetConfig returns the current repository configuration.
func GetConfig() *RepositoryConfig {
	return repoConfig
}
