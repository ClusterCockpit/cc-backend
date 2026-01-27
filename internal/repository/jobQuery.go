// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package repository provides job query functionality with filtering, pagination,
// and security controls. This file contains the main query builders and security
// checks for job retrieval operations.
package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
)

const (
	// Default initial capacity for job result slices
	defaultJobsCapacity = 50
)

// QueryJobs retrieves jobs from the database with optional filtering, pagination,
// and sorting. Security controls are automatically applied based on the user context.
//
// Parameters:
//   - ctx: Context containing user authentication information
//   - filters: Optional job filters (cluster, state, user, time ranges, etc.)
//   - page: Optional pagination parameters (page number and items per page)
//   - order: Optional sorting specification (column or footprint field)
//
// Returns a slice of jobs matching the criteria, or an error if the query fails.
// The function enforces role-based access control through SecurityCheck.
func (r *JobRepository) QueryJobs(
	ctx context.Context,
	filters []*model.JobFilter,
	page *model.PageRequest,
	order *model.OrderByInput,
) ([]*schema.Job, error) {
	query, qerr := SecurityCheck(ctx, sq.Select(jobColumns...).From("job"))
	if qerr != nil {
		return nil, qerr
	}

	if order != nil {
		field := toSnakeCase(order.Field)
		if order.Type == "col" {
			switch order.Order {
			case model.SortDirectionEnumAsc:
				query = query.OrderBy(fmt.Sprintf("job.%s ASC", field))
			case model.SortDirectionEnumDesc:
				query = query.OrderBy(fmt.Sprintf("job.%s DESC", field))
			default:
				return nil, errors.New("invalid sorting order for column")
			}
		} else {
			// Order by footprint JSON field values
			query = query.Where("JSON_VALID(meta_data)")
			switch order.Order {
			case model.SortDirectionEnumAsc:
				query = query.OrderBy(fmt.Sprintf("JSON_EXTRACT(footprint, \"$.%s\") ASC", field))
			case model.SortDirectionEnumDesc:
				query = query.OrderBy(fmt.Sprintf("JSON_EXTRACT(footprint, \"$.%s\") DESC", field))
			default:
				return nil, errors.New("invalid sorting order for footprint")
			}
		}
	}

	if page != nil && page.ItemsPerPage != -1 {
		limit := uint64(page.ItemsPerPage)
		query = query.Offset((uint64(page.Page) - 1) * limit).Limit(limit)
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		return nil, fmt.Errorf("query failed [%s] %v: %w", queryString, queryVars, err)
	}
	defer rows.Close()

	jobs := make([]*schema.Job, 0, defaultJobsCapacity)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			cclog.Warnf("Error scanning job row: %v", err)
			return nil, fmt.Errorf("failed to scan job row: %w", err)
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating job rows: %w", err)
	}

	return jobs, nil
}

// CountJobs returns the total number of jobs matching the given filters.
// Security controls are automatically applied based on the user context.
// Uses DISTINCT count to handle tag filters correctly (jobs may appear multiple
// times when joined with the tag table).
func (r *JobRepository) CountJobs(
	ctx context.Context,
	filters []*model.JobFilter,
) (int, error) {
	query, qerr := SecurityCheck(ctx, sq.Select("count(DISTINCT job.id)").From("job"))
	if qerr != nil {
		return 0, qerr
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	var count int
	if err := query.RunWith(r.DB).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count jobs: %w", err)
	}

	return count, nil
}

// SecurityCheckWithUser applies role-based access control filters to a job query
// based on the provided user's roles and permissions.
//
// Access rules by role:
//   - API role (exclusive): Full access to all jobs
//   - Admin/Support roles: Full access to all jobs
//   - Manager role: Access to jobs in managed projects plus own jobs
//   - User role: Access only to own jobs
//
// Returns an error if the user is nil or has no recognized roles.
func SecurityCheckWithUser(user *schema.User, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	if user == nil {
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user context is nil")
	}

	switch {
	case len(user.Roles) == 1 && user.HasRole(schema.RoleApi):
		return query, nil
	case user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}):
		return query, nil
	case user.HasRole(schema.RoleManager):
		if len(user.Projects) != 0 {
			return query.Where(sq.Or{sq.Eq{"job.project": user.Projects}, sq.Eq{"job.hpc_user": user.Username}}), nil
		}
		cclog.Debugf("Manager '%s' has no assigned projects, restricting to personal jobs", user.Username)
		return query.Where("job.hpc_user = ?", user.Username), nil
	case user.HasRole(schema.RoleUser):
		return query.Where("job.hpc_user = ?", user.Username), nil
	default:
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user has no or unknown roles")
	}
}

// SecurityCheck extracts the user from the context and applies role-based access
// control filters to the query. This is a convenience wrapper around SecurityCheckWithUser.
func SecurityCheck(ctx context.Context, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	user := GetUserFromContext(ctx)
	return SecurityCheckWithUser(user, query)
}

// BuildWhereClause constructs SQL WHERE conditions from a JobFilter and applies
// them to the query. Supports filtering by job properties (cluster, state, user),
// time ranges, resource usage, tags, and JSON field searches in meta_data,
// footprint, and resources columns.
func BuildWhereClause(filter *model.JobFilter, query sq.SelectBuilder) sq.SelectBuilder {
	// Primary Key
	if filter.DbID != nil {
		dbIDs := make([]string, len(filter.DbID))
		copy(dbIDs, filter.DbID)
		query = query.Where(sq.Eq{"job.id": dbIDs})
	}
	// Explicit indices
	if filter.Cluster != nil {
		query = buildStringCondition("job.cluster", filter.Cluster, query)
	}
	if filter.Partition != nil {
		query = buildStringCondition("job.cluster_partition", filter.Partition, query)
	}
	if filter.State != nil {
		states := make([]string, len(filter.State))
		for i, val := range filter.State {
			states[i] = string(val)
		}
		query = query.Where(sq.Eq{"job.job_state": states})
	}
	if filter.Shared != nil {
		query = query.Where("job.shared = ?", *filter.Shared)
	}
	if filter.Project != nil {
		query = buildStringCondition("job.project", filter.Project, query)
	}
	if filter.User != nil {
		query = buildStringCondition("job.hpc_user", filter.User, query)
	}
	if filter.NumNodes != nil {
		query = buildIntCondition("job.num_nodes", filter.NumNodes, query)
	}
	if filter.NumAccelerators != nil {
		query = buildIntCondition("job.num_acc", filter.NumAccelerators, query)
	}
	if filter.NumHWThreads != nil {
		query = buildIntCondition("job.num_hwthreads", filter.NumHWThreads, query)
	}
	if filter.ArrayJobID != nil {
		query = query.Where("job.array_job_id = ?", *filter.ArrayJobID)
	}
	if filter.StartTime != nil {
		query = buildTimeCondition("job.start_time", filter.StartTime, query)
	}
	if filter.Duration != nil {
		query = buildIntCondition("job.duration", filter.Duration, query)
	}
	if filter.Energy != nil {
		query = buildFloatCondition("job.energy", filter.Energy, query)
	}
	// Indices on Tag Table
	if filter.Tags != nil {
		// This is an OR-Logic query: Returns all distinct jobs with at least one of the requested tags; TODO: AND-Logic query?
		query = query.Join("jobtag ON jobtag.job_id = job.id").Where(sq.Eq{"jobtag.tag_id": filter.Tags}).Distinct()
	}
	// No explicit Indices
	if filter.JobID != nil {
		query = buildStringCondition("job.job_id", filter.JobID, query)
	}
	// Queries Within JSONs
	if filter.MetricStats != nil {
		for _, ms := range filter.MetricStats {
			query = buildFloatJSONCondition(ms.MetricName, ms.Range, query)
		}
	}
	if filter.Node != nil {
		query = buildResourceJSONCondition("hostname", filter.Node, query)
	}
	if filter.JobName != nil {
		query = buildMetaJSONCondition("jobName", filter.JobName, query)
	}
	if filter.Schedule != nil {
		interactiveJobname := "interactive"
		switch *filter.Schedule {
		case "interactive":
			iFilter := model.StringInput{Eq: &interactiveJobname}
			query = buildMetaJSONCondition("jobName", &iFilter, query)
		case "batch":
			sFilter := model.StringInput{Neq: &interactiveJobname}
			query = buildMetaJSONCondition("jobName", &sFilter, query)
		}
	}

	// Configurable Filter to exclude recently started jobs, see config.go: ShortRunningJobsDuration
	if filter.MinRunningFor != nil {
		now := time.Now().Unix()
		// Only jobs whose start timestamp is more than MinRunningFor seconds in the past
		// If a job completed within the configured timeframe, it will still show up after the start_time matches the condition!
		query = query.Where(sq.Lt{"job.start_time": (now - int64(*filter.MinRunningFor))})
	}
	return query
}

// buildIntCondition creates a BETWEEN clause for integer range filters.
// Reminder: BETWEEN Queries are slower and dont use indices as frequently: Only use if both conditions required
func buildIntCondition(field string, cond *config.IntRange, query sq.SelectBuilder) sq.SelectBuilder {
	if cond.From != 0 && cond.To != 0 {
		return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
	} else if cond.From != 0 {
		return query.Where("? <= "+field, cond.From)
	} else if cond.To != 0 {
		return query.Where(field+" <= ?", cond.To)
	} else {
		return query
	}
}

// buildFloatCondition creates a BETWEEN clause for float range filters.
// Reminder: BETWEEN Queries are slower and dont use indices as frequently: Only use if both conditions required
func buildFloatCondition(field string, cond *model.FloatRange, query sq.SelectBuilder) sq.SelectBuilder {
	if cond.From != 0.0 && cond.To != 0.0 {
		return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
	} else if cond.From != 0.0 {
		return query.Where("? <= "+field, cond.From)
	} else if cond.To != 0.0 {
		return query.Where(field+" <= ?", cond.To)
	} else {
		return query
	}
}

// buildTimeCondition creates time range filters supporting absolute timestamps,
// relative time ranges (last6h, last24h, last7d, last30d), or open-ended ranges.
// Reminder: BETWEEN Queries are slower and dont use indices as frequently: Only use if both conditions required
func buildTimeCondition(field string, cond *config.TimeRange, query sq.SelectBuilder) sq.SelectBuilder {
	if cond.From != nil && cond.To != nil {
		return query.Where(field+" BETWEEN ? AND ?", cond.From.Unix(), cond.To.Unix())
	} else if cond.From != nil {
		return query.Where("? <= "+field, cond.From.Unix())
	} else if cond.To != nil {
		return query.Where(field+" <= ?", cond.To.Unix())
	} else if cond.Range != "" {
		now := time.Now().Unix()
		var then int64
		switch cond.Range {
		case "last6h":
			then = now - (60 * 60 * 6)
		case "last24h":
			then = now - (60 * 60 * 24)
		case "last7d":
			then = now - (60 * 60 * 24 * 7)
		case "last30d":
			then = now - (60 * 60 * 24 * 30)
		default:
			cclog.Debugf("No known named timeRange: startTime.range = %s", cond.Range)
			return query
		}
		return query.Where("? <= "+field, then)
	} else {
		return query
	}
}

// buildFloatJSONCondition creates a filter on a numeric field within the footprint JSON column.
// Reminder: BETWEEN Queries are slower and dont use indices as frequently: Only use if both conditions required
func buildFloatJSONCondition(condName string, condRange *model.FloatRange, query sq.SelectBuilder) sq.SelectBuilder {
	query = query.Where("JSON_VALID(footprint)")
	if condRange.From != 0.0 && condRange.To != 0.0 {
		return query.Where("JSON_EXTRACT(footprint, \"$."+condName+"\") BETWEEN ? AND ?", condRange.From, condRange.To)
	} else if condRange.From != 0.0 {
		return query.Where("? <= JSON_EXTRACT(footprint, \"$."+condName+"\")", condRange.From)
	} else if condRange.To != 0.0 {
		return query.Where("JSON_EXTRACT(footprint, \"$."+condName+"\") <= ?", condRange.To)
	} else {
		return query
	}
}

// buildStringCondition creates filters for string fields supporting equality,
// inequality, prefix, suffix, substring, and IN list matching.
func buildStringCondition(field string, cond *model.StringInput, query sq.SelectBuilder) sq.SelectBuilder {
	if cond.Eq != nil {
		return query.Where(field+" = ?", *cond.Eq)
	}
	if cond.Neq != nil {
		return query.Where(field+" != ?", *cond.Neq)
	}
	if cond.StartsWith != nil {
		return query.Where(field+" LIKE ?", fmt.Sprint(*cond.StartsWith, "%"))
	}
	if cond.EndsWith != nil {
		return query.Where(field+" LIKE ?", fmt.Sprint("%", *cond.EndsWith))
	}
	if cond.Contains != nil {
		return query.Where(field+" LIKE ?", fmt.Sprint("%", *cond.Contains, "%"))
	}
	if cond.In != nil {
		queryElements := make([]string, len(cond.In))
		copy(queryElements, cond.In)
		return query.Where(sq.Or{sq.Eq{field: queryElements}})
	}
	return query
}

// buildMetaJSONCondition creates filters on fields within the meta_data JSON column.
func buildMetaJSONCondition(jsonField string, cond *model.StringInput, query sq.SelectBuilder) sq.SelectBuilder {
	query = query.Where("JSON_VALID(meta_data)")
	if cond.Eq != nil {
		return query.Where("JSON_EXTRACT(meta_data, \"$."+jsonField+"\") = ?", *cond.Eq)
	}
	if cond.Neq != nil {
		return query.Where("JSON_EXTRACT(meta_data, \"$."+jsonField+"\") != ?", *cond.Neq)
	}
	if cond.StartsWith != nil {
		return query.Where("JSON_EXTRACT(meta_data, \"$."+jsonField+"\") LIKE ?", fmt.Sprint(*cond.StartsWith, "%"))
	}
	if cond.EndsWith != nil {
		return query.Where("JSON_EXTRACT(meta_data, \"$."+jsonField+"\") LIKE ?", fmt.Sprint("%", *cond.EndsWith))
	}
	if cond.Contains != nil {
		return query.Where("JSON_EXTRACT(meta_data, \"$."+jsonField+"\") LIKE ?", fmt.Sprint("%", *cond.Contains, "%"))
	}
	return query
}

// buildResourceJSONCondition creates filters on fields within the resources JSON array column.
// Uses json_each to search within array elements.
func buildResourceJSONCondition(jsonField string, cond *model.StringInput, query sq.SelectBuilder) sq.SelectBuilder {
	query = query.Where("JSON_VALID(resources)")
	if cond.Eq != nil {
		return query.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, \"$."+jsonField+"\") = ?)", *cond.Eq)
	}
	if cond.Neq != nil { // Currently Unused
		return query.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, \"$."+jsonField+"\") != ?)", *cond.Neq)
	}
	if cond.StartsWith != nil { // Currently Unused
		return query.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, \"$."+jsonField+"\")) LIKE ?)", fmt.Sprint(*cond.StartsWith, "%"))
	}
	if cond.EndsWith != nil { // Currently Unused
		return query.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, \"$."+jsonField+"\") LIKE ?)", fmt.Sprint("%", *cond.EndsWith))
	}
	if cond.Contains != nil {
		return query.Where("EXISTS (SELECT 1 FROM json_each(job.resources) WHERE json_extract(value, \"$."+jsonField+"\") LIKE ?)", fmt.Sprint("%", *cond.Contains, "%"))
	}
	return query
}

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

// toSnakeCase converts camelCase strings to snake_case for SQL column names.
// Includes security checks to prevent SQL injection attempts.
// Panics if potentially dangerous characters are detected.
func toSnakeCase(str string) string {
	for _, c := range str {
		if c == '\'' || c == '\\' || c == '"' || c == ';' || c == '-' || c == ' ' {
			cclog.Panicf("toSnakeCase: potentially dangerous character detected in input: %q", str)
		}
	}

	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
