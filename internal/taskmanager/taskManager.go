// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

const (
	DefaultCompressOlderThan = 7
)

// Retention defines the configuration for job retention policies.
type Retention struct {
	Policy             string `json:"policy"`
	Format             string `json:"format"`
	Age                int    `json:"age"`
	IncludeDB          bool   `json:"include-db"`
	OmitTagged         string `json:"omit-tagged"`
	TargetKind         string `json:"target-kind"`
	TargetPath         string `json:"target-path"`
	TargetEndpoint     string `json:"target-endpoint"`
	TargetBucket       string `json:"target-bucket"`
	TargetAccessKey    string `json:"target-access-key"`
	TargetSecretKey    string `json:"target-secret-key"`
	TargetRegion       string `json:"target-region"`
	TargetUsePathStyle bool   `json:"target-use-path-style"`
	MaxFileSizeMB      int    `json:"max-file-size-mb"`
}

// CronFrequency defines the execution intervals for various background workers.
type CronFrequency struct {
	// Duration Update Worker [Defaults to '2m']
	CommitJobWorker string `json:"commit-job-worker"`
	// Duration Update Worker [Defaults to '5m']
	DurationWorker string `json:"duration-worker"`
	// Metric-Footprint Update Worker [Defaults to '10m']
	FootprintWorker string `json:"footprint-worker"`
}

var (
	s       gocron.Scheduler
	jobRepo *repository.JobRepository
	// Keys holds the configured frequencies for cron jobs.
	Keys CronFrequency
)

// parseDuration parses a duration string and handles errors by logging them.
// It returns the duration and any error encountered.
func parseDuration(s string) (time.Duration, error) {
	interval, err := time.ParseDuration(s)
	if err != nil {
		cclog.Warnf("Could not parse duration for sync interval: %v",
			s)
		return 0, err
	}

	if interval == 0 {
		cclog.Info("TaskManager: Sync interval is zero")
	}

	return interval, nil
}

func initArchiveServices(config json.RawMessage) {
	var cfg struct {
		Retention   Retention `json:"retention"`
		Compression int       `json:"compression"`
	}
	cfg.Retention.IncludeDB = true

	if err := json.Unmarshal(config, &cfg); err != nil {
		cclog.Errorf("error while unmarshaling raw config json: %v", err)
	}

	switch cfg.Retention.Policy {
	case "delete":
		RegisterRetentionDeleteService(cfg.Retention)
	case "copy":
		RegisterRetentionCopyService(cfg.Retention)
	case "move":
		RegisterRetentionMoveService(cfg.Retention)
	}

	if cfg.Compression > 0 {
		RegisterCompressionService(cfg.Compression)
	} else {
		RegisterCompressionService(DefaultCompressOlderThan)
	}
}

// Start initializes the task manager, parses configurations, and registers background tasks.
// It starts the gocron scheduler.
func Start(cronCfg, archiveConfig json.RawMessage) {
	var err error
	jobRepo = repository.GetJobRepository()
	s, err = gocron.NewScheduler()
	if err != nil {
		cclog.Abortf("Taskmanager Start: Could not create gocron scheduler.\nError: %s\n", err.Error())
	}

	if config.Keys.StopJobsExceedingWalltime > 0 {
		RegisterStopJobsExceedTime()
	}

	dec := json.NewDecoder(bytes.NewReader(cronCfg))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&Keys); err != nil {
		cclog.Errorf("error while decoding cron config: %v", err)
	}

	if archiveConfig != nil {
		initArchiveServices(archiveConfig)
	} else {
		// Always enable compression
		RegisterCompressionService(DefaultCompressOlderThan)
	}

	lc := auth.Keys.LdapConfig

	if lc != nil && lc.SyncInterval != "" {
		RegisterLdapSyncService(lc.SyncInterval)
	}

	RegisterFootprintWorker()
	RegisterUpdateDurationWorker()
	RegisterCommitJobService()

	if config.Keys.NodeStateRetention != nil && config.Keys.NodeStateRetention.Policy != "" {
		initNodeStateRetention()
	}

	s.Start()
}

func initNodeStateRetention() {
	cfg := config.Keys.NodeStateRetention
	age := cfg.Age
	if age <= 0 {
		age = 24
	}

	switch cfg.Policy {
	case "delete":
		RegisterNodeStateRetentionDeleteService(age)
	case "move":
		RegisterNodeStateRetentionMoveService(cfg)
	default:
		cclog.Warnf("Unknown nodestate-retention policy: %s", cfg.Policy)
	}
}

// Shutdown stops the task manager and its scheduler.
func Shutdown() {
	if s != nil {
		s.Shutdown()
	}
}
