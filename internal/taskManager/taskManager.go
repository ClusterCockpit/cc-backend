// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"encoding/json"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/go-co-op/gocron/v2"
)

var (
	s       gocron.Scheduler
	jobRepo *repository.JobRepository
)

func parseDuration(s string) (time.Duration, error) {
	interval, err := time.ParseDuration(s)
	if err != nil {
		log.Warnf("Could not parse duration for sync interval: %v",
			s)
		return 0, err
	}

	if interval == 0 {
		log.Info("TaskManager: Sync interval is zero")
	}

	return interval, nil
}

func Start() {
	var err error
	jobRepo = repository.GetJobRepository()
	s, err = gocron.NewScheduler()
	if err != nil {
		log.Abortf("Taskmanager Start: Could not create gocron scheduler.\nError: %s\n", err.Error())
	}

	if config.Keys.StopJobsExceedingWalltime > 0 {
		RegisterStopJobsExceedTime()
	}

	var cfg struct {
		Retention   schema.Retention `json:"retention"`
		Compression int              `json:"compression"`
	}
	cfg.Retention.IncludeDB = true

	if err := json.Unmarshal(config.Keys.Archive, &cfg); err != nil {
		log.Warn("Error while unmarshaling raw config json")
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

	lc := config.Keys.LdapConfig

	if lc != nil && lc.SyncInterval != "" {
		RegisterLdapSyncService(lc.SyncInterval)
	}

	RegisterFootprintWorker()
	RegisterUpdateDurationWorker()

	s.Start()
}

func Shutdown() {
	s.Shutdown()
}
