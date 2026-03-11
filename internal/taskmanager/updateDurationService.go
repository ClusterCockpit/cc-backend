// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterUpdateDurationWorker() {
	var frequency string
	if Keys.DurationWorker != "" {
		frequency = Keys.DurationWorker
	} else {
		frequency = "5m"
	}

	d, err := parseDuration(frequency)
	if err != nil {
		cclog.Errorf("RegisterUpdateDurationWorker: %v", err)
		return
	}

	cclog.Infof("Register Duration Update service with %s interval", frequency)

	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				start := time.Now()
				cclog.Infof("Update duration started at %s", start.Format(time.RFC3339))
				jobRepo.UpdateDuration()
				cclog.Infof("Update duration is done and took %s", time.Since(start))
			}))
}
