// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package repository provides data access and persistence layer for ClusterCockpit.
//
// This file implements tag management functionality for job categorization and classification.
// Tags support both manual assignment (via REST/GraphQL APIs) and automatic detection
// (via tagger plugins). The implementation includes role-based access control through
// tag scopes and maintains bidirectional consistency between the SQL database and
// the file-based job archive.
//
// Database Schema:
//
//	CREATE TABLE tag (
//	    id INTEGER PRIMARY KEY AUTOINCREMENT,
//	    tag_type VARCHAR(255) NOT NULL,
//	    tag_name VARCHAR(255) NOT NULL,
//	    tag_scope VARCHAR(255) NOT NULL DEFAULT "global",
//	    CONSTRAINT tag_unique UNIQUE (tag_type, tag_name, tag_scope)
//	);
//
//	CREATE TABLE jobtag (
//	    job_id INTEGER,
//	    tag_id INTEGER,
//	    PRIMARY KEY (job_id, tag_id),
//	    FOREIGN KEY (job_id) REFERENCES job(id) ON DELETE CASCADE,
//	    FOREIGN KEY (tag_id) REFERENCES tag(id) ON DELETE CASCADE
//	);
//
// The jobtag junction table enables many-to-many relationships between jobs and tags.
// CASCADE deletion ensures referential integrity when jobs or tags are removed.
package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
)

// Tag Scope Rules:
//
// Tags in ClusterCockpit have three visibility scopes that control who can see and use them:
//
//  1. "global" - Visible to all users, can be used by anyone
//     Example: System-generated tags like "energy-efficient", "failed", "short"
//
//  2. "private" - Only visible to the creating user
//     Example: Personal notes like "needs-review", "interesting-case"
//
//  3. "admin" - Only visible to users with admin or support roles
//     Example: Internal notes like "hardware-issue", "billing-problem"
//
// Authorization Rules:
//   - Regular users can only create/see "global" and their own "private" tags
//   - Admin/Support can create/see all scopes including "admin" tags
//   - Users can only add tags to jobs they have permission to view
//   - Tag scope is enforced at query time in GetTags() and CountTags()

// AddTag adds the tag with id `tagId` to the job with the database id `jobId`.
// Requires user authentication for security checks.
//
// The user must have permission to view the job. Tag visibility is determined by scope:
//   - "global" tags: visible to all users
//   - "private" tags: only visible to the tag creator
//   - "admin" tags: only visible to admin/support users
func (r *JobRepository) AddTag(user *schema.User, job int64, tag int64) ([]*schema.Tag, error) {
	j, err := r.FindByIDWithUser(user, job)
	if err != nil {
		cclog.Warnf("Error finding job %d for user %s: %v", job, user.Username, err)
		return nil, err
	}

	return r.addJobTag(job, tag, j, func() ([]*schema.Tag, error) {
		return r.GetTags(user, &job)
	})
}

// AddTagDirect adds a tag without user security checks.
// Use only for internal/admin operations.
func (r *JobRepository) AddTagDirect(job int64, tag int64) ([]*schema.Tag, error) {
	j, err := r.FindByIDDirect(job)
	if err != nil {
		cclog.Warnf("Error finding job %d: %v", job, err)
		return nil, err
	}

	return r.addJobTag(job, tag, j, func() ([]*schema.Tag, error) {
		return r.GetTagsDirect(&job)
	})
}

// RemoveTag removes the tag with the database id `tag` from the job with the database id `job`.
// Requires user authentication for security checks. Used by GraphQL API.
func (r *JobRepository) RemoveTag(user *schema.User, job, tag int64) ([]*schema.Tag, error) {
	j, err := r.FindByIDWithUser(user, job)
	if err != nil {
		cclog.Warnf("Error while finding job %d for user %s during tag removal: %v", job, user.Username, err)
		return nil, err
	}

	q := sq.Delete("jobtag").Where("jobtag.job_id = ?", job).Where("jobtag.tag_id = ?", tag)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error removing tag with %s: %v", s, err)
		return nil, err
	}

	tags, err := r.GetTags(user, &job)
	if err != nil {
		cclog.Warn("Error while getting tags for job")
		return nil, err
	}

	archiveTags, err := r.getArchiveTags(&job)
	if err != nil {
		cclog.Warnf("Error while getting archive tags for job %d in RemoveTag: %v", job, err)
		return nil, err
	}

	return tags, archive.UpdateTags(j, archiveTags)
}

// RemoveJobTagByRequest removes a tag from the job with the database id `job` by tag type, name, and scope.
// Requires user authentication for security checks. Used by REST API.
func (r *JobRepository) RemoveJobTagByRequest(user *schema.User, job int64, tagType string, tagName string, tagScope string) ([]*schema.Tag, error) {
	// Get Tag ID to delete
	tagID, exists := r.TagID(tagType, tagName, tagScope)
	if !exists {
		cclog.Warnf("Tag does not exist (name, type, scope): %s, %s, %s", tagName, tagType, tagScope)
		return nil, fmt.Errorf("tag does not exist (name, type, scope): %s, %s, %s", tagName, tagType, tagScope)
	}

	// Get Job
	j, err := r.FindByIDWithUser(user, job)
	if err != nil {
		cclog.Warnf("Error while finding job %d for user %s during tag removal by request: %v", job, user.Username, err)
		return nil, err
	}

	// Handle Delete
	q := sq.Delete("jobtag").Where("jobtag.job_id = ?", job).Where("jobtag.tag_id = ?", tagID)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error removing tag from table 'jobTag' with %s: %v", s, err)
		return nil, err
	}

	tags, err := r.GetTags(user, &job)
	if err != nil {
		cclog.Warnf("Error while getting tags for job %d in RemoveJobTagByRequest: %v", job, err)
		return nil, err
	}

	archiveTags, err := r.getArchiveTags(&job)
	if err != nil {
		cclog.Warnf("Error while getting archive tags for job %d in RemoveJobTagByRequest: %v", job, err)
		return nil, err
	}

	return tags, archive.UpdateTags(j, archiveTags)
}

// removeTagFromArchiveJobs updates the job archive for all affected jobs after a tag deletion.
//
// This function is called asynchronously (via goroutine) after removing a tag from the database
// to synchronize the file-based job archive with the database state. Errors are logged but not
// returned since this runs in the background.
//
// Parameters:
//   - jobIds: Database IDs of all jobs that had the deleted tag
//
// Implementation note: Each job is processed individually to handle partial failures gracefully.
// If one job fails to update, others will still be processed.
func (r *JobRepository) removeTagFromArchiveJobs(jobIds []int64) {
	for _, j := range jobIds {
		tags, err := r.getArchiveTags(&j)
		if err != nil {
			cclog.Warnf("Error while getting tags for job %d", j)
			continue
		}

		job, err := r.FindByIDDirect(j)
		if err != nil {
			cclog.Warnf("Error while getting job %d", j)
			continue
		}

		archive.UpdateTags(job, tags)
	}
}

// Removes a tag from db by tag info
// Used by REST API. Does not update tagged jobs in Job archive.
func (r *JobRepository) RemoveTagByRequest(tagType string, tagName string, tagScope string) error {
	// Get Tag ID to delete
	tagID, exists := r.TagID(tagType, tagName, tagScope)
	if !exists {
		cclog.Warnf("Tag does not exist (name, type, scope): %s, %s, %s", tagName, tagType, tagScope)
		return fmt.Errorf("tag does not exist (name, type, scope): %s, %s, %s", tagName, tagType, tagScope)
	}

	return r.RemoveTagByID(tagID)
}

// Removes a tag from db by tag id
// Used by GraphQL API.
func (r *JobRepository) RemoveTagByID(tagID int64) error {
	jobIds, err := r.FindJobIdsByTag(tagID)
	if err != nil {
		return err
	}

	// Handle Delete JobTagTable
	qJobTag := sq.Delete("jobtag").Where("jobtag.tag_id = ?", tagID)

	if _, err := qJobTag.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := qJobTag.ToSql()
		cclog.Errorf("Error removing tag from table 'jobTag' with %s: %v", s, err)
		return err
	}

	// Handle Delete TagTable
	qTag := sq.Delete("tag").Where("tag.id = ?", tagID)

	if _, err := qTag.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := qTag.ToSql()
		cclog.Errorf("Error removing tag from table 'tag' with %s: %v", s, err)
		return err
	}

	// asynchronously update archive jobs
	go r.removeTagFromArchiveJobs(jobIds)

	return nil
}

// CreateTag creates a new tag with the specified type, name, and scope.
// Returns the database ID of the newly created tag.
//
// Scope defaults to "global" if empty string is provided.
// Valid scopes: "global", "private", "admin"
//
// Example:
//
//	tagID, err := repo.CreateTag("performance", "high-memory", "global")
func (r *JobRepository) CreateTag(tagType string, tagName string, tagScope string) (tagID int64, err error) {
	// Default to "Global" scope if none defined
	if tagScope == "" {
		tagScope = "global"
	}

	q := sq.Insert("tag").Columns("tag_type", "tag_name", "tag_scope").Values(tagType, tagName, tagScope)

	res, err := q.RunWith(r.stmtCache).Exec()
	if err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error inserting tag with %s: %v", s, err)
		return 0, err
	}

	return res.LastInsertId()
}

// CountTags returns all tags visible to the user and the count of jobs for each tag.
// Applies scope-based filtering to respect tag visibility rules.
//
// Returns:
//   - tags: slice of tags the user can see
//   - counts: map of tag name to job count
//   - err: any error encountered
func (r *JobRepository) CountTags(user *schema.User) (tags []schema.Tag, counts map[string]int, err error) {
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
		readable, err := r.checkScopeAuth(user, "read", t.Scope)
		if err != nil {
			return nil, nil, err
		}
		if readable {
			tags = append(tags, t)
		}
	}

	// Query and Count Jobs with attached Tags
	q := sq.Select("t.tag_type, t.tag_name, t.id, count(jt.tag_id)").
		From("tag t").
		LeftJoin("jobtag jt ON t.id = jt.tag_id").
		GroupBy("t.tag_type, t.tag_name")

	// Build scope list for filtering
	var scopeBuilder strings.Builder
	scopeBuilder.WriteString(`"global"`)
	if user != nil {
		scopeBuilder.WriteString(`,"`)
		scopeBuilder.WriteString(user.Username)
		scopeBuilder.WriteString(`"`)
		if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}) {
			scopeBuilder.WriteString(`,"admin"`)
		}
	}
	q = q.Where("t.tag_scope IN (" + scopeBuilder.String() + ")")

	// Handle Job Ownership
	if user != nil && user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}) { // ADMIN || SUPPORT: Count all jobs
		// cclog.Debug("CountTags: User Admin or Support -> Count all Jobs for Tags")
		// Unchanged: Needs to be own case still, due to UserRole/NoRole compatibility handling in else case
	} else if user != nil && user.HasRole(schema.RoleManager) { // MANAGER: Count own jobs plus project's jobs
		// Build ("project1", "project2", ...) list of variable length directly in SQL string
		q = q.Where("jt.job_id IN (SELECT id FROM job WHERE job.hpc_user = ? OR job.project IN (\""+strings.Join(user.Projects, "\",\"")+"\"))", user.Username)
	} else if user != nil { // USER OR NO ROLE (Compatibility): Only count own jobs
		q = q.Where("jt.job_id IN (SELECT id FROM job WHERE job.hpc_user = ?)", user.Username)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		return nil, nil, err
	}

	counts = make(map[string]int)
	for rows.Next() {
		var tagType string
		var tagName string
		var tagID int
		var count int
		if err = rows.Scan(&tagType, &tagName, &tagID, &count); err != nil {
			return nil, nil, err
		}
		// Use tagId as second Map-Key component to differentiate tags with identical names
		counts[fmt.Sprint(tagType, tagName, tagID)] = count
	}
	err = rows.Err()

	return tags, counts, err
}

var (
	// ErrTagNotFound is returned when a tag ID or tag identifier (type, name, scope) does not exist in the database.
	ErrTagNotFound = errors.New("the tag does not exist")

	// ErrJobNotOwned is returned when a user attempts to tag a job they do not have permission to access.
	ErrJobNotOwned = errors.New("user is not owner of job")

	// ErrTagNoAccess is returned when a user attempts to use a tag they cannot access due to scope restrictions.
	ErrTagNoAccess = errors.New("user not permitted to use that tag")

	// ErrTagPrivateScope is returned when a user attempts to access another user's private tag.
	ErrTagPrivateScope = errors.New("tag is private to another user")

	// ErrTagAdminScope is returned when a non-admin user attempts to use an admin-scoped tag.
	ErrTagAdminScope = errors.New("tag requires admin privileges")

	// ErrTagsIncompatScopes is returned when attempting to combine admin and non-admin scoped tags in a single operation.
	ErrTagsIncompatScopes = errors.New("combining admin and non-admin scoped tags not allowed")
)

// addJobTag is a helper function that inserts a job-tag association and updates the archive.
//
// This function performs three operations atomically:
//  1. Inserts the job-tag association into the jobtag junction table
//  2. Retrieves the updated tag list for the job (using the provided getTags callback)
//  3. Updates the job archive with the new tags to maintain database-archive consistency
//
// Parameters:
//   - jobId: Database ID of the job
//   - tagId: Database ID of the tag to associate
//   - job: Full job object needed for archive update
//   - getTags: Callback function to retrieve updated tags (allows different security contexts)
//
// Returns the complete updated tag list for the job or an error.
//
// Note: This function does NOT validate tag scope permissions - callers must perform
// authorization checks before invoking this helper.
func (r *JobRepository) addJobTag(jobID int64, tagID int64, job *schema.Job, getTags func() ([]*schema.Tag, error)) ([]*schema.Tag, error) {
	q := sq.Insert("jobtag").Columns("job_id", "tag_id").Values(jobID, tagID)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error adding tag with %s: %v", s, err)
		return nil, err
	}

	tags, err := getTags()
	if err != nil {
		cclog.Warnf("Error getting tags for job %d: %v", jobID, err)
		return nil, err
	}

	archiveTags, err := r.getArchiveTags(&jobID)
	if err != nil {
		cclog.Warnf("Error getting archive tags for job %d: %v", jobID, err)
		return nil, err
	}

	return tags, archive.UpdateTags(job, archiveTags)
}

// AddTagOrCreate adds the tag with the specified type and name to the job with the database id `jobId`.
// If such a tag does not yet exist, it is created.
func (r *JobRepository) AddTagOrCreate(user *schema.User, jobID int64, tagType string, tagName string, tagScope string) (tagID int64, err error) {
	// Default to "Global" scope if none defined
	if tagScope == "" {
		tagScope = "global"
	}

	writable, err := r.checkScopeAuth(user, "write", tagScope)
	if err != nil {
		return 0, err
	}
	if !writable {
		return 0, fmt.Errorf("cannot write tag scope with current authorization")
	}

	tagID, exists := r.TagID(tagType, tagName, tagScope)
	if !exists {
		tagID, err = r.CreateTag(tagType, tagName, tagScope)
		if err != nil {
			return 0, err
		}
	}

	if _, err := r.AddTag(user, jobID, tagID); err != nil {
		return 0, err
	}

	return tagID, nil
}

func (r *JobRepository) AddTagOrCreateDirect(jobID int64, tagType string, tagName string) (tagID int64, err error) {
	tagScope := "global"

	tagID, exists := r.TagID(tagType, tagName, tagScope)
	if !exists {
		tagID, err = r.CreateTag(tagType, tagName, tagScope)
		if err != nil {
			return 0, err
		}
	}

	cclog.Infof("Adding tag %s:%s:%s (direct)", tagType, tagName, tagScope)

	if _, err := r.AddTagDirect(jobID, tagID); err != nil {
		return 0, err
	}

	return tagID, nil
}

func (r *JobRepository) HasTag(jobID int64, tagType string, tagName string) bool {
	var id int64
	q := sq.Select("id").From("tag").Join("jobtag ON jobtag.tag_id = tag.id").
		Where("jobtag.job_id = ?", jobID).Where("tag.tag_type = ?", tagType).
		Where("tag.tag_name = ?", tagName)
	err := q.RunWith(r.stmtCache).QueryRow().Scan(&id)
	if err != nil {
		return false
	} else {
		return true
	}
}

// TagID returns the database id of the tag with the specified type and name.
func (r *JobRepository) TagID(tagType string, tagName string, tagScope string) (tagID int64, exists bool) {
	exists = true
	if err := sq.Select("id").From("tag").
		Where("tag.tag_type = ?", tagType).Where("tag.tag_name = ?", tagName).Where("tag.tag_scope = ?", tagScope).
		RunWith(r.stmtCache).QueryRow().Scan(&tagID); err != nil {
		exists = false
	}
	return
}

// TagInfo returns the database infos of the tag with the specified id.
func (r *JobRepository) TagInfo(tagID int64) (tagType string, tagName string, tagScope string, exists bool) {
	exists = true
	if err := sq.Select("tag.tag_type", "tag.tag_name", "tag.tag_scope").From("tag").Where("tag.id = ?", tagID).
		RunWith(r.stmtCache).QueryRow().Scan(&tagType, &tagName, &tagScope); err != nil {
		exists = false
	}
	return
}

// GetTags returns a list of all scoped tags if job is nil or of the tags that the job with that database ID has.
func (r *JobRepository) GetTags(user *schema.User, job *int64) ([]*schema.Tag, error) {
	q := sq.Select("id", "tag_type", "tag_name", "tag_scope").From("tag")
	if job != nil {
		q = q.Join("jobtag ON jobtag.tag_id = tag.id").Where("jobtag.job_id = ?", *job)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error get tags with %s: %v", s, err)
		return nil, err
	}

	tags := make([]*schema.Tag, 0)
	for rows.Next() {
		tag := &schema.Tag{}
		if err := rows.Scan(&tag.ID, &tag.Type, &tag.Name, &tag.Scope); err != nil {
			cclog.Warnf("Error while scanning tag rows in GetTags: %v", err)
			return nil, err
		}
		// Handle Scope Filtering: Tag Scope is Global, Private (== Username) or User is auth'd to view Admin Tags
		readable, err := r.checkScopeAuth(user, "read", tag.Scope)
		if err != nil {
			return nil, err
		}
		if readable {
			tags = append(tags, tag)
		}
	}

	return tags, nil
}

func (r *JobRepository) GetTagsDirect(job *int64) ([]*schema.Tag, error) {
	q := sq.Select("id", "tag_type", "tag_name", "tag_scope").From("tag")
	if job != nil {
		q = q.Join("jobtag ON jobtag.tag_id = tag.id").Where("jobtag.job_id = ?", *job)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error get tags with %s: %v", s, err)
		return nil, err
	}

	tags := make([]*schema.Tag, 0)
	for rows.Next() {
		tag := &schema.Tag{}
		if err := rows.Scan(&tag.ID, &tag.Type, &tag.Name, &tag.Scope); err != nil {
			cclog.Warnf("Error while scanning tag rows in GetTagsDirect: %v", err)
			return nil, err
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// getArchiveTags returns all tags for a job WITHOUT applying scope-based filtering.
//
// This internal function is used exclusively for job archive synchronization where we need
// to store all tags regardless of the current user's permissions. Unlike GetTags() which
// filters by scope, this returns the complete unfiltered tag list.
//
// Parameters:
//   - job: Pointer to job database ID, or nil to return all tags in the system
//
// Returns all tags without scope filtering, used only for archive operations.
//
// WARNING: Do NOT expose this function to user-facing APIs as it bypasses authorization.
func (r *JobRepository) getArchiveTags(job *int64) ([]*schema.Tag, error) {
	q := sq.Select("id", "tag_type", "tag_name", "tag_scope").From("tag")
	if job != nil {
		q = q.Join("jobtag ON jobtag.tag_id = tag.id").Where("jobtag.job_id = ?", *job)
	}

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error get tags with %s: %v", s, err)
		return nil, err
	}

	tags := make([]*schema.Tag, 0)
	for rows.Next() {
		tag := &schema.Tag{}
		if err := rows.Scan(&tag.ID, &tag.Type, &tag.Name, &tag.Scope); err != nil {
			cclog.Warnf("Error while scanning tag rows in getArchiveTags: %v", err)
			return nil, err
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

func (r *JobRepository) ImportTag(jobID int64, tagType string, tagName string, tagScope string) (err error) {
	// Import has no scope ctx, only import from metafile to DB (No recursive archive update required), only returns err

	tagID, exists := r.TagID(tagType, tagName, tagScope)
	if !exists {
		tagID, err = r.CreateTag(tagType, tagName, tagScope)
		if err != nil {
			return err
		}
	}

	q := sq.Insert("jobtag").Columns("job_id", "tag_id").Values(jobID, tagID)

	if _, err := q.RunWith(r.stmtCache).Exec(); err != nil {
		s, _, _ := q.ToSql()
		cclog.Errorf("Error adding tag on import with %s: %v", s, err)
		return err
	}

	return nil
}

// checkScopeAuth validates whether a user is authorized to perform an operation on a tag with the given scope.
//
// This function implements the tag scope authorization matrix:
//
//	Scope        | Read Access                      | Write Access
//	-------------|----------------------------------|----------------------------------
//	"global"     | All users                        | Admin, Support, API-only
//	"admin"      | Admin, Support                   | Admin, API-only
//	<username>   | Owner only                       | Owner only (private tags)
//
// Parameters:
//   - user: User attempting the operation (must not be nil)
//   - operation: Either "read" or "write"
//   - scope: Tag scope value ("global", "admin", or username for private tags)
//
// Returns:
//   - pass: true if authorized, false if denied
//   - err: error only if operation is invalid or user is nil
//
// Special cases:
//   - API-only users (single role: RoleApi) can write to admin and global scopes for automation
//   - Private tags use the username as scope, granting exclusive access to that user
func (r *JobRepository) checkScopeAuth(user *schema.User, operation string, scope string) (pass bool, err error) {
	if user != nil {
		switch {
		case operation == "write" && scope == "admin":
			if user.HasRole(schema.RoleAdmin) || (len(user.Roles) == 1 && user.HasRole(schema.RoleAPI)) {
				return true, nil
			}
			return false, nil
		case operation == "write" && scope == "global":
			if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}) || (len(user.Roles) == 1 && user.HasRole(schema.RoleAPI)) {
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
