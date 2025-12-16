// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	sq "github.com/Masterminds/squirrel"
)

// Find executes a SQL query to find a specific batch job.
// The job is queried using the batch job id, the cluster name,
// and the start time of the job in UNIX epoch time seconds.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) Find(
	jobID *int64,
	cluster *string,
	startTime *int64,
) (*schema.Job, error) {
	start := time.Now()
	q := sq.Select(jobColumns...).From("job").
		Where("job.job_id = ?", *jobID)

	if cluster != nil {
		q = q.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job.start_time = ?", *startTime)
	}

	q = q.OrderBy("job.id DESC") // always use newest matching job by db id if more than one match

	cclog.Debugf("Timer Find %s", time.Since(start))
	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

func (r *JobRepository) FindCached(
	jobID *int64,
	cluster *string,
	startTime *int64,
) (*schema.Job, error) {
	q := sq.Select(jobCacheColumns...).From("job_cache").
		Where("job_cache.job_id = ?", *jobID)

	if cluster != nil {
		q = q.Where("job_cache.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job_cache.start_time = ?", *startTime)
	}

	q = q.OrderBy("job_cache.id DESC") // always use newest matching job by db id if more than one match

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindAll executes a SQL query to find all batch jobs matching the given criteria.
// Jobs are queried using the batch job id, and optionally filtered by cluster name
// and start time (UNIX epoch time seconds).
// It returns a slice of pointers to schema.Job data structures and an error variable.
// An empty slice is returned if no matching jobs are found.
func (r *JobRepository) FindAll(
	jobID *int64,
	cluster *string,
	startTime *int64,
) ([]*schema.Job, error) {
	start := time.Now()
	q := sq.Select(jobColumns...).From("job").
		Where("job.job_id = ?", *jobID)

	if cluster != nil {
		q = q.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job.start_time = ?", *startTime)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Error("Error while running query")
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*schema.Job, 0, 10)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}
		jobs = append(jobs, job)
	}
	cclog.Debugf("Timer FindAll %s", time.Since(start))
	return jobs, nil
}

// GetJobList returns job IDs for non-running jobs.
// This is useful to process large job counts and intended to be used
// together with FindById to process jobs one by one.
// Use limit and offset for pagination. Use limit=0 to get all results (not recommended for large datasets).
func (r *JobRepository) GetJobList(limit int, offset int) ([]int64, error) {
	query := sq.Select("id").From("job").
		Where("job.job_state != 'running'")

	// Add pagination if limit is specified
	if limit > 0 {
		query = query.Limit(uint64(limit)).Offset(uint64(offset))
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Error("Error while running query")
		return nil, err
	}
	defer rows.Close()

	jl := make([]int64, 0, 1000)
	for rows.Next() {
		var id int64
		err := rows.Scan(&id)
		if err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}
		jl = append(jl, id)
	}

	cclog.Infof("Return job count %d", len(jl))
	return jl, nil
}

// FindByID executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByID(ctx context.Context, jobID int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobID)

	q, qerr := SecurityCheck(ctx, q)
	if qerr != nil {
		return nil, qerr
	}

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindByIDWithUser executes a SQL query to find a specific batch job.
// The job is queried using the database id. The user is passed directly,
// instead as part of the context.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByIDWithUser(user *schema.User, jobID int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobID)

	q, qerr := SecurityCheckWithUser(user, q)
	if qerr != nil {
		return nil, qerr
	}

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindByIDDirect executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByIDDirect(jobID int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobID)
	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindByJobID executes a SQL query to find a specific batch job.
// The job is queried using the slurm id and the clustername.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByJobID(ctx context.Context, jobID int64, startTime int64, cluster string) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").
		Where("job.job_id = ?", jobID).
		Where("job.cluster = ?", cluster).
		Where("job.start_time = ?", startTime)

	q, qerr := SecurityCheck(ctx, q)
	if qerr != nil {
		return nil, qerr
	}

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// IsJobOwner executes a SQL query to find a specific batch job.
// The job is queried using the slurm id,a username and the cluster.
// It returns a bool.
// If job was found, user is owner: test err != sql.ErrNoRows
func (r *JobRepository) IsJobOwner(jobID int64, startTime int64, user string, cluster string) bool {
	q := sq.Select("id").
		From("job").
		Where("job.job_id = ?", jobID).
		Where("job.hpc_user = ?", user).
		Where("job.cluster = ?", cluster).
		Where("job.start_time = ?", startTime)

	_, err := scanJob(q.RunWith(r.stmtCache).QueryRow())
	return err != sql.ErrNoRows
}

func (r *JobRepository) FindConcurrentJobs(
	ctx context.Context,
	job *schema.Job,
) (*model.JobLinkResultList, error) {
	if job == nil {
		return nil, nil
	}

	query, qerr := SecurityCheck(ctx, sq.Select("job.id", "job.job_id", "job.start_time").From("job"))
	if qerr != nil {
		return nil, qerr
	}

	query = query.Where("cluster = ?", job.Cluster)
	var startTime int64
	var stopTime int64

	startTime = job.StartTime
	hostname := job.Resources[0].Hostname

	if job.State == schema.JobStateRunning {
		stopTime = time.Now().Unix()
	} else {
		stopTime = startTime + int64(job.Duration)
	}

	// Add 200s overlap for jobs start time at the end
	startTimeTail := startTime + 10
	stopTimeTail := stopTime - 200
	startTimeFront := startTime + 200

	queryRunning := query.Where("job.job_state = ?").Where("(job.start_time BETWEEN ? AND ? OR job.start_time < ?)",
		"running", startTimeTail, stopTimeTail, startTime)
	// Get At Least One Exact Hostname Match from JSON Resources Array in Database
	queryRunning = queryRunning.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, '$.hostname') = ?)", hostname)

	query = query.Where("job.job_state != ?").Where("((job.start_time BETWEEN ? AND ?) OR (job.start_time + job.duration) BETWEEN ? AND ? OR (job.start_time < ?) AND (job.start_time + job.duration) > ?)",
		"running", startTimeTail, stopTimeTail, startTimeFront, stopTimeTail, startTime, stopTime)
	// Get At Least One Exact Hostname Match from JSON Resources Array in Database
	query = query.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, '$.hostname') = ?)", hostname)

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Errorf("Error while running query: %v", err)
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.JobLink, 0, 10)
	queryString := fmt.Sprintf("cluster=%s", job.Cluster)

	for rows.Next() {
		var id, jobID, startTime sql.NullInt64

		if err = rows.Scan(&id, &jobID, &startTime); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			queryString += fmt.Sprintf("&jobId=%d", int(jobID.Int64))
			items = append(items,
				&model.JobLink{
					ID:    fmt.Sprint(id.Int64),
					JobID: int(jobID.Int64),
				})
		}
	}

	rows, err = queryRunning.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Errorf("Error while running query: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, jobID, startTime sql.NullInt64

		if err := rows.Scan(&id, &jobID, &startTime); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			queryString += fmt.Sprintf("&jobId=%d", int(jobID.Int64))
			items = append(items,
				&model.JobLink{
					ID:    fmt.Sprint(id.Int64),
					JobID: int(jobID.Int64),
				})
		}
	}

	cnt := len(items)

	return &model.JobLinkResultList{
		ListQuery: &queryString,
		Items:     items,
		Count:     &cnt,
	}, nil
}
