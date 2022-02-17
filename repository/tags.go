package repository

import (
	"fmt"

	"github.com/ClusterCockpit/cc-backend/schema"
	sq "github.com/Masterminds/squirrel"
)

// Add the tag with id `tagId` to the job with the database id `jobId`.
func (r *JobRepository) AddTag(jobId int64, tagId int64) error {
	_, err := r.DB.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES ($1, $2)`, jobId, tagId)
	return err
}

// Removes a tag from a job
func (r *JobRepository) RemoveTag(job, tag int64) error {
	_, err := r.DB.Exec("DELETE FROM jobtag WHERE jobtag.job_id = $1 AND jobtag.tag_id = $2", job, tag)
	return err
}

// CreateTag creates a new tag with the specified type and name and returns its database id.
func (r *JobRepository) CreateTag(tagType string, tagName string) (tagId int64, err error) {
	res, err := r.DB.Exec("INSERT INTO tag (tag_type, tag_name) VALUES ($1, $2)", tagType, tagName)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (r *JobRepository) CountTags(user *string) (tags []schema.Tag, counts map[string]int, err error) {
	tags = make([]schema.Tag, 0, 100)
	xrows, err := r.DB.Queryx("SELECT * FROM tag")
	if err != nil {
		return nil, nil, err
	}

	for xrows.Next() {
		var t schema.Tag
		if err := xrows.StructScan(&t); err != nil {
			return nil, nil, err
		}
		tags = append(tags, t)
	}

	q := sq.Select("t.tag_name, count(jt.tag_id)").
		From("tag t").
		LeftJoin("jobtag jt ON t.id = jt.tag_id").
		GroupBy("t.tag_name")
	if user != nil {
		q = q.Where("jt.job_id IN (SELECT id FROM job WHERE job.user = ?)", *user)
	}

	rows, err := q.RunWith(r.DB).Query()
	if err != nil {
		return nil, nil, err
	}

	counts = make(map[string]int)

	for rows.Next() {
		var tagName string
		var count int
		err = rows.Scan(&tagName, &count)
		if err != nil {
			fmt.Println(err)
		}
		counts[tagName] = count
	}
	err = rows.Err()

	return
}

// AddTagOrCreate adds the tag with the specified type and name to the job with the database id `jobId`.
// If such a tag does not yet exist, it is created.
func (r *JobRepository) AddTagOrCreate(jobId int64, tagType string, tagName string) (tagId int64, err error) {
	tagId, exists := r.TagId(tagType, tagName)
	if !exists {
		tagId, err = r.CreateTag(tagType, tagName)
		if err != nil {
			return 0, err
		}
	}

	return tagId, r.AddTag(jobId, tagId)
}

// TagId returns the database id of the tag with the specified type and name.
func (r *JobRepository) TagId(tagType string, tagName string) (tagId int64, exists bool) {
	exists = true
	if err := sq.Select("id").From("tag").
		Where("tag.tag_type = ?", tagType).Where("tag.tag_name = ?", tagName).
		RunWith(r.DB).QueryRow().Scan(&tagId); err != nil {
		exists = false
	}
	return
}

// GetTags returns a list of all tags if job is nil or of the tags that the job with that database ID has.
func (r *JobRepository) GetTags(job *int64) ([]*schema.Tag, error) {
	q := sq.Select("id", "tag_type", "tag_name").From("tag")
	if job != nil {
		q = q.Join("jobtag ON jobtag.tag_id = tag.id").Where("jobtag.job_id = ?", *job)
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}

	tags := make([]*schema.Tag, 0)
	if err := r.DB.Select(&tags, sql, args...); err != nil {
		return nil, err
	}

	return tags, nil
}
