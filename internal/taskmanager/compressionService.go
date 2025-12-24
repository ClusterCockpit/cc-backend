// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-co-op/gocron/v2"
)

func RegisterCompressionService(compressOlderThan int) {
	cclog.Info("Register compression service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(5, 0, 0))),
		gocron.NewTask(
			func() {
				var jobs []*schema.Job
				var err error

				ar := archive.GetHandle()
				startTime := time.Now().Unix() - int64(compressOlderThan*24*3600)
				lastTime := ar.CompressLast(startTime)
				if startTime == lastTime {
					cclog.Info("Compression Service - Complete archive run")
					jobs, err = jobRepo.FindJobsBetween(0, startTime, false)

				} else {
					jobs, err = jobRepo.FindJobsBetween(lastTime, startTime, false)
				}

				if err != nil {
					cclog.Warnf("Error while looking for compression jobs: %v", err)
				}
				ar.Compress(jobs)
			}))
}
