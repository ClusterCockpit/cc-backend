// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archiver

import (
	"context"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	sq "github.com/Masterminds/squirrel"
)

var (
	archivePending sync.WaitGroup
	archiveChannel chan *schema.Job
	jobRepo        *repository.JobRepository
)

func Start(r *repository.JobRepository) {
	archiveChannel = make(chan *schema.Job, 128)
	jobRepo = r

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
			if _, err := jobRepo.FetchMetadata(job); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at check metadata step: %s", job.ID, err.Error())
				jobRepo.UpdateMonitoringStatus(*job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			// ArchiveJob will fetch all the data from a MetricDataRepository and push into configured archive backend
			// TODO: Maybe use context with cancel/timeout here
			jobMeta, err := ArchiveJob(job, context.Background())
			if err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at archiving job step: %s", job.ID, err.Error())
				jobRepo.UpdateMonitoringStatus(*job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			stmt := sq.Update("job").Where("job.id = ?", job.ID)

			if stmt, err = jobRepo.UpdateFootprint(stmt, jobMeta); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at update Footprint step: %s", job.ID, err.Error())
				continue
			}
			if stmt, err = jobRepo.UpdateEnergy(stmt, jobMeta); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at update Energy step: %s", job.ID, err.Error())
				continue
			}
			// Update the jobs database entry one last time:
			stmt = jobRepo.MarkArchived(stmt, schema.MonitoringStatusArchivingSuccessful)
			if err := jobRepo.Execute(stmt); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at db execute: %s", job.ID, err.Error())
				continue
			}
			cclog.Debugf("archiving job %d took %s", job.JobID, time.Since(start))
			cclog.Printf("archiving job (dbid: %d) successful", job.ID)

			repository.CallJobStopHooks(job)
			archivePending.Done()
		default:
			continue
		}
	}
}

// Trigger async archiving
func TriggerArchiving(job *schema.Job) {
	if archiveChannel == nil {
		cclog.Fatal("Cannot archive without archiving channel. Did you Start the archiver?")
	}

	archivePending.Add(1)
	archiveChannel <- job
}

// Wait for background thread to finish pending archiving operations
func WaitForArchiving() {
	// close channel and wait for worker to process remaining jobs
	archivePending.Wait()
}
