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

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
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
	DB     *sqlx.DB
	driver string

	stmtCache *sq.StmtCache
	cache     *lrucache.Cache

	archiveChannel chan *schema.Job
	archivePending sync.WaitGroup
}

func GetJobRepository() *JobRepository {
	jobRepoOnce.Do(func() {
		db := GetConnection()

		jobRepoInstance = &JobRepository{
			DB:     db.DB,
			driver: db.Driver,

			stmtCache:      sq.NewStmtCache(db.DB),
			cache:          lrucache.New(1024 * 1024),
			archiveChannel: make(chan *schema.Job, 128),
		}
		// start archiving worker
		go jobRepoInstance.archivingWorker()
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
		&job.Duration, &job.Walltime, &job.RawResources /*&job.RawMetaData*/); err != nil {
		log.Warnf("Error while scanning rows (Job): %v", err)
		return nil, err
	}

	if err := json.Unmarshal(job.RawResources, &job.Resources); err != nil {
		log.Warn("Error while unmarhsaling raw resources json")
		return nil, err
	}

	// if err := json.Unmarshal(job.RawMetaData, &job.MetaData); err != nil {
	// 	return nil, err
	// }

	job.StartTime = time.Unix(job.StartTimeUnix, 0)
	if job.Duration == 0 && job.State == schema.JobStateRunning {
		job.Duration = int32(time.Since(job.StartTime).Seconds())
	}

	job.RawResources = nil
	return job, nil
}

func (r *JobRepository) Optimize() error {
	var err error

	switch r.driver {
	case "sqlite3":
		if _, err = r.DB.Exec(`VACUUM`); err != nil {
			return err
		}
	case "mysql":
		log.Info("Optimize currently not supported for mysql driver")
	}

	return nil
}

func (r *JobRepository) Flush() error {
	var err error

	switch r.driver {
	case "sqlite3":
		if _, err = r.DB.Exec(`DELETE FROM jobtag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`DELETE FROM tag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`DELETE FROM job`); err != nil {
			return err
		}
	case "mysql":
		if _, err = r.DB.Exec(`SET FOREIGN_KEY_CHECKS = 0`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`TRUNCATE TABLE jobtag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`TRUNCATE TABLE tag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`TRUNCATE TABLE job`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`SET FOREIGN_KEY_CHECKS = 1`); err != nil {
			return err
		}
	}

	return nil
}

func scanJobLink(row interface{ Scan(...interface{}) error }) (*model.JobLink, error) {
	jobLink := &model.JobLink{}
	if err := row.Scan(
		&jobLink.ID, &jobLink.JobID); err != nil {
		log.Warn("Error while scanning rows (jobLink)")
		return nil, err
	}

	return jobLink, nil
}

func (r *JobRepository) FetchMetadata(job *schema.Job) (map[string]string, error) {
	start := time.Now()
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		job.MetaData = cached.(map[string]string)
		return job.MetaData, nil
	}

	if err := sq.Select("job.meta_data").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawMetaData); err != nil {
		log.Warn("Error while scanning for job metadata")
		return nil, err
	}

	if len(job.RawMetaData) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawMetaData, &job.MetaData); err != nil {
		log.Warn("Error while unmarshaling raw metadata json")
		return nil, err
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	log.Debugf("Timer FetchMetadata %s", time.Since(start))
	return job.MetaData, nil
}

func (r *JobRepository) UpdateMetadata(job *schema.Job, key, val string) (err error) {
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	r.cache.Del(cachekey)
	if job.MetaData == nil {
		if _, err = r.FetchMetadata(job); err != nil {
			log.Warnf("Error while fetching metadata for job, DB ID '%v'", job.ID)
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
		log.Warnf("Error while marshaling metadata for job, DB ID '%v'", job.ID)
		return err
	}

	if _, err = sq.Update("job").Set("meta_data", job.RawMetaData).Where("job.id = ?", job.ID).RunWith(r.stmtCache).Exec(); err != nil {
		log.Warnf("Error while updating metadata for job, DB ID '%v'", job.ID)
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

	start := time.Now()
	q := sq.Select(jobColumns...).From("job").
		Where("job.job_id = ?", *jobId)

	if cluster != nil {
		q = q.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		q = q.Where("job.start_time = ?", *startTime)
	}

	log.Debugf("Timer Find %s", time.Since(start))
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

// FindById executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindById(jobId int64) (*schema.Job, error) {
	q := sq.Select(jobColumns...).
		From("job").Where("job.id = ?", jobId)
	return scanJob(q.RunWith(r.stmtCache).QueryRow())
}

func (r *JobRepository) FindConcurrentJobs(
	ctx context.Context,
	job *schema.Job) (*model.JobLinkResultList, error) {
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
	queryRunning = queryRunning.Where("job.resources LIKE ?", fmt.Sprint("%", hostname, "%"))

	query = query.Where("job.job_state != ?").Where("((job.start_time BETWEEN ? AND ?) OR (job.start_time + job.duration) BETWEEN ? AND ? OR (job.start_time < ?) AND (job.start_time + job.duration) > ?)",
		"running", startTimeTail, stopTimeTail, startTimeFront, stopTimeTail, startTime, stopTime)
	query = query.Where("job.resources LIKE ?", fmt.Sprint("%", hostname, "%"))

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

// Start inserts a new job in the table, returning the unique job ID.
// Statistics are not transfered!
func (r *JobRepository) Start(job *schema.JobMeta) (id int64, err error) {
	job.RawResources, err = json.Marshal(job.Resources)
	if err != nil {
		return -1, fmt.Errorf("REPOSITORY/JOB > encoding resources field failed: %w", err)
	}

	job.RawMetaData, err = json.Marshal(job.MetaData)
	if err != nil {
		return -1, fmt.Errorf("REPOSITORY/JOB > encoding metaData field failed: %w", err)
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

func (r *JobRepository) DeleteJobsBefore(startTime int64) (int, error) {
	var cnt int
	qs := fmt.Sprintf("SELECT count(*) FROM job WHERE job.start_time < %d", startTime)
	err := r.DB.Get(&cnt, qs) //ignore error as it will also occur in delete statement
	_, err = r.DB.Exec(`DELETE FROM job WHERE job.start_time < ?`, startTime)
	if err != nil {
		log.Errorf(" DeleteJobsBefore(%d): error %#v", startTime, err)
	} else {
		log.Debugf("DeleteJobsBefore(%d): Deleted %d jobs", startTime, cnt)
	}
	return cnt, err
}

func (r *JobRepository) DeleteJobById(id int64) error {
	_, err := r.DB.Exec(`DELETE FROM job WHERE job.id = ?`, id)
	if err != nil {
		log.Errorf("DeleteJobById(%d): error %#v", id, err)
	} else {
		log.Debugf("DeleteJobById(%d): Success", id)
	}
	return err
}

func (r *JobRepository) UpdateMonitoringStatus(job int64, monitoringStatus int32) (err error) {
	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", job)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}

// Stop updates the job with the database id jobId using the provided arguments.
func (r *JobRepository) MarkArchived(
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
		case "cpu_load":
			stmt = stmt.Set("load_avg", stats.Avg)
		case "net_bw":
			stmt = stmt.Set("net_bw_avg", stats.Avg)
		case "file_bw":
			stmt = stmt.Set("file_bw_avg", stats.Avg)
		default:
			log.Debugf("MarkArchived() Metric '%v' unknown", metric)
		}
	}

	if _, err := stmt.RunWith(r.stmtCache).Exec(); err != nil {
		log.Warn("Error while marking job as archived")
		return err
	}
	return nil
}

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
				log.Errorf("archiving job (dbid: %d) failed: %s", job.ID, err.Error())
				r.UpdateMonitoringStatus(job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			// metricdata.ArchiveJob will fetch all the data from a MetricDataRepository and push into configured archive backend
			// TODO: Maybe use context with cancel/timeout here
			jobMeta, err := metricdata.ArchiveJob(job, context.Background())
			if err != nil {
				log.Errorf("archiving job (dbid: %d) failed: %s", job.ID, err.Error())
				r.UpdateMonitoringStatus(job.ID, schema.MonitoringStatusArchivingFailed)
				continue
			}

			// Update the jobs database entry one last time:
			if err := r.MarkArchived(job.ID, schema.MonitoringStatusArchivingSuccessful, jobMeta.Statistics); err != nil {
				log.Errorf("archiving job (dbid: %d) failed: %s", job.ID, err.Error())
				continue
			}
			log.Debugf("archiving job %d took %s", job.JobID, time.Since(start))
			log.Printf("archiving job (dbid: %d) successful", job.ID)
			r.archivePending.Done()
		}
	}
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

func (r *JobRepository) FindUserOrProjectOrJobname(user *schema.User, searchterm string) (jobid string, username string, project string, jobname string) {
	if _, err := strconv.Atoi(searchterm); err == nil { // Return empty on successful conversion: parent method will redirect for integer jobId
		return searchterm, "", "", ""
	} else { // Has to have letters and logged-in user for other guesses
		if user != nil {
			// Find username in jobs (match)
			uresult, _ := r.FindColumnValue(user, searchterm, "job", "user", "user", false)
			if uresult != "" {
				return "", uresult, "", ""
			}
			// Find username by name (like)
			nresult, _ := r.FindColumnValue(user, searchterm, "user", "username", "name", true)
			if nresult != "" {
				return "", nresult, "", ""
			}
			// Find projectId in jobs (match)
			presult, _ := r.FindColumnValue(user, searchterm, "job", "project", "project", false)
			if presult != "" {
				return "", "", presult, ""
			}
		}
		// Return searchterm if no match before: Forward as jobname query to GQL in handleSearchbar function
		return "", "", "", searchterm
	}
}

var ErrNotFound = errors.New("no such jobname, project or user")
var ErrForbidden = errors.New("not authorized")

func (r *JobRepository) FindColumnValue(user *schema.User, searchterm string, table string, selectColumn string, whereColumn string, isLike bool) (result string, err error) {
	compareStr := " = ?"
	query := searchterm
	if isLike {
		compareStr = " LIKE ?"
		query = "%" + searchterm + "%"
	}
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		theQuery := sq.Select(table+"."+selectColumn).Distinct().From(table).
			Where(table+"."+whereColumn+compareStr, query)

		// theSql, args, theErr := theQuery.ToSql()
		// if theErr != nil {
		// 	log.Warn("Error while converting query to sql")
		// 	return "", err
		// }
		// log.Debugf("SQL query (FindColumnValue): `%s`, args: %#v", theSql, args)

		err := theQuery.RunWith(r.stmtCache).QueryRow().Scan(&result)

		if err != nil && err != sql.ErrNoRows {
			return "", err
		} else if err == nil {
			return result, nil
		}
		return "", ErrNotFound
	} else {
		log.Infof("Non-Admin User %s : Requested Query '%s' on table '%s' : Forbidden", user.Name, query, table)
		return "", ErrForbidden
	}
}

func (r *JobRepository) FindColumnValues(user *schema.User, query string, table string, selectColumn string, whereColumn string) (results []string, err error) {
	emptyResult := make([]string, 0)
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		rows, err := sq.Select(table+"."+selectColumn).Distinct().From(table).
			Where(table+"."+whereColumn+" LIKE ?", fmt.Sprint("%", query, "%")).
			RunWith(r.stmtCache).Query()
		if err != nil && err != sql.ErrNoRows {
			return emptyResult, err
		} else if err == nil {
			for rows.Next() {
				var result string
				err := rows.Scan(&result)
				if err != nil {
					rows.Close()
					log.Warnf("Error while scanning rows: %v", err)
					return emptyResult, err
				}
				results = append(results, result)
			}
			return results, nil
		}
		return emptyResult, ErrNotFound

	} else {
		log.Infof("Non-Admin User %s : Requested Query '%s' on table '%s' : Forbidden", user.Name, query, table)
		return emptyResult, ErrForbidden
	}
}

func (r *JobRepository) Partitions(cluster string) ([]string, error) {
	var err error
	start := time.Now()
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
	log.Debugf("Timer Partitions %s", time.Since(start))
	return partitions.([]string), nil
}

// AllocatedNodes returns a map of all subclusters to a map of hostnames to the amount of jobs running on that host.
// Hosts with zero jobs running on them will not show up!
func (r *JobRepository) AllocatedNodes(cluster string) (map[string]map[string]int, error) {

	start := time.Now()
	subclusters := make(map[string]map[string]int)
	rows, err := sq.Select("resources", "subcluster").From("job").
		Where("job.job_state = 'running'").
		Where("job.cluster = ?", cluster).
		RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	var raw []byte
	defer rows.Close()
	for rows.Next() {
		raw = raw[0:0]
		var resources []*schema.Resource
		var subcluster string
		if err := rows.Scan(&raw, &subcluster); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}
		if err := json.Unmarshal(raw, &resources); err != nil {
			log.Warn("Error while unmarshaling raw resources json")
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

	log.Debugf("Timer AllocatedNodes %s", time.Since(start))
	return subclusters, nil
}

func (r *JobRepository) StopJobsExceedingWalltimeBy(seconds int) error {

	start := time.Now()
	res, err := sq.Update("job").
		Set("monitoring_status", schema.MonitoringStatusArchivingFailed).
		Set("duration", 0).
		Set("job_state", schema.JobStateFailed).
		Where("job.job_state = 'running'").
		Where("job.walltime > 0").
		Where(fmt.Sprintf("(%d - job.start_time) > (job.walltime + %d)", time.Now().Unix(), seconds)).
		RunWith(r.DB).Exec()
	if err != nil {
		log.Warn("Error while stopping jobs exceeding walltime")
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		log.Warn("Error while fetching affected rows after stopping due to exceeded walltime")
		return err
	}

	if rowsAffected > 0 {
		log.Infof("%d jobs have been marked as failed due to running too long", rowsAffected)
	}
	log.Debugf("Timer StopJobsExceedingWalltimeBy %s", time.Since(start))
	return nil
}

func (r *JobRepository) FindJobsBetween(startTimeBegin int64, startTimeEnd int64) ([]*schema.Job, error) {

	var query sq.SelectBuilder

	if startTimeBegin == startTimeEnd || startTimeBegin > startTimeEnd {
		return nil, errors.New("startTimeBegin is equal or larger startTimeEnd")
	}

	if startTimeBegin == 0 {
		log.Infof("Find jobs before %d", startTimeEnd)
		query = sq.Select(jobColumns...).From("job").Where(fmt.Sprintf(
			"job.start_time < %d", startTimeEnd))
	} else {
		log.Infof("Find jobs between %d and %d", startTimeBegin, startTimeEnd)
		query = sq.Select(jobColumns...).From("job").Where(fmt.Sprintf(
			"job.start_time BETWEEN %d AND %d", startTimeBegin, startTimeEnd))
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
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

	log.Infof("Return job count %d", len(jobs))
	return jobs, nil
}

const NamedJobInsert string = `INSERT INTO job (
	job_id, user, project, cluster, subcluster, ` + "`partition`" + `, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, resources, meta_data,
	mem_used_max, flops_any_avg, mem_bw_avg, load_avg, net_bw_avg, net_data_vol_total, file_bw_avg, file_data_vol_total
) VALUES (
	:job_id, :user, :project, :cluster, :subcluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
	:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :resources, :meta_data,
	:mem_used_max, :flops_any_avg, :mem_bw_avg, :load_avg, :net_bw_avg, :net_data_vol_total, :file_bw_avg, :file_data_vol_total
);`

func (r *JobRepository) InsertJob(job *schema.Job) (int64, error) {
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
