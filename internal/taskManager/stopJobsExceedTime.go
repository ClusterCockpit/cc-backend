// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"runtime"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/go-co-op/gocron/v2"
)

func RegisterStopJobsExceedTime() {
	log.Info("Register undead jobs service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(03, 0, 0))),
		gocron.NewTask(
			func() {
				err := jobRepo.StopJobsExceedingWalltimeBy(config.Keys.StopJobsExceedingWalltime)
				if err != nil {
					log.Warnf("Error while looking for jobs exceeding their walltime: %s", err.Error())
				}
				runtime.GC()
			}))
}
