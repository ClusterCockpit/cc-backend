// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/go-co-op/gocron/v2"
)

func RegisterCompressionService(compressOlderThan int) {
	log.Info("Register compression service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(05, 0, 0))),
		gocron.NewTask(
			func() {
				var jobs []*schema.Job
				var err error

				ar := archive.GetHandle()
				startTime := time.Now().Unix() - int64(compressOlderThan*24*3600)
				lastTime := ar.CompressLast(startTime)
				if startTime == lastTime {
					log.Info("Compression Service - Complete archive run")
					jobs, err = jobRepo.FindJobsBetween(0, startTime)

				} else {
					jobs, err = jobRepo.FindJobsBetween(lastTime, startTime)
				}

				if err != nil {
					log.Warnf("Error while looking for compression jobs: %v", err)
				}
				ar.Compress(jobs)
			}))
}
