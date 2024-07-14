// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/go-co-op/gocron/v2"
)

var (
	s       gocron.Scheduler
	jobRepo *repository.JobRepository
)

func init() {
	var err error
	jobRepo = repository.GetJobRepository()
	s, err = gocron.NewScheduler()
	if err != nil {
		log.Fatalf("Error while creating gocron scheduler: %s", err.Error())
	}
}

func Shutdown() {
	s.Shutdown()
}
