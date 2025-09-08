// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

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

type Retention struct {
	Policy    string `json:"policy"`
	Location  string `json:"location"`
	Age       int    `json:"age"`
	IncludeDB bool   `json:"includeDB"`
}

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
	Keys    CronFrequency
)

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
			cfg.Retention.IncludeDB)
	case "move":
		RegisterRetentionMoveService(
			cfg.Retention.Age,
			cfg.Retention.IncludeDB,
			cfg.Retention.Location)
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

func Shutdown() {
	s.Shutdown()
}
