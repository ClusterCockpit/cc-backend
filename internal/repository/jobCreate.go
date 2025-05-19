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

const NamedJobCacheInsert string = `INSERT INTO job_cache (
	job_id, hpc_user, project, cluster, subcluster, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, footprint, energy, energy_footprint, resources, meta_data
) VALUES (
	:job_id, :hpc_user, :project, :cluster, :subcluster, :cluster_partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
  :exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :footprint,  :energy, :energy_footprint, :resources, :meta_data
);`

const NamedJobInsert string = `INSERT INTO job (
	job_id, hpc_user, project, cluster, subcluster, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, footprint, energy, energy_footprint, resources, meta_data
) VALUES (
	:job_id, :hpc_user, :project, :cluster, :subcluster, :cluster_partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
  :exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :footprint,  :energy, :energy_footprint, :resources, :meta_data
);`

func (r *JobRepository) InsertJob(job *schema.JobMeta) (int64, error) {
	r.Mutex.Lock()
	res, err := r.DB.NamedExec(NamedJobCacheInsert, job)
	r.Mutex.Unlock()
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

func (r *JobRepository) SyncJobs() ([]*schema.Job, error) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	query := sq.Select(jobColumns...).From("job_cache")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Errorf("Error while running query %v", err)
		return nil, err
	}

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			rows.Close()
			log.Warn("Error while scanning rows")
			return nil, err
		}
		jobs = append(jobs, job)
	}

	_, err = r.DB.Exec(
		"INSERT INTO job (job_id, cluster, subcluster, start_time, hpc_user, project, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc, exclusive, monitoring_status, smt, job_state, duration, walltime, footprint, energy, energy_footprint, resources, meta_data) SELECT job_id, cluster, subcluster, start_time, hpc_user, project, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc, exclusive, monitoring_status, smt, job_state, duration, walltime, footprint, energy, energy_footprint, resources, meta_data FROM job_cache")
	if err != nil {
		log.Warnf("Error while Job sync: %v", err)
		return nil, err
	}

	_, err = r.DB.Exec("DELETE FROM job_cache")
	if err != nil {
		log.Warnf("Error while Job cache clean: %v", err)
		return nil, err
	}

	return jobs, nil
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

func (r *JobRepository) StopCached(
	jobId int64,
	duration int32,
	state schema.JobState,
	monitoringStatus int32,
) (err error) {
	stmt := sq.Update("job_cache").
		Set("job_state", state).
		Set("duration", duration).
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", jobId)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}
