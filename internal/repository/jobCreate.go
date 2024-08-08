// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

const NamedJobInsert string = `INSERT INTO job (
	job_id, user, project, cluster, subcluster, ` + "`partition`" + `, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, footprint, resources, meta_data
) VALUES (
	:job_id, :user, :project, :cluster, :subcluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
  :exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :footprint, :resources, :meta_data
);`

func (r *JobRepository) InsertJob(job *schema.JobMeta) (int64, error) {
	res, err := r.DB.NamedExec(NamedJobInsert, job)
	if err != nil {
		log.Warn("Error while NamedJobInsert")
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn("Error while getting last insert ID")
		return 0, err
	}

	return id, nil
}

// Start inserts a new job in the table, returning the unique job ID.
// Statistics are not transfered!
func (r *JobRepository) Start(job *schema.JobMeta) (id int64, err error) {
	job.RawFootprint, err = json.Marshal(job.Footprint)
	if err != nil {
		return -1, fmt.Errorf("REPOSITORY/JOB > encoding footprint field failed: %w", err)
	}

	job.RawResources, err = json.Marshal(job.Resources)
	if err != nil {
		return -1, fmt.Errorf("REPOSITORY/JOB > encoding resources field failed: %w", err)
	}

	job.RawMetaData, err = json.Marshal(job.MetaData)
	if err != nil {
		return -1, fmt.Errorf("REPOSITORY/JOB > encoding metaData field failed: %w", err)
	}

	return r.InsertJob(job)
}

// Stop updates the job with the database id jobId using the provided arguments.
func (r *JobRepository) Stop(
	jobId int64,
	duration int32,
	state schema.JobState,
	monitoringStatus int32,
) (err error) {
	stmt := sq.Update("job").
		Set("job_state", state).
		Set("duration", duration).
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", jobId)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}
