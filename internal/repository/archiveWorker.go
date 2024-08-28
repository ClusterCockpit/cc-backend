// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

// Archiving worker thread
func (r *JobRepository) archivingWorker() {
	for {
		select {
		case job, ok := <-r.archiveChannel:
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

			// metricdata.ArchiveJob will fetch all the data from a MetricDataRepository and push into configured archive backend
			// TODO: Maybe use context with cancel/timeout here
			jobMeta, err := archiver.ArchiveJob(job, context.Background())
			if err != nil {
				log.Errorf("archiving job (dbid: %d) failed at archiving job step: %s", job.ID, err.Error())
				r.UpdateMonitoringStatus(job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			// Update the jobs database entry one last time:
			if err := r.MarkArchived(jobMeta, schema.MonitoringStatusArchivingSuccessful); err != nil {
				log.Errorf("archiving job (dbid: %d) failed at marking archived step: %s", job.ID, err.Error())
				continue
			}
			log.Debugf("archiving job %d took %s", job.JobID, time.Since(start))
			log.Printf("archiving job (dbid: %d) successful", job.ID)
			r.archivePending.Done()
		}
	}
}

// Stop updates the job with the database id jobId using the provided arguments.
func (r *JobRepository) MarkArchived(
	jobMeta *schema.JobMeta,
	monitoringStatus int32,
) error {
	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", jobMeta.JobID)

	sc, err := archive.GetSubCluster(jobMeta.Cluster, jobMeta.SubCluster)
	if err != nil {
		log.Errorf("cannot get subcluster: %s", err.Error())
		return err
	}
	footprint := make(map[string]float64)

	for _, fp := range sc.Footprint {
		footprint[fp] = LoadJobStat(jobMeta, fp)
	}

	var rawFootprint []byte

	if rawFootprint, err = json.Marshal(footprint); err != nil {
		log.Warnf("Error while marshaling footprint for job, DB ID '%v'", jobMeta.ID)
		return err
	}

	stmt = stmt.Set("footprint", rawFootprint)

	if _, err := stmt.RunWith(r.stmtCache).Exec(); err != nil {
		log.Warn("Error while marking job as archived")
		return err
	}
	return nil
}

func (r *JobRepository) UpdateMonitoringStatus(job int64, monitoringStatus int32) (err error) {
	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", job)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}

// Trigger async archiving
func (r *JobRepository) TriggerArchiving(job *schema.Job) {
	r.archivePending.Add(1)
	r.archiveChannel <- job
}

// Wait for background thread to finish pending archiving operations
func (r *JobRepository) WaitForArchiving() {
	// close channel and wait for worker to process remaining jobs
	r.archivePending.Wait()
}
