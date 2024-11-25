// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type JobWithUser struct {
	Job  *schema.JobMeta
	User *schema.User
}

var (
	jobStartPending sync.WaitGroup
	jobStartChannel chan JobWithUser
)

func startJobStartWorker() {
	jobStartChannel = make(chan JobWithUser, 128)

	go jobStartWorker()
}

// Archiving worker thread
func jobStartWorker() {
	for {
		select {
		case req, ok := <-jobStartChannel:
			if !ok {
				break
			}
			jobRepo := GetJobRepository()

			id, err := jobRepo.Start(req.Job)
			if err != nil {
				log.Errorf("insert into database failed: %v", err)
			}

			for _, tag := range req.Job.Tags {
				if _, err := jobRepo.AddTagOrCreate(req.User, id, tag.Type, tag.Name, tag.Scope); err != nil {
					log.Errorf("adding tag to new job %d failed: %v", id, err)
				}
			}

			jobStartPending.Done()
		}
	}
}

// Trigger async archiving
func TriggerJobStart(req JobWithUser) {
	if jobStartChannel == nil {
		log.Fatal("Cannot start Job without jobStart channel. Did you Start the worker?")
	}

	jobStartPending.Add(1)
	jobStartChannel <- req
}

// Wait for background thread to finish pending archiving operations
func WaitForJobStart() {
	// close channel and wait for worker to process remaining jobs
	jobStartPending.Wait()
}
