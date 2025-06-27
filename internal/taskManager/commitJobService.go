// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/go-co-op/gocron/v2"
)

func RegisterCommitJobService() {
	var frequency string
	if config.Keys.CronFrequency != nil && config.Keys.CronFrequency.CommitJobWorker != "" {
		frequency = config.Keys.CronFrequency.CommitJobWorker
	} else {
		frequency = "2m"
	}
	d, _ := time.ParseDuration(frequency)
	log.Infof("Register commitJob service with %s interval", frequency)

	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				start := time.Now()
				log.Printf("Jobcache sync started at %s", start.Format(time.RFC3339))
				jobs, _ := jobRepo.SyncJobs()
				repository.CallJobStartHooks(jobs)
				log.Printf("Jobcache sync and job callbacks are done and took %s", time.Since(start))
			}))
}
