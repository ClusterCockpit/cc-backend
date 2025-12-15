// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskmanager

import (
	"runtime"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterStopJobsExceedTime() {
	cclog.Info("Register undead jobs service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(0o3, 0, 0))),
		gocron.NewTask(
			func() {
				err := jobRepo.StopJobsExceedingWalltimeBy(config.Keys.StopJobsExceedingWalltime)
				if err != nil {
					cclog.Warnf("Error while looking for jobs exceeding their walltime: %s", err.Error())
				}
				runtime.GC()
			}))
}
