// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/go-co-op/gocron/v2"
)

func RegisterUpdateDurationWorker() {
	log.Info("Register duration update service")

	d, _ := time.ParseDuration("5m")
	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				start := time.Now()
				log.Printf("Update duration started at %s", start.Format(time.RFC3339))
				jobRepo.UpdateDuration()
				log.Print("Update duration is done and took %s", time.Since(start))
			}))
}
