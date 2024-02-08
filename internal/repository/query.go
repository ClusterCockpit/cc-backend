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

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

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

		switch order.Order {
		case model.SortDirectionEnumAsc:
			query = query.OrderBy(fmt.Sprintf("job.%s ASC", field))
		case model.SortDirectionEnumDesc:
			query = query.OrderBy(fmt.Sprintf("job.%s DESC", field))
		default:
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

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Errorf("Error while running query: %v", err)
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

func (r *JobRepository) CountJobs(
	ctx context.Context,
	filters []*model.JobFilter) (int, error) {

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

func SecurityCheck(ctx context.Context, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	user := GetUserFromContext(ctx)
	if user == nil {
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user context is nil")
	} else if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleApi}) { // Admin & Co. : All jobs
		return query, nil
	} else if user.HasRole(schema.RoleManager) { // Manager : Add filter for managed projects' jobs only + personal jobs
		if len(user.Projects) != 0 {
			return query.Where(sq.Or{sq.Eq{"job.project": user.Projects}, sq.Eq{"job.user": user.Username}}), nil
		} else {
			log.Debugf("Manager-User '%s' has no defined projects to lookup! Query only personal jobs ...", user.Username)
			return query.Where("job.user = ?", user.Username), nil
		}
	} else if user.HasRole(schema.RoleUser) { // User : Only personal jobs
		return query.Where("job.user = ?", user.Username), nil
	} else {
		// Shortterm compatibility: Return User-Query if no roles:
		return query.Where("job.user = ?", user.Username), nil
		// // On the longterm: Return Error instead of fallback:
		// var qnil sq.SelectBuilder
		// return qnil, fmt.Errorf("user '%s' with unknown roles [%#v]", user.Username, user.Roles)
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
	if filter.Node != nil {
		query = buildStringCondition("job.resources", filter.Node, query)
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
		queryElements := make([]string, len(cond.In))
		for i, val := range cond.In {
			queryElements[i] = val
		}
		return query.Where(sq.Or{sq.Eq{field: queryElements}})
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
