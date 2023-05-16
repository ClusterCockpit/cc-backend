// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

// QueryJobs returns a list of jobs matching the provided filters. page and order are optional-
func (r *JobRepository) QueryJobs(
	ctx context.Context,
	filters []*model.JobFilter,
	page *model.PageRequest,
	order *model.OrderByInput) ([]*schema.Job, error) {

	query, qerr := SecurityCheck(ctx, sq.Select(jobColumns...).From("job"))

	if qerr != nil {
		return nil, qerr
	}

	if order != nil {
		field := toSnakeCase(order.Field)
		if order.Order == model.SortDirectionEnumAsc {
			query = query.OrderBy(fmt.Sprintf("job.%s ASC", field))
		} else if order.Order == model.SortDirectionEnumDesc {
			query = query.OrderBy(fmt.Sprintf("job.%s DESC", field))
		} else {
			return nil, errors.New("REPOSITORY/QUERY > invalid sorting order")
		}
	}

	if page != nil && page.ItemsPerPage != -1 {
		limit := uint64(page.ItemsPerPage)
		query = query.Offset((uint64(page.Page) - 1) * limit).Limit(limit)
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		log.Warn("Error while converting query to sql")
		return nil, err
	}

	log.Debugf("SQL query: `%s`, args: %#v", sql, args)
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
			log.Warn("Error while scanning rows (Jobs)")
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// QueryJobLinks returns a list of minimal job information (DB-ID and jobId) of shared jobs for link-building based the provided filters.
func (r *JobRepository) QueryJobLinks(
	ctx context.Context,
	filters []*model.JobFilter) ([]*model.JobLink, error) {

	query, qerr := SecurityCheck(ctx, sq.Select("job.id", "job.job_id").From("job"))

	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		log.Warn("Error while converting query to sql")
		return nil, err
	}

	log.Debugf("SQL query: `%s`, args: %#v", sql, args)
	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	jobLinks := make([]*model.JobLink, 0, 50)
	for rows.Next() {
		jobLink, err := scanJobLink(rows)
		if err != nil {
			rows.Close()
			log.Warn("Error while scanning rows (JobLinks)")
			return nil, err
		}
		jobLinks = append(jobLinks, jobLink)
	}

	return jobLinks, nil
}

// CountJobs counts the number of jobs matching the filters.
func (r *JobRepository) CountJobs(
	ctx context.Context,
	filters []*model.JobFilter) (int, error) {

	// count all jobs:
	query, qerr := SecurityCheck(ctx, sq.Select("count(*)").From("job"))

	if qerr != nil {
		return 0, qerr
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}
	var count int
	if err := query.RunWith(r.DB).Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func SecurityCheck(ctx context.Context, query sq.SelectBuilder) (queryOut sq.SelectBuilder, err error) {
	user := auth.GetUser(ctx)
	if user == nil || user.HasAnyRole([]auth.Role{auth.RoleAdmin, auth.RoleSupport, auth.RoleApi}) { // Admin & Co. : All jobs
		return query, nil
	} else if user.HasRole(auth.RoleManager) { // Manager : Add filter for managed projects' jobs only + personal jobs
		if len(user.Projects) != 0 {
			return query.Where(sq.Or{sq.Eq{"job.project": user.Projects}, sq.Eq{"job.user": user.Username}}), nil
		} else {
			log.Infof("Manager-User '%s' has no defined projects to lookup! Query only personal jobs ...", user.Username)
			return query.Where("job.user = ?", user.Username), nil
		}
	} else if user.HasRole(auth.RoleUser) { // User : Only personal jobs
		return query.Where("job.user = ?", user.Username), nil
	} else { // Unauthorized : Error
		var qnil sq.SelectBuilder
		return qnil, errors.New(fmt.Sprintf("User '%s' with unknown roles! [%#v]\n", user.Username, user.Roles))
	}
}

// Build a sq.SelectBuilder out of a schema.JobFilter.
func BuildWhereClause(filter *model.JobFilter, query sq.SelectBuilder) sq.SelectBuilder {
	if filter.Tags != nil {
		query = query.Join("jobtag ON jobtag.job_id = job.id").Where(sq.Eq{"jobtag.tag_id": filter.Tags})
	}
	if filter.JobID != nil {
		query = buildStringCondition("job.job_id", filter.JobID, query)
	}
	if filter.ArrayJobID != nil {
		query = query.Where("job.array_job_id = ?", *filter.ArrayJobID)
	}
	if filter.User != nil {
		query = buildStringCondition("job.user", filter.User, query)
	}
	if filter.Project != nil {
		query = buildStringCondition("job.project", filter.Project, query)
	}
	if filter.JobName != nil {
		query = buildStringCondition("job.meta_data", filter.JobName, query)
	}
	if filter.Cluster != nil {
		query = buildStringCondition("job.cluster", filter.Cluster, query)
	}
	if filter.Partition != nil {
		query = buildStringCondition("job.partition", filter.Partition, query)
	}
	if filter.StartTime != nil {
		query = buildTimeCondition("job.start_time", filter.StartTime, query)
	}
	if filter.Duration != nil {
		now := time.Now().Unix() // There does not seam to be a portable way to get the current unix timestamp accross different DBs.
		query = query.Where("(CASE WHEN job.job_state = 'running' THEN (? - job.start_time) ELSE job.duration END) BETWEEN ? AND ?", now, filter.Duration.From, filter.Duration.To)
	}
	if filter.MinRunningFor != nil {
		now := time.Now().Unix() // There does not seam to be a portable way to get the current unix timestamp accross different DBs.
		query = query.Where("(job.job_state != 'running' OR (? - job.start_time) > ?)", now, *filter.MinRunningFor)
	}
	if filter.State != nil {
		states := make([]string, len(filter.State))
		for i, val := range filter.State {
			states[i] = string(val)
		}

		query = query.Where(sq.Eq{"job.job_state": states})
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
	if filter.FlopsAnyAvg != nil {
		query = buildFloatCondition("job.flops_any_avg", filter.FlopsAnyAvg, query)
	}
	if filter.MemBwAvg != nil {
		query = buildFloatCondition("job.mem_bw_avg", filter.MemBwAvg, query)
	}
	if filter.LoadAvg != nil {
		query = buildFloatCondition("job.load_avg", filter.LoadAvg, query)
	}
	if filter.MemUsedMax != nil {
		query = buildFloatCondition("job.mem_used_max", filter.MemUsedMax, query)
	}
	// Shared Jobs Query
	if filter.Exclusive != nil {
		query = query.Where("job.exclusive = ?", *filter.Exclusive)
	}
	if filter.SharedNode != nil {
		query = buildStringCondition("job.resources", filter.SharedNode, query)
	}
	if filter.SelfJobID != nil {
		query = buildStringCondition("job.job_id", filter.SelfJobID, query)
	}
	if filter.SelfStartTime != nil && filter.SelfDuration != nil {
		start := filter.SelfStartTime.Unix() + 10 // There does not seem to be a portable way to get the current unix timestamp accross different DBs.
		end := start + int64(*filter.SelfDuration) - 20
		query = query.Where("((job.start_time BETWEEN ? AND ?) OR ((job.start_time + job.duration) BETWEEN ? AND ?))", start, end, start, end)
	}
	return query
}

func buildIntCondition(field string, cond *schema.IntRange, query sq.SelectBuilder) sq.SelectBuilder {
	return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
}

func buildTimeCondition(field string, cond *schema.TimeRange, query sq.SelectBuilder) sq.SelectBuilder {
	if cond.From != nil && cond.To != nil {
		return query.Where(field+" BETWEEN ? AND ?", cond.From.Unix(), cond.To.Unix())
	} else if cond.From != nil {
		return query.Where("? <= "+field, cond.From.Unix())
	} else if cond.To != nil {
		return query.Where(field+" <= ?", cond.To.Unix())
	} else {
		return query
	}
}

func buildFloatCondition(field string, cond *model.FloatRange, query sq.SelectBuilder) sq.SelectBuilder {
	return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
}

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
		queryUsers := make([]string, len(cond.In))
		for i, val := range cond.In {
			queryUsers[i] = val
		}
		return query.Where(sq.Or{sq.Eq{"job.user": queryUsers}})
	}
	return query
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	for _, c := range str {
		if c == '\'' || c == '\\' {
			log.Panic("toSnakeCase() attack vector!")
		}
	}

	str = strings.ReplaceAll(str, "'", "")
	str = strings.ReplaceAll(str, "\\", "")
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
