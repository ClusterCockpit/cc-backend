package repository

import (
	"database/sql"

	"github.com/ClusterCockpit/cc-backend/log"
	"github.com/ClusterCockpit/cc-backend/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

type JobRepository struct {
	DB *sqlx.DB
}

func (r *JobRepository) FindJobById(
	jobId int64) (*schema.Job, error) {
	sql, args, err := sq.Select(schema.JobColumns...).
		From("job").Where("job.id = ?", jobId).ToSql()
	if err != nil {
		return nil, err
	}

	job, err := schema.ScanJob(r.DB.QueryRowx(sql, args...))
	return job, err
}

// func (r *JobRepository) FindJobsByFilter( ) ([]*schema.Job, int, error) {

// }

func (r *JobRepository) FindJobByIdWithUser(
	jobId int64,
	username string) (*schema.Job, error) {

	sql, args, err := sq.Select(schema.JobColumns...).
		From("job").
		Where("job.id = ?", jobId).
		Where("job.user = ?", username).ToSql()
	if err != nil {
		return nil, err
	}

	job, err := schema.ScanJob(r.DB.QueryRowx(sql, args...))
	return job, err
}

func (r *JobRepository) JobExists(
	jobId int64,
	cluster string,
	startTime int64) (rows *sql.Rows, err error) {
	rows, err = r.DB.Query(`SELECT job.id FROM job WHERE job.job_id = ? AND job.cluster = ? AND job.start_time = ?`,
		jobId, cluster, startTime)
	return
}

func (r *JobRepository) Add(job schema.JobMeta) (res sql.Result, err error) {
	res, err = r.DB.NamedExec(`INSERT INTO job (
		job_id, user, project, cluster, `+"`partition`"+`, array_job_id, num_nodes, num_hwthreads, num_acc,
		exclusive, monitoring_status, smt, job_state, start_time, duration, resources, meta_data
	) VALUES (
		:job_id, :user, :project, :cluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
		:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :resources, :meta_data
	);`, job)
	return
}

func (r *JobRepository) Stop(
	jobId int64,
	cluster string,
	startTime int64) (job *schema.Job, err error) {
	var sql string
	var args []interface{}
	qb := sq.Select(schema.JobColumns...).From("job").
		Where("job.job_id = ?", jobId).
		Where("job.cluster = ?", cluster)
	if startTime != 0 {
		qb = qb.Where("job.start_time = ?", startTime)
	}
	sql, args, err = qb.ToSql()
	if err != nil {
		return
	}
	job, err = schema.ScanJob(r.DB.QueryRowx(sql, args...))
	return
}

func (r *JobRepository) StopById(id string) (job *schema.Job, err error) {
	var sql string
	var args []interface{}
	qb := sq.Select(schema.JobColumns...).From("job").
		Where("job.id = ?", id)
	sql, args, err = qb.ToSql()
	if err != nil {
		return
	}
	job, err = schema.ScanJob(r.DB.QueryRowx(sql, args...))
	return
}

func (r *JobRepository) Close(
	jobId int64,
	duration int32,
	state schema.JobState,
	metricStats map[string]schema.JobStatistics) {

	stmt := sq.Update("job").
		Set("job_state", state).
		Set("duration", duration).
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

	sql, args, err := stmt.ToSql()
	if err != nil {
		log.Errorf("archiving job (dbid: %d) failed: %s", jobId, err.Error())
	}

	if _, err := r.DB.Exec(sql, args...); err != nil {
		log.Errorf("archiving job (dbid: %d) failed: %s", jobId, err.Error())
	}
}

func (r *JobRepository) findById(id string) (job *schema.Job, err error) {
	var sql string
	var args []interface{}
	sql, args, err = sq.Select(schema.JobColumns...).From("job").Where("job.id = ?", id).ToSql()
	if err != nil {
		return
	}
	job, err = schema.ScanJob(r.DB.QueryRowx(sql, args...))
	return
}

func (r *JobRepository) Exists(jobId int64) bool {
	rows, err := r.DB.Query(`SELECT job.id FROM job WHERE job.job_id = ?`, jobId)

	if err != nil {
		return false
	}

	if rows.Next() {
		return true
	}

	return false
}

func (r *JobRepository) AddTag(jobId int64, tagId int64) error {
	_, err := r.DB.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, jobId, tagId)
	return err
}

func (r *JobRepository) TagExists(tagType string, tagName string) (exists bool, tagId int64) {
	exists = true
	if err := sq.Select("id").From("tag").
		Where("tag.tag_type = ?", tagType).Where("tag.tag_name = ?", tagName).
		RunWith(r.DB).QueryRow().Scan(&tagId); err != nil {
		exists = false
		return exists, tagId
	} else {
		return exists, tagId
	}
}
