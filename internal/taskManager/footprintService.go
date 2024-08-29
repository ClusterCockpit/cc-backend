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

func registerFootprintWorker() {
	log.Info("Register Footprint Update service")
	d, _ := time.ParseDuration("10m")
	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				t := time.Now()
				log.Printf("Update Footprints started at %s", t.Format(time.RFC3339))

				log.Print("Update Footprints done")
			}))
}
