// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

var (
	jobRepoOnce     sync.Once
	jobRepoInstance *JobRepository
)

type JobRepository struct {
	DB *sqlx.DB

	stmtCache *sq.StmtCache
	cache     *lrucache.Cache
}

func GetJobRepository() *JobRepository {
	jobRepoOnce.Do(func() {
		db := GetConnection()

		jobRepoInstance = &JobRepository{
			DB:        db.DB,
			stmtCache: sq.NewStmtCache(db.DB),
			cache:     lrucache.New(1024 * 1024),
		}
	})

	return jobRepoInstance
}

var jobColumns []string = []string{
	"job.id", "job.job_id", "job.user", "job.project", "job.cluster", "job.subcluster", "job.start_time", "job.partition", "job.array_job_id",
	"job.num_nodes", "job.num_hwthreads", "job.num_acc", "job.exclusive", "job.monitoring_status", "job.smt", "job.job_state",
	"job.duration", "job.walltime", "job.resources", // "job.meta_data",
}

func scanJob(row interface{ Scan(...interface{}) error }) (*schema.Job, error) {
	job := &schema.Job{}
	if err := row.Scan(
		&job.ID, &job.JobID, &job.User, &job.Project, &job.Cluster, &job.SubCluster, &job.StartTimeUnix, &job.Partition, &job.ArrayJobId,
		&job.NumNodes, &job.NumHWThreads, &job.NumAcc, &job.Exclusive, &job.MonitoringStatus, &job.SMT, &job.State,
		&job.Duration, &job.Walltime, &job.RawResources /*&job.MetaData*/); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(job.RawResources, &job.Resources); err != nil {
		return nil, err
	}

	job.StartTime = time.Unix(job.StartTimeUnix, 0)
	if job.Duration == 0 && job.State == schema.JobStateRunning {
		job.Duration = int32(time.Since(job.StartTime).Seconds())
	}

	job.RawResources = nil
	return job, nil
}

func (r *JobRepository) FetchMetadata(job *schema.Job) (map[string]string, error) {
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		job.MetaData = cached.(map[string]string)
		return job.MetaData, nil
	}

	if err := sq.Select("job.meta_data").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawMetaData); err != nil {
		return nil, err
	}

	if len(job.RawMetaData) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawMetaData, &job.MetaData); err != nil {
		return nil, err
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	return job.MetaData, nil
}

func (r *JobRepository) UpdateMetadata(job *schema.Job, key, val string) (err error) {
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	r.cache.Del(cachekey)
	if job.MetaData == nil {
		if _, err = r.FetchMetadata(job); err != nil {
			return err
		}
	}

	if job.MetaData != nil {
		cpy := make(map[string]string, len(job.MetaData)+1)
		for k, v := range job.MetaData {
			cpy[k] = v
		}
		cpy[key] = val
		job.MetaData = cpy
	} else {
		job.MetaData = map[string]string{key: val}
	}

	if job.RawMetaData, err = json.Marshal(job.MetaData); err != nil {
		return err
	}

	if _, err = sq.Update("job").Set("meta_data", job.RawMetaData).Where("job.id = ?", job.ID).RunWith(r.stmtCache).Exec(); err != nil {
		return err
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	return nil
}

// Find executes a SQL query to find a specific batch job.
// The job is queried using the batch job id, the cluster name,
// and the start time of the job in UNIX epoch time seconds.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) Find(
	jobId *int64,
	cluster *string,
	startTime *int64) (*schema.Job, error) {

	q := sq.Select(jobColumns...).From("job").
		Where("job.job_id = ?", *jobId)

	if cluster != nil {
		q = q.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job.start_time = ?", *startTime)
	}

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
	startTime *int64) ([]*schema.Job, error) {

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
		return nil, err
	}

	jobs := make([]*schema.Job, 0, 10)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// FindById executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindById(jobId int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobId)
	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

// Start inserts a new job in the table, returning the unique job ID.
// Statistics are not transfered!
func (r *JobRepository) Start(job *schema.JobMeta) (id int64, err error) {
	job.RawResources, err = json.Marshal(job.Resources)
	if err != nil {
		return -1, fmt.Errorf("encoding resources field failed: %w", err)
	}

	job.RawMetaData, err = json.Marshal(job.MetaData)
	if err != nil {
		return -1, fmt.Errorf("encoding metaData field failed: %w", err)
	}

	res, err := r.DB.NamedExec(`INSERT INTO job (
		job_id, user, project, cluster, subcluster, `+"`partition`"+`, array_job_id, num_nodes, num_hwthreads, num_acc,
		exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, resources, meta_data
	) VALUES (
		:job_id, :user, :project, :cluster, :subcluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
		:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :resources, :meta_data
	);`, job)
	if err != nil {
		return -1, err
	}

	return res.LastInsertId()
}

// Stop updates the job with the database id jobId using the provided arguments.
func (r *JobRepository) Stop(
	jobId int64,
	duration int32,
	state schema.JobState,
	monitoringStatus int32) (err error) {

	stmt := sq.Update("job").
		Set("job_state", state).
		Set("duration", duration).
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", jobId)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}

// TODO: Use node hours instead: SELECT job.user, sum(job.num_nodes * (CASE WHEN job.job_state = "running" THEN CAST(strftime('%s', 'now') AS INTEGER) - job.start_time ELSE job.duration END)) as x FROM job GROUP BY user ORDER BY x DESC;
func (r *JobRepository) CountGroupedJobs(ctx context.Context, aggreg model.Aggregate, filters []*model.JobFilter, weight *model.Weights, limit *int) (map[string]int, error) {
	if !aggreg.IsValid() {
		return nil, errors.New("invalid aggregate")
	}

	runner := (sq.BaseRunner)(r.stmtCache)
	count := "count(*) as count"
	if weight != nil {
		switch *weight {
		case model.WeightsNodeCount:
			count = "sum(job.num_nodes) as count"
		case model.WeightsNodeHours:
			now := time.Now().Unix()
			count = fmt.Sprintf(`sum(job.num_nodes * (CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END)) as count`, now)
			runner = r.DB
		}
	}

	q := sq.Select("job."+string(aggreg), count).From("job").GroupBy("job." + string(aggreg)).OrderBy("count DESC")
	q = SecurityCheck(ctx, q)
	for _, f := range filters {
		q = BuildWhereClause(f, q)
	}
	if limit != nil {
		q = q.Limit(uint64(*limit))
	}

	counts := map[string]int{}
	rows, err := q.RunWith(runner).Query()
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var group string
		var count int
		if err := rows.Scan(&group, &count); err != nil {
			return nil, err
		}

		counts[group] = count
	}

	return counts, nil
}

func (r *JobRepository) UpdateMonitoringStatus(job int64, monitoringStatus int32) (err error) {
	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", job)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}

// Stop updates the job with the database id jobId using the provided arguments.
func (r *JobRepository) Archive(
	jobId int64,
	monitoringStatus int32,
	metricStats map[string]schema.JobStatistics) error {

	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", jobId)

	for metric, stats := range metricStats {
		switch metric {
		case "flops_any":
			stmt = stmt.Set("flops_any_avg", stats.Avg)
		case "mem_used":
			stmt = stmt.Set("mem_used_max", stats.Max)
		case "mem_bw":
			stmt = stmt.Set("mem_bw_avg", stats.Avg)
		case "load":
			stmt = stmt.Set("load_avg", stats.Avg)
		case "net_bw":
			stmt = stmt.Set("net_bw_avg", stats.Avg)
		case "file_bw":
			stmt = stmt.Set("file_bw_avg", stats.Avg)
		}
	}

	if _, err := stmt.RunWith(r.stmtCache).Exec(); err != nil {
		return err
	}
	return nil
}

var ErrNotFound = errors.New("no such job or user")

// FindJobOrUser returns a job database ID or a username if a job or user machtes the search term.
// As 0 is a valid job id, check if username is "" instead in order to check what machted.
// If nothing matches the search, `ErrNotFound` is returned.
func (r *JobRepository) FindJobOrUser(ctx context.Context, searchterm string) (job int64, username string, err error) {
	user := auth.GetUser(ctx)
	if id, err := strconv.Atoi(searchterm); err == nil {
		qb := sq.Select("job.id").From("job").Where("job.job_id = ?", id)
		if user != nil && !user.HasRole(auth.RoleAdmin) && !user.HasRole(auth.RoleSupport) {
			qb = qb.Where("job.user = ?", user.Username)
		}

		err := qb.RunWith(r.stmtCache).QueryRow().Scan(&job)
		if err != nil && err != sql.ErrNoRows {
			return 0, "", err
		} else if err == nil {
			return job, "", nil
		}
	}

	if user == nil || user.HasRole(auth.RoleAdmin) || user.HasRole(auth.RoleSupport) {
		err := sq.Select("job.user").Distinct().From("job").
			Where("job.user = ?", searchterm).
			RunWith(r.stmtCache).QueryRow().Scan(&username)
		if err != nil && err != sql.ErrNoRows {
			return 0, "", err
		} else if err == nil {
			return 0, username, nil
		}
	}

	return 0, "", ErrNotFound
}

func (r *JobRepository) Partitions(cluster string) ([]string, error) {
	var err error
	partitions := r.cache.Get("partitions:"+cluster, func() (interface{}, time.Duration, int) {
		parts := []string{}
		if err = r.DB.Select(&parts, `SELECT DISTINCT job.partition FROM job WHERE job.cluster = ?;`, cluster); err != nil {
			return nil, 0, 1000
		}

		return parts, 1 * time.Hour, 1
	})
	if err != nil {
		return nil, err
	}
	return partitions.([]string), nil
}

// AllocatedNodes returns a map of all subclusters to a map of hostnames to the amount of jobs running on that host.
// Hosts with zero jobs running on them will not show up!
func (r *JobRepository) AllocatedNodes(cluster string) (map[string]map[string]int, error) {
	subclusters := make(map[string]map[string]int)
	rows, err := sq.Select("resources", "subcluster").From("job").
		Where("job.job_state = 'running'").
		Where("job.cluster = ?", cluster).
		RunWith(r.stmtCache).Query()
	if err != nil {
		return nil, err
	}

	var raw []byte
	defer rows.Close()
	for rows.Next() {
		raw = raw[0:0]
		var resources []*schema.Resource
		var subcluster string
		if err := rows.Scan(&raw, &subcluster); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &resources); err != nil {
			return nil, err
		}

		hosts, ok := subclusters[subcluster]
		if !ok {
			hosts = make(map[string]int)
			subclusters[subcluster] = hosts
		}

		for _, resource := range resources {
			hosts[resource.Hostname] += 1
		}
	}

	return subclusters, nil
}

func (r *JobRepository) StopJobsExceedingWalltimeBy(seconds int) error {
	res, err := sq.Update("job").
		Set("monitoring_status", schema.MonitoringStatusArchivingFailed).
		Set("duration", 0).
		Set("job_state", schema.JobStateFailed).
		Where("job.job_state = 'running'").
		Where("job.walltime > 0").
		Where(fmt.Sprintf("(%d - job.start_time) > (job.walltime + %d)", time.Now().Unix(), seconds)).
		RunWith(r.DB).Exec()
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected > 0 {
		log.Warnf("%d jobs have been marked as failed due to running too long", rowsAffected)
	}
	return nil
}
