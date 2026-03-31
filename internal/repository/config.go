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
	// Default: 10 minutes
	ConnectionMaxIdleTime time.Duration

	// MinRunningJobDuration is the minimum duration in seconds for a job to be
	// considered in "running jobs" queries. This filters out very short jobs.
	// Default: 600 seconds (10 minutes)
	MinRunningJobDuration int

	// DbCacheSizeMB is the SQLite page cache size per connection in MB.
	// Uses negative PRAGMA cache_size notation (KiB). With MaxOpenConnections=4
	// and DbCacheSizeMB=2048, total page cache is up to 8GB.
	// Default: 2048 (2GB)
	DbCacheSizeMB int

	// DbSoftHeapLimitMB is the process-wide SQLite soft heap limit in MB.
	// SQLite will try to release cache pages to stay under this limit.
	// It's a soft limit — queries won't fail, but cache eviction becomes more aggressive.
	// Default: 16384 (16GB)
	DbSoftHeapLimitMB int

	// BusyTimeoutMs is the SQLite busy_timeout in milliseconds.
	// When a write is blocked by another writer, SQLite retries internally
	// using a backoff mechanism for up to this duration before returning SQLITE_BUSY.
	// Default: 60000 (60 seconds)
	BusyTimeoutMs int
}

// DefaultConfig returns the default repository configuration.
// These values are optimized for typical deployments.
func DefaultConfig() *RepositoryConfig {
	return &RepositoryConfig{
		CacheSize:             1 * 1024 * 1024, // 1MB
		MaxOpenConnections:    4,
		MaxIdleConnections:    4,
		ConnectionMaxLifetime: time.Hour,
		ConnectionMaxIdleTime: 10 * time.Minute,
		MinRunningJobDuration: 600,   // 10 minutes
		DbCacheSizeMB:         2048,  // 2GB per connection
		DbSoftHeapLimitMB:     16384, // 16GB process-wide
		BusyTimeoutMs:         60000, // 60 seconds
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
