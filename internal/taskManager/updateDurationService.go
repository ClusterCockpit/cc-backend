// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterUpdateDurationWorker() {
	var frequency string
	if config.Keys.CronFrequency != nil && config.Keys.CronFrequency.DurationWorker != "" {
		frequency = config.Keys.CronFrequency.DurationWorker
	} else {
		frequency = "5m"
	}
	d, _ := time.ParseDuration(frequency)
	cclog.Infof("Register Duration Update service with %s interval", frequency)

	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				start := time.Now()
				cclog.Printf("Update duration started at %s", start.Format(time.RFC3339))
				jobRepo.UpdateDuration()
				cclog.Printf("Update duration is done and took %s", time.Since(start))
			}))
}
