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
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

// Retention defines the configuration for job retention policies.
type Retention struct {
	Policy     string `json:"policy"`
	Location   string `json:"location"`
	Age        int    `json:"age"`
	IncludeDB  bool   `json:"includeDB"`
	OmitTagged bool   `json:"omitTagged"`
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

	var cfg struct {
		Retention   Retention `json:"retention"`
		Compression int       `json:"compression"`
	}
	cfg.Retention.IncludeDB = true

	if err := json.Unmarshal(archiveConfig, &cfg); err != nil {
		cclog.Warn("Error while unmarshaling raw config json")
	}

	switch cfg.Retention.Policy {
	case "delete":
		RegisterRetentionDeleteService(
			cfg.Retention.Age,
			cfg.Retention.IncludeDB,
			cfg.Retention.OmitTagged)
	case "move":
		RegisterRetentionMoveService(
			cfg.Retention.Age,
			cfg.Retention.IncludeDB,
			cfg.Retention.Location,
			cfg.Retention.OmitTagged)
	}

	if cfg.Compression > 0 {
		RegisterCompressionService(cfg.Compression)
	}

	lc := auth.Keys.LdapConfig

	if lc != nil && lc.SyncInterval != "" {
		RegisterLdapSyncService(lc.SyncInterval)
	}

	RegisterFootprintWorker()
	RegisterUpdateDurationWorker()
	RegisterCommitJobService()

	s.Start()
}

// Shutdown stops the task manager and its scheduler.
func Shutdown() {
	if s != nil {
		s.Shutdown()
	}
}
