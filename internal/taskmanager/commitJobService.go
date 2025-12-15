// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterCommitJobService() {
	var frequency string
	if Keys.CommitJobWorker != "" {
		frequency = Keys.CommitJobWorker
	} else {
		frequency = "2m"
	}

	d, err := parseDuration(frequency)
	if err != nil {
		cclog.Errorf("RegisterCommitJobService: %v", err)
		return
	}

	cclog.Infof("register commitJob service with %s interval", frequency)

	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				start := time.Now()
				cclog.Debugf("jobcache sync started at %s\n", start.Format(time.RFC3339))
				jobs, _ := jobRepo.SyncJobs()
				repository.CallJobStartHooks(jobs)
				cclog.Debugf("jobcache sync and job callbacks are done and took %s\n", time.Since(start))
			}))
}
