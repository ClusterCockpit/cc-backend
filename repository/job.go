package repository

import (
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

type JobRepository struct {
	DB *sqlx.DB
}

func (r *JobRepository) JobExists(jobId int64, cluster string, startTime int64) (rows *sql.Rows, err error) {
	rows, err = r.DB.Query(`SELECT job.id FROM job WHERE job.job_id = ? AND job.cluster = ? AND job.start_time = ?`,
		jobId, cluster, startTime)
	return
}

func (r *JobRepository) IdExists(jobId int64) bool {

	return true
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
