// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"encoding/json"
	"fmt"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
)

const NamedJobCacheInsert string = `INSERT INTO job_cache (
	job_id, hpc_user, project, cluster, subcluster, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc,
	shared, monitoring_status, smt, job_state, start_time, duration, walltime, footprint, energy, energy_footprint, resources, meta_data
) VALUES (
	:job_id, :hpc_user, :project, :cluster, :subcluster, :cluster_partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
  :shared, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :footprint,  :energy, :energy_footprint, :resources, :meta_data
);`

const NamedJobInsert string = `INSERT INTO job (
	job_id, hpc_user, project, cluster, subcluster, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc,
	shared, monitoring_status, smt, job_state, start_time, duration, walltime, footprint, energy, energy_footprint, resources, meta_data
) VALUES (
	:job_id, :hpc_user, :project, :cluster, :subcluster, :cluster_partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
  :shared, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :footprint,  :energy, :energy_footprint, :resources, :meta_data
);`

// InsertJobDirect inserts a job directly into the job table (not job_cache).
// Use this when the returned ID will be used for operations on the job table
// (e.g., adding tags), or for imported jobs that are already completed.
func (r *JobRepository) InsertJobDirect(job *schema.Job) (int64, error) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	res, err := r.DB.NamedExec(NamedJobInsert, job)
	if err != nil {
		cclog.Warn("Error while NamedJobInsert (direct)")
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		cclog.Warn("Error while getting last insert ID (direct)")
		return 0, err
	}

	return id, nil
}

func (r *JobRepository) InsertJob(job *schema.Job) (int64, error) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	res, err := r.DB.NamedExec(NamedJobCacheInsert, job)
	if err != nil {
		cclog.Warn("Error while NamedJobInsert")
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		cclog.Warn("Error while getting last insert ID")
		return 0, err
	}

	return id, nil
}

func (r *JobRepository) SyncJobs() ([]*schema.Job, error) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()

	query := sq.Select(jobCacheColumns...).From("job_cache")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Errorf("Error while running query %v", err)
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}
		jobs = append(jobs, job)
	}

	// Use INSERT OR IGNORE to skip jobs already transferred by the stop path
	_, err = r.DB.Exec(
		"INSERT OR IGNORE INTO job (job_id, cluster, subcluster, start_time, hpc_user, project, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc, shared, monitoring_status, smt, job_state, duration, walltime, footprint, energy, energy_footprint, resources, meta_data) SELECT job_id, cluster, subcluster, start_time, hpc_user, project, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc, shared, monitoring_status, smt, job_state, duration, walltime, footprint, energy, energy_footprint, resources, meta_data FROM job_cache")
	if err != nil {
		cclog.Warnf("Error while Job sync: %v", err)
		return nil, err
	}

	_, err = r.DB.Exec("DELETE FROM job_cache")
	if err != nil {
		cclog.Warnf("Error while Job cache clean: %v", err)
		return nil, err
	}

	// Resolve correct job.id from the job table. The IDs read from job_cache
	// are from a different auto-increment sequence and must not be used to
	// query the job table.
	for _, job := range jobs {
		var newID int64
		if err := sq.Select("job.id").From("job").
			Where("job.job_id = ? AND job.cluster = ? AND job.start_time = ?",
				job.JobID, job.Cluster, job.StartTime).
			RunWith(r.stmtCache).QueryRow().Scan(&newID); err != nil {
			cclog.Warnf("SyncJobs: could not resolve job table id for job %d on %s: %v",
				job.JobID, job.Cluster, err)
			continue
		}
		job.ID = &newID
	}

	return jobs, nil
}

// TransferCachedJobToMain moves a job from job_cache to the job table.
// Caller must hold r.Mutex. Returns the new job table ID.
func (r *JobRepository) TransferCachedJobToMain(cacheID int64) (int64, error) {
	res, err := r.DB.Exec(
		"INSERT INTO job (job_id, cluster, subcluster, start_time, hpc_user, project, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc, shared, monitoring_status, smt, job_state, duration, walltime, footprint, energy, energy_footprint, resources, meta_data) SELECT job_id, cluster, subcluster, start_time, hpc_user, project, cluster_partition, array_job_id, num_nodes, num_hwthreads, num_acc, shared, monitoring_status, smt, job_state, duration, walltime, footprint, energy, energy_footprint, resources, meta_data FROM job_cache WHERE id = ?",
		cacheID)
	if err != nil {
		return 0, fmt.Errorf("transferring cached job %d to main table failed: %w", cacheID, err)
	}

	newID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting new job ID after transfer failed: %w", err)
	}

	_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", cacheID)
	if err != nil {
		return 0, fmt.Errorf("deleting cached job %d after transfer failed: %w", cacheID, err)
	}

	return newID, nil
}

// Start inserts a new job in the table, returning the unique job ID.
// Statistics are not transfered!
func (r *JobRepository) Start(job *schema.Job) (id int64, err error) {
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

// StartDirect inserts a new job directly into the job table (not job_cache).
// Use this when the returned ID will immediately be used for job table
// operations such as adding tags.
func (r *JobRepository) StartDirect(job *schema.Job) (id int64, err error) {
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

	return r.InsertJobDirect(job)
}

// Stop updates the job with the database id jobId using the provided arguments.
func (r *JobRepository) Stop(
	jobID int64,
	duration int32,
	state schema.JobState,
	monitoringStatus int32,
) (err error) {
	// Invalidate cache entries as job state is changing
	r.cache.Del(fmt.Sprintf("metadata:%d", jobID))
	r.cache.Del(fmt.Sprintf("energyFootprint:%d", jobID))

	stmt := sq.Update("job").
		Set("job_state", state).
		Set("duration", duration).
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", jobID)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return err
}

