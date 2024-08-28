// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archiver

import (
	"context"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var (
	archivePending sync.WaitGroup
	archiveChannel chan *schema.Job
	r              *repository.JobRepository
)

func Start(jobRepo *repository.JobRepository) {
	archiveChannel = make(chan *schema.Job, 128)

	go archivingWorker()
}

// Archiving worker thread
func archivingWorker() {
	for {
		select {
		case job, ok := <-archiveChannel:
			if !ok {
				break
			}
			start := time.Now()
			// not using meta data, called to load JobMeta into Cache?
			// will fail if job meta not in repository
			if _, err := r.FetchMetadata(job); err != nil {
				log.Errorf("archiving job (dbid: %d) failed at check metadata step: %s", job.ID, err.Error())
				r.UpdateMonitoringStatus(job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			// ArchiveJob will fetch all the data from a MetricDataRepository and push into configured archive backend
			// TODO: Maybe use context with cancel/timeout here
			jobMeta, err := ArchiveJob(job, context.Background())
			if err != nil {
				log.Errorf("archiving job (dbid: %d) failed at archiving job step: %s", job.ID, err.Error())
				r.UpdateMonitoringStatus(job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			if err := r.UpdateFootprint(jobMeta); err != nil {
				log.Errorf("archiving job (dbid: %d) failed at update Footprint step: %s", job.ID, err.Error())
				continue
			}
			// Update the jobs database entry one last time:
			if err := r.MarkArchived(jobMeta, schema.MonitoringStatusArchivingSuccessful); err != nil {
				log.Errorf("archiving job (dbid: %d) failed at marking archived step: %s", job.ID, err.Error())
				continue
			}
			log.Debugf("archiving job %d took %s", job.JobID, time.Since(start))
			log.Printf("archiving job (dbid: %d) successful", job.ID)
			archivePending.Done()
		}
	}
}

// Trigger async archiving
func TriggerArchiving(job *schema.Job) {
	archivePending.Add(1)
	archiveChannel <- job
}

// Wait for background thread to finish pending archiving operations
func WaitForArchiving() {
	// close channel and wait for worker to process remaining jobs
	archivePending.Wait()
}
