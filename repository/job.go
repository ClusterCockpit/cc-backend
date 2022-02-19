package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/graph/model"
	"github.com/ClusterCockpit/cc-backend/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

type JobRepository struct {
	DB *sqlx.DB
}

func (r *JobRepository) Init() error {
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

	qb := sq.Select(schema.JobColumns...).From("job").
		Where("job.job_id = ?", jobId)

	if cluster != nil {
		qb = qb.Where("job.cluster = ?", *cluster)
	}
	if startTime != nil {
		qb = qb.Where("job.start_time = ?", *startTime)
	}

	sqlQuery, args, err := qb.ToSql()
	if err != nil {
		return nil, err
	}

	return schema.ScanJob(r.DB.QueryRowx(sqlQuery, args...))
}

// FindById executes a SQL query to find a specific batch job.
// The job is queried using the database id.
// It returns a pointer to a schema.Job data structure and an error variable.
// To check if no job was found test err == sql.ErrNoRows
func (r *JobRepository) FindById(
	jobId int64) (*schema.Job, error) {
	sqlQuery, args, err := sq.Select(schema.JobColumns...).
		From("job").Where("job.id = ?", jobId).ToSql()
	if err != nil {
		return nil, err
	}

	return schema.ScanJob(r.DB.QueryRowx(sqlQuery, args...))
}

// Start inserts a new job in the table, returning the unique job ID.
// Statistics are not transfered!
func (r *JobRepository) Start(job *schema.JobMeta) (id int64, err error) {
	res, err := r.DB.NamedExec(`INSERT INTO job (
		job_id, user, project, cluster, `+"`partition`"+`, array_job_id, num_nodes, num_hwthreads, num_acc,
		exclusive, monitoring_status, smt, job_state, start_time, duration, resources, meta_data
	) VALUES (
		:job_id, :user, :project, :cluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
		:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :resources, :meta_data
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

	_, err = stmt.RunWith(r.DB).Exec()
	return
}

func (r *JobRepository) CountGroupedJobs(ctx context.Context, aggreg model.Aggregate, filters []*model.JobFilter, limit *int) (map[string]int, error) {
	if !aggreg.IsValid() {
		return nil, errors.New("invalid aggregate")
	}

	q := sq.Select("job."+string(aggreg), "count(*) as count").From("job").GroupBy("job." + string(aggreg)).OrderBy("count DESC")
	q = SecurityCheck(ctx, q)
	for _, f := range filters {
		q = BuildWhereClause(f, q)
	}
	if limit != nil {
		q = q.Limit(uint64(*limit))
	}

	counts := map[string]int{}
	rows, err := q.RunWith(r.DB).Query()
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

	_, err = stmt.RunWith(r.DB).Exec()
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

	if _, err := stmt.RunWith(r.DB).Exec(); err != nil {
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
		if user != nil && !user.HasRole(auth.RoleAdmin) {
			qb = qb.Where("job.user = ?", user.Username)
		}

		err := qb.RunWith(r.DB).QueryRow().Scan(&job)
		if err != nil && err != sql.ErrNoRows {
			return 0, "", err
		} else if err == nil {
			return job, "", nil
		}
	}

	if user == nil || user.HasRole(auth.RoleAdmin) {
		err := sq.Select("job.user").Distinct().From("job").
			Where("job.user = ?", searchterm).
			RunWith(r.DB).QueryRow().Scan(&username)
		if err != nil && err != sql.ErrNoRows {
			return 0, "", err
		} else if err == nil {
			return 0, username, nil
		}
	}

	return 0, "", ErrNotFound
}
