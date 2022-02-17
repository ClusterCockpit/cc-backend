package repository

import (
	"fmt"

	"github.com/ClusterCockpit/cc-backend/schema"
	sq "github.com/Masterminds/squirrel"
)

// Add the tag with id `tagId` to the job with the database id `jobId`.
func (r *JobRepository) AddTag(jobId int64, tagId int64) error {
	_, err := r.DB.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, jobId, tagId)
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
