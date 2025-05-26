// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

// Find executes a SQL query to find a specific batch job.
// The job is queried using the batch job id, the cluster name,
// and the start time of the job in UNIX epoch time seconds.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) Find(
	jobId *int64,
	cluster *string,
	startTime *int64,
) (*schema.Job, error) {
	start := time.Now()
	q := sq.Select(jobColumns...).From("job").
		Where("job.job_id = ?", *jobId)

	if cluster != nil {
		q = q.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job.start_time = ?", *startTime)
	}

	q = q.OrderBy("job.id DESC") // always use newest matching job by db id if more than one match

	log.Debugf("Timer Find %s", time.Since(start))
	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

func (r *JobRepository) FindCached(
	jobId *int64,
	cluster *string,
	startTime *int64,
) (*schema.Job, error) {
	q := sq.Select(jobCacheColumns...).From("job_cache").
		Where("job_cache.job_id = ?", *jobId)

	if cluster != nil {
		q = q.Where("job_cache.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job_cache.start_time = ?", *startTime)
	}

	q = q.OrderBy("job_cache.id DESC") // always use newest matching job by db id if more than one match

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// Find executes a SQL query to find a specific batch job.
// The job is queried using the batch job id, the cluster name,
// and the start time of the job in UNIX epoch time seconds.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindAll(
	jobId *int64,
	cluster *string,
	startTime *int64,
) ([]*schema.Job, error) {
	start := time.Now()
	q := sq.Select(jobColumns...).From("job").
		Where("job.job_id = ?", *jobId)

	if cluster != nil {
		q = q.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job.start_time = ?", *startTime)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	jobs := make([]*schema.Job, 0, 10)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}
		jobs = append(jobs, job)
	}
	log.Debugf("Timer FindAll %s", time.Since(start))
	return jobs, nil
}

// Get complete joblist only consisting of db ids.
// This is useful to process large job counts and intended to be used
// together with FindById to process jobs one by one
func (r *JobRepository) GetJobList() ([]int64, error) {
	query := sq.Select("id").From("job").
		Where("job.job_state != 'running'")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	jl := make([]int64, 0, 1000)
	for rows.Next() {
		var id int64
		err := rows.Scan(&id)
		if err != nil {
			rows.Close()
			log.Warn("Error while scanning rows")
			return nil, err
		}
		jl = append(jl, id)
	}

	log.Infof("Return job count %d", len(jl))
	return jl, nil
}

// FindById executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindById(ctx context.Context, jobId int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobId)

	q, qerr := SecurityCheck(ctx, q)
	if qerr != nil {
		return nil, qerr
	}

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindByIdWithUser executes a SQL query to find a specific batch job.
// The job is queried using the database id. The user is passed directly,
// instead as part of the context.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByIdWithUser(user *schema.User, jobId int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobId)

	q, qerr := SecurityCheckWithUser(user, q)
	if qerr != nil {
		return nil, qerr
	}

	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindByIdDirect executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByIdDirect(jobId int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobId)
	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// FindByJobId executes a SQL query to find a specific batch job.
// The job is queried using the slurm id and the clustername.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindByJobId(ctx context.Context, jobId int64, startTime int64, cluster string) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").
		Where("job.job_id = ?", jobId).
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
func (r *JobRepository) IsJobOwner(jobId int64, startTime int64, user string, cluster string) bool {
	q := sq.Select("id").
		From("job").
		Where("job.job_id = ?", jobId).
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

	startTime = job.StartTimeUnix
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
		log.Errorf("Error while running query: %v", err)
		return nil, err
	}

	items := make([]*model.JobLink, 0, 10)
	queryString := fmt.Sprintf("cluster=%s", job.Cluster)

	for rows.Next() {
		var id, jobId, startTime sql.NullInt64

		if err = rows.Scan(&id, &jobId, &startTime); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			queryString += fmt.Sprintf("&jobId=%d", int(jobId.Int64))
			items = append(items,
				&model.JobLink{
					ID:    fmt.Sprint(id.Int64),
					JobID: int(jobId.Int64),
				})
		}
	}

	rows, err = queryRunning.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Errorf("Error while running query: %v", err)
		return nil, err
	}

	for rows.Next() {
		var id, jobId, startTime sql.NullInt64

		if err := rows.Scan(&id, &jobId, &startTime); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			queryString += fmt.Sprintf("&jobId=%d", int(jobId.Int64))
			items = append(items,
				&model.JobLink{
					ID:    fmt.Sprint(id.Int64),
					JobID: int(jobId.Int64),
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
