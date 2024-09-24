// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

// Add the tag with id `tagId` to the job with the database id `jobId`.
func (r *JobRepository) AddTag(ctx context.Context, job int64, tag int64) ([]*schema.Tag, error) {

	j, err := r.FindById(ctx, job)
	if err != nil {
		log.Warn("Error while finding job by id")
		return nil, err
	}

	q := sq.Insert("jobtag").Columns("job_id", "tag_id").Values(job, tag)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		log.Errorf("Error adding tag with %s: %v", s, err)
		return nil, err
	}

	tags, err := r.GetTags(ctx, &job)
	if err != nil {
		log.Warn("Error while getting tags for job")
		return nil, err
	}

	archiveTags, err := r.getArchiveTags(&job)
	if err != nil {
		log.Warn("Error while getting tags for job")
		return nil, err
	}

	return tags, archive.UpdateTags(j, archiveTags)
}

// Removes a tag from a job
func (r *JobRepository) RemoveTag(ctx context.Context, job, tag int64) ([]*schema.Tag, error) {

	j, err := r.FindById(ctx, job)
	if err != nil {
		log.Warn("Error while finding job by id")
		return nil, err
	}

	q := sq.Delete("jobtag").Where("jobtag.job_id = ?", job).Where("jobtag.tag_id = ?", tag)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		log.Errorf("Error removing tag with %s: %v", s, err)
		return nil, err
	}

	tags, err := r.GetTags(ctx, &job)
	if err != nil {
		log.Warn("Error while getting tags for job")
		return nil, err
	}

	archiveTags, err := r.getArchiveTags(&job)
	if err != nil {
		log.Warn("Error while getting tags for job")
		return nil, err
	}

	return tags, archive.UpdateTags(j, archiveTags)
}

// CreateTag creates a new tag with the specified type and name and returns its database id.
func (r *JobRepository) CreateTag(tagType string, tagName string, tagScope string) (tagId int64, err error) {

	// Default to "Global" scope if none defined
	if tagScope == "" {
		tagScope = "global"
	}

	q := sq.Insert("tag").Columns("tag_type", "tag_name", "tag_scope").Values(tagType, tagName, tagScope)

	res, err := q.RunWith(r.stmtCache).Exec()
	if err != nil {
		s, _, _ := q.ToSql()
		log.Errorf("Error inserting tag with %s: %v", s, err)
		return 0, err
	}

	return res.LastInsertId()
}

func (r *JobRepository) CountTags(ctx context.Context) (tags []schema.Tag, counts map[string]int, err error) {
	// Fetch all Tags in DB for Display in Frontend Tag-View
	tags = make([]schema.Tag, 0, 100)
	xrows, err := r.DB.Queryx("SELECT id, tag_type, tag_name, tag_scope FROM tag")
	if err != nil {
		return nil, nil, err
	}

	for xrows.Next() {
		var t schema.Tag
		if err = xrows.StructScan(&t); err != nil {
			return nil, nil, err
		}

		// Handle Scope Filtering: Tag Scope is Global, Private (== Username) or User is auth'd to view Admin Tags
		readable, err := r.checkScopeAuth(ctx, "read", t.Scope)
		if err != nil {
			return nil, nil, err
		}
		if readable {
			tags = append(tags, t)
		}
	}

	user := GetUserFromContext(ctx)

	// Query and Count Jobs with attached Tags
	q := sq.Select("t.tag_name, t.id, count(jt.tag_id)").
		From("tag t").
		LeftJoin("jobtag jt ON t.id = jt.tag_id").
		GroupBy("t.tag_name")

	// Handle Scope Filtering
	scopeList := "\"global\""
	if user != nil {
		scopeList += ",\"" + user.Username + "\""
	}
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}) {
		scopeList += ",\"admin\""
	}
	q = q.Where("t.tag_scope IN (" + scopeList + ")")

	// Handle Job Ownership
	if user != nil && user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}) { // ADMIN || SUPPORT: Count all jobs
		// log.Debug("CountTags: User Admin or Support -> Count all Jobs for Tags")
		// Unchanged: Needs to be own case still, due to UserRole/NoRole compatibility handling in else case
	} else if user != nil && user.HasRole(schema.RoleManager) { // MANAGER: Count own jobs plus project's jobs
		// Build ("project1", "project2", ...) list of variable length directly in SQL string
		q = q.Where("jt.job_id IN (SELECT id FROM job WHERE job.user = ? OR job.project IN (\""+strings.Join(user.Projects, "\",\"")+"\"))", user.Username)
	} else if user != nil { // USER OR NO ROLE (Compatibility): Only count own jobs
		q = q.Where("jt.job_id IN (SELECT id FROM job WHERE job.user = ?)", user.Username)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		return nil, nil, err
	}

	counts = make(map[string]int)
	for rows.Next() {
		var tagName string
		var tagId int
		var count int
		if err = rows.Scan(&tagName, &tagId, &count); err != nil {
			return nil, nil, err
		}
		// Use tagId as second Map-Key component to differentiate tags with identical names
		counts[fmt.Sprint(tagName, tagId)] = count
	}
	err = rows.Err()

	return tags, counts, err
}

// AddTagOrCreate adds the tag with the specified type and name to the job with the database id `jobId`.
// If such a tag does not yet exist, it is created.
func (r *JobRepository) AddTagOrCreate(ctx context.Context, jobId int64, tagType string, tagName string, tagScope string) (tagId int64, err error) {

	// Default to "Global" scope if none defined
	if tagScope == "" {
		tagScope = "global"
	}

	writable, err := r.checkScopeAuth(ctx, "write", tagScope)
	if err != nil {
		return 0, err
	}
	if !writable {
		return 0, fmt.Errorf("cannot write tag scope with current authorization")
	}

	tagId, exists := r.TagId(tagType, tagName, tagScope)
	if !exists {
		tagId, err = r.CreateTag(tagType, tagName, tagScope)
		if err != nil {
			return 0, err
		}
	}

	if _, err := r.AddTag(ctx, jobId, tagId); err != nil {
		return 0, err
	}

	return tagId, nil
}

// TagId returns the database id of the tag with the specified type and name.
func (r *JobRepository) TagId(tagType string, tagName string, tagScope string) (tagId int64, exists bool) {
	exists = true
	if err := sq.Select("id").From("tag").
		Where("tag.tag_type = ?", tagType).Where("tag.tag_name = ?", tagName).Where("tag.tag_scope = ?", tagScope).
		RunWith(r.stmtCache).QueryRow().Scan(&tagId); err != nil {
		exists = false
	}
	return
}

// GetTags returns a list of all scoped tags if job is nil or of the tags that the job with that database ID has.
func (r *JobRepository) GetTags(ctx context.Context, job *int64) ([]*schema.Tag, error) {
	q := sq.Select("id", "tag_type", "tag_name", "tag_scope").From("tag")
	if job != nil {
		q = q.Join("jobtag ON jobtag.tag_id = tag.id").Where("jobtag.job_id = ?", *job)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		s, _, _ := q.ToSql()
		log.Errorf("Error get tags with %s: %v", s, err)
		return nil, err
	}

	tags := make([]*schema.Tag, 0)
	for rows.Next() {
		tag := &schema.Tag{}
		if err := rows.Scan(&tag.ID, &tag.Type, &tag.Name, &tag.Scope); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}
		// Handle Scope Filtering: Tag Scope is Global, Private (== Username) or User is auth'd to view Admin Tags
		readable, err := r.checkScopeAuth(ctx, "read", tag.Scope)
		if err != nil {
			return nil, err
		}
		if readable {
			tags = append(tags, tag)
		}
	}

	return tags, nil
}

// GetArchiveTags returns a list of all tags *regardless of scope* for archiving if job is nil or of the tags that the job with that database ID has.
func (r *JobRepository) getArchiveTags(job *int64) ([]*schema.Tag, error) {
	q := sq.Select("id", "tag_type", "tag_name", "tag_scope").From("tag")
	if job != nil {
		q = q.Join("jobtag ON jobtag.tag_id = tag.id").Where("jobtag.job_id = ?", *job)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		s, _, _ := q.ToSql()
		log.Errorf("Error get tags with %s: %v", s, err)
		return nil, err
	}

	tags := make([]*schema.Tag, 0)
	for rows.Next() {
		tag := &schema.Tag{}
		if err := rows.Scan(&tag.ID, &tag.Type, &tag.Name, &tag.Scope); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

func (r *JobRepository) ImportTag(jobId int64, tagType string, tagName string, tagScope string) (err error) {
	// Import has no scope ctx, only import from metafile to DB (No recursive archive update required), only returns err

	tagId, exists := r.TagId(tagType, tagName, tagScope)
	if !exists {
		tagId, err = r.CreateTag(tagType, tagName, tagScope)
		if err != nil {
			return err
		}
	}

	q := sq.Insert("jobtag").Columns("job_id", "tag_id").Values(jobId, tagId)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		log.Errorf("Error adding tag on import with %s: %v", s, err)
		return err
	}

	return nil
}

func (r *JobRepository) checkScopeAuth(ctx context.Context, operation string, scope string) (pass bool, err error) {
	user := GetUserFromContext(ctx)
	if user != nil {
		switch {
		case operation == "write" && scope == "admin":
			if user.HasRole(schema.RoleAdmin) || (len(user.Roles) == 1 && user.HasRole(schema.RoleApi)) {
				return true, nil
			}
			return false, nil
		case operation == "write" && scope == "global":
			if user.HasRole(schema.RoleAdmin) || (len(user.Roles) == 1 && user.HasRole(schema.RoleApi)) {
				return true, nil
			}
			return false, nil
		case operation == "write" && scope == user.Username:
			return true, nil
		case operation == "read" && scope == "admin":
			return user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}), nil
		case operation == "read" && scope == "global":
			return true, nil
		case operation == "read" && scope == user.Username:
			return true, nil
		default:
			if operation == "read" || operation == "write" {
				// No acceptable scope: deny tag
				return false, nil
			} else {
				return false, fmt.Errorf("error while checking tag operation auth: unknown operation (%s)", operation)
			}
		}
	} else {
		return false, fmt.Errorf("error while checking tag operation auth: no user in context")
	}
}
