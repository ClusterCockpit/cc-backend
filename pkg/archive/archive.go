// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package archive implements the job archive interface and various backend implementations.
//
// The archive package provides a pluggable storage backend system for job metadata and performance data.
// It supports three backend types:
//
//   - file: Filesystem-based storage with hierarchical directory structure
//   - s3: AWS S3 and S3-compatible object storage (MinIO, localstack)
//   - sqlite: Single-file SQLite database with BLOB storage
//
// # Backend Selection
//
// Choose a backend based on your deployment requirements:
//
//   - File: Best for single-server deployments with local fast storage
//   - S3: Best for distributed deployments requiring redundancy and multi-instance access
//   - SQLite: Best for portable archives with SQL query capability and transactional integrity
//
// # Configuration
//
// The archive backend is configured via JSON in the application config file:
//
//	{
//	  "archive": {
//	    "kind": "file",           // or "s3" or "sqlite"
//	    "path": "/var/lib/archive" // for file backend
//	  }
//	}
//
// For S3 backend:
//
//	{
//	  "archive": {
//	    "kind": "s3",
//	    "bucket": "my-job-archive",
//	    "region": "us-east-1",
//	    "accessKey": "...",
//	    "secretKey": "..."
//	  }
//	}
//
// For SQLite backend:
//
//	{
//	  "archive": {
//	    "kind": "sqlite",
//	    "dbPath": "/var/lib/archive.db"
//	  }
//	}
//
// # Usage
//
// The package is initialized once at application startup:
//
//	err := archive.Init(rawConfig, false)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// After initialization, use the global functions to interact with the archive:
//
//	// Check if a job exists
//	exists := archive.GetHandle().Exists(job)
//
//	// Load job metadata
//	jobMeta, err := archive.GetHandle().LoadJobMeta(job)
//
//	// Store job metadata
//	err = archive.GetHandle().StoreJobMeta(job)
//
// # Thread Safety
//
// All backend implementations are safe for concurrent use. The package uses
// internal locking for operations that modify shared state.
package archive

import (
	"encoding/json"
	"fmt"
	"maps"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/lrucache"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Version is the current archive schema version.
// The archive backend must match this version for compatibility.
const Version uint64 = 3

// ArchiveBackend defines the interface that all archive storage backends must implement.
// Implementations include FsArchive (filesystem), S3Archive (object storage), and SqliteArchive (database).
//
// All methods are safe for concurrent use unless otherwise noted.
type ArchiveBackend interface {
	// Init initializes the archive backend with the provided configuration.
	// Returns the archive version found in the backend storage.
	// Returns an error if the version is incompatible or initialization fails.
	Init(rawConfig json.RawMessage) (uint64, error)

	// Info prints archive statistics to stdout, including job counts,
	// date ranges, and storage sizes per cluster.
	Info()

	// Exists checks if a job with the given ID, cluster, and start time
	// exists in the archive.
	Exists(job *schema.Job) bool

	// LoadJobMeta loads job metadata from the archive.
	// Returns the complete Job structure including resources, tags, and statistics.
	LoadJobMeta(job *schema.Job) (*schema.Job, error)

	// LoadJobData loads the complete time-series performance data for a job.
	// Returns a map of metric names to their scoped data (node, socket, core, etc.).
	LoadJobData(job *schema.Job) (schema.JobData, error)

	// LoadJobStats loads pre-computed statistics from the job data.
	// Returns scoped statistics (min, max, avg) for all metrics.
	LoadJobStats(job *schema.Job) (schema.ScopedJobStats, error)

	// LoadClusterCfg loads the cluster configuration.
	// Returns the cluster topology, metrics, and hardware specifications.
	LoadClusterCfg(name string) (*schema.Cluster, error)

	// StoreJobMeta stores job metadata to the archive.
	// Overwrites existing metadata for the same job ID, cluster, and start time.
	StoreJobMeta(jobMeta *schema.Job) error

	// StoreClusterCfg stores the cluster configuration to the archive.
	// Overwrites an existing configuration for the same cluster.
	StoreClusterCfg(name string, config *schema.Cluster) error

	// ImportJob stores both job metadata and performance data to the archive.
	// This is typically used during initial job archiving.
	ImportJob(jobMeta *schema.Job, jobData *schema.JobData) error

	// GetClusters returns a list of all cluster names found in the archive.
	GetClusters() []string

	// CleanUp removes the specified jobs from the archive.
	// Used by retention policies to delete old jobs.
	CleanUp(jobs []*schema.Job)

	// Move relocates jobs to a different path within the archive.
	// The implementation depends on the backend type.
	Move(jobs []*schema.Job, path string)

	// Clean removes jobs outside the specified time range.
	// Jobs with start_time < before OR start_time > after are deleted.
	// Set after=0 to only use the before parameter.
	Clean(before int64, after int64)

	// Compress compresses job data files to save storage space.
	// For filesystem and SQLite backends, this applies gzip compression.
	// For S3, this compresses and replaces objects.
	Compress(jobs []*schema.Job)

	// CompressLast returns the timestamp of the last compression run
	// and updates it to the provided starttime.
	CompressLast(starttime int64) int64

	// Iter returns a channel that yields all jobs in the archive.
	// If loadMetricData is true, includes performance data; otherwise only metadata.
	// The channel is closed when iteration completes.
	Iter(loadMetricData bool) <-chan JobContainer
}

// JobContainer combines job metadata and optional performance data.
// Used by Iter() to yield jobs during archive iteration.
type JobContainer struct {
	Meta *schema.Job     // Job metadata (always present)
	Data *schema.JobData // Performance data (nil if not loaded)
}

var (
	initOnce   sync.Once
	cache      *lrucache.Cache = lrucache.New(128 * 1024 * 1024)
	ar         ArchiveBackend
	useArchive bool
	mutex      sync.Mutex
)

// Init initializes the archive backend with the provided configuration.
// Must be called once at application startup before using any archive functions.
//
// Parameters:
//   - rawConfig: JSON configuration for the archive backend
//   - disableArchive: if true, disables archive functionality
//
// The configuration determines which backend is used (file, s3, or sqlite).
// Returns an error if initialization fails or version is incompatible.
func Init(rawConfig json.RawMessage, disableArchive bool) error {
	var err error

	initOnce.Do(func() {
		useArchive = !disableArchive

		var cfg struct {
			Kind string `json:"kind"`
		}

		config.Validate(configSchema, rawConfig)
		if err = json.Unmarshal(rawConfig, &cfg); err != nil {
			cclog.Warn("Error while unmarshaling raw config json")
			return
		}

		switch cfg.Kind {
		case "file":
			ar = &FsArchive{}
		case "s3":
			ar = &S3Archive{}
		case "sqlite":
			ar = &SqliteArchive{}
		default:
			err = fmt.Errorf("ARCHIVE/ARCHIVE > unkown archive backend '%s''", cfg.Kind)
		}

		var version uint64
		version, err = ar.Init(rawConfig)
		if err != nil {
			cclog.Errorf("Error while initializing archiveBackend: %s", err.Error())
			return
		}
		cclog.Infof("Load archive version %d", version)

		err = initClusterConfig()
	})

	return err
}

// GetHandle returns the initialized archive backend instance.
// Must be called after Init().
func GetHandle() ArchiveBackend {
	return ar
}

// InitBackend creates and initializes a new archive backend instance
// without affecting the global singleton. This is useful for archive migration
// tools that need to work with multiple archive backends simultaneously.
//
// Parameters:
//   - rawConfig: JSON configuration for the archive backend
//
// Returns the initialized backend instance or an error if initialization fails.
// Does not validate the configuration against the schema.
func InitBackend(rawConfig json.RawMessage) (ArchiveBackend, error) {
	var cfg struct {
		Kind string `json:"kind"`
	}

	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		cclog.Warn("Error while unmarshaling raw config json")
		return nil, err
	}

	var backend ArchiveBackend
	switch cfg.Kind {
	case "file":
		backend = &FsArchive{}
	case "s3":
		backend = &S3Archive{}
	case "sqlite":
		backend = &SqliteArchive{}
	default:
		return nil, fmt.Errorf("ARCHIVE/ARCHIVE > unknown archive backend '%s'", cfg.Kind)
	}

	_, err := backend.Init(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing archive backend: %w", err)
	}

	return backend, nil
}

// LoadAveragesFromArchive loads average metric values for a job from the archive.
// This is a helper function that extracts average values from job statistics.
//
// Parameters:
//   - job: Job to load averages for
//   - metrics: List of metric names to retrieve
//   - data: 2D slice where averages will be appended (one row per metric)
func LoadAveragesFromArchive(
	job *schema.Job,
	metrics []string,
	data [][]schema.Float,
) error {
	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		cclog.Errorf("Error while loading job metadata from archiveBackend: %s", err.Error())
		return err
	}

	for i, m := range metrics {
		if stat, ok := metaFile.Statistics[m]; ok {
			data[i] = append(data[i], schema.Float(stat.Avg))
		} else {
			data[i] = append(data[i], schema.NaN)
		}
	}

	return nil
}

// LoadStatsFromArchive loads metric statistics for a job from the archive.
// Returns a map of metric names to their statistics (min, max, avg).
func LoadStatsFromArchive(
	job *schema.Job,
	metrics []string,
) (map[string]schema.MetricStatistics, error) {
	data := make(map[string]schema.MetricStatistics, len(metrics))
	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		cclog.Errorf("Error while loading job metadata from archiveBackend: %s", err.Error())
		return data, err
	}

	for _, m := range metrics {
		stat, ok := metaFile.Statistics[m]
		if !ok {
			data[m] = schema.MetricStatistics{Min: 0.0, Avg: 0.0, Max: 0.0}
			continue
		}

		data[m] = schema.MetricStatistics{
			Avg: stat.Avg,
			Min: stat.Min,
			Max: stat.Max,
		}
	}

	return data, nil
}

// LoadScopedStatsFromArchive loads scoped statistics for a job from the archive.
// Returns statistics organized by metric scope (node, socket, core, etc.).
func LoadScopedStatsFromArchive(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
) (schema.ScopedJobStats, error) {
	data, err := ar.LoadJobStats(job)
	if err != nil {
		cclog.Errorf("Error while loading job stats from archiveBackend: %s", err.Error())
		return nil, err
	}

	return data, nil
}

// GetStatistics returns all metric statistics for a job.
// Returns a map of metric names to their job-level statistics.
func GetStatistics(job *schema.Job) (map[string]schema.JobStatistics, error) {
	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		cclog.Errorf("Error while loading job metadata from archiveBackend: %s", err.Error())
		return nil, err
	}

	return metaFile.Statistics, nil
}

// UpdateMetadata updates the metadata map for an archived job.
// If the job is still running or archiving is disabled, this is a no-op.
//
// This function is safe for concurrent use (protected by mutex).
func UpdateMetadata(job *schema.Job, metadata map[string]string) error {
	mutex.Lock()
	defer mutex.Unlock()

	if job.State == schema.JobStateRunning || !useArchive {
		return nil
	}

	jobMeta, err := ar.LoadJobMeta(job)
	if err != nil {
		cclog.Errorf("Error while loading job metadata from archiveBackend: %s", err.Error())
		return err
	}

	maps.Copy(jobMeta.MetaData, metadata)

	return ar.StoreJobMeta(jobMeta)
}

// UpdateTags updates the tag list for an archived job.
// If the job is still running or archiving is disabled, this is a no-op.
//
// This function is safe for concurrent use (protected by mutex).
func UpdateTags(job *schema.Job, tags []*schema.Tag) error {
	mutex.Lock()
	defer mutex.Unlock()

	if job.State == schema.JobStateRunning || !useArchive {
		return nil
	}

	jobMeta, err := ar.LoadJobMeta(job)
	if err != nil {
		cclog.Errorf("Error while loading job metadata from archiveBackend: %s", err.Error())
		return err
	}

	jobMeta.Tags = make([]*schema.Tag, 0)
	for _, tag := range tags {
		jobMeta.Tags = append(jobMeta.Tags, &schema.Tag{
			Name:  tag.Name,
			Type:  tag.Type,
			Scope: tag.Scope,
		})
	}

	return ar.StoreJobMeta(jobMeta)
}
