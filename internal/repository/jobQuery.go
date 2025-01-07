// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
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
	order *model.OrderByInput,
) ([]*schema.Job, error) {
	query, qerr := SecurityCheck(ctx, sq.Select(jobColumns...).From("job"))
	if qerr != nil {
		return nil, qerr
	}

	if order != nil {
		field := toSnakeCase(order.Field)
		if order.Type == "col" {
			// "col": Fixed column name query
			switch order.Order {
			case model.SortDirectionEnumAsc:
				query = query.OrderBy(fmt.Sprintf("job.%s ASC", field))
			case model.SortDirectionEnumDesc:
				query = query.OrderBy(fmt.Sprintf("job.%s DESC", field))
			default:
				return nil, errors.New("REPOSITORY/QUERY > invalid sorting order for column")
			}
		} else {
			// "foot": Order by footprint JSON field values
			// Verify and Search Only in Valid Jsons
			query = query.Where("JSON_VALID(meta_data)")
			switch order.Order {
			case model.SortDirectionEnumAsc:
				query = query.OrderBy(fmt.Sprintf("JSON_EXTRACT(footprint, \"$.%s\") ASC", field))
			case model.SortDirectionEnumDesc:
				query = query.OrderBy(fmt.Sprintf("JSON_EXTRACT(footprint, \"$.%s\") DESC", field))
			default:
				return nil, errors.New("REPOSITORY/QUERY > invalid sorting order for footprint")
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
	filters []*model.JobFilter,
) (int, error) {
	// DISTICT count for tags filters, does not affect other queries
	query, qerr := SecurityCheck(ctx, sq.Select("count(DISTINCT job.id)").From("job"))
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

func SecurityCheckWithUser(user *schema.User, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	if user == nil {
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user context is nil")
	}

	switch {
	case len(user.Roles) == 1 && user.HasRole(schema.RoleApi): // API-User : All jobs
		return query, nil
	case user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}): // Admin & Support : All jobs
		return query, nil
	case user.HasRole(schema.RoleManager): // Manager : Add filter for managed projects' jobs only + personal jobs
		if len(user.Projects) != 0 {
			return query.Where(sq.Or{sq.Eq{"job.project": user.Projects}, sq.Eq{"job.hpc_user": user.Username}}), nil
		} else {
			log.Debugf("Manager-User '%s' has no defined projects to lookup! Query only personal jobs ...", user.Username)
			return query.Where("job.hpc_user = ?", user.Username), nil
		}
	case user.HasRole(schema.RoleUser): // User : Only personal jobs
		return query.Where("job.hpc_user = ?", user.Username), nil
	default: // No known Role, return error
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user has no or unknown roles")
	}
}

func SecurityCheck(ctx context.Context, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	user := GetUserFromContext(ctx)

	return SecurityCheckWithUser(user, query)
}

// Build a sq.SelectBuilder out of a schema.JobFilter.
func BuildWhereClause(filter *model.JobFilter, query sq.SelectBuilder) sq.SelectBuilder {
	if filter.Tags != nil {
		// This is an OR-Logic query: Returns all distinct jobs with at least one of the requested tags; TODO: AND-Logic query?
		query = query.Join("jobtag ON jobtag.job_id = job.id").Where(sq.Eq{"jobtag.tag_id": filter.Tags}).Distinct()
	}
	if filter.JobID != nil {
		query = buildStringCondition("job.job_id", filter.JobID, query)
	}
	if filter.ArrayJobID != nil {
		query = query.Where("job.array_job_id = ?", *filter.ArrayJobID)
	}
	if filter.User != nil {
		query = buildStringCondition("job.hpc_user", filter.User, query)
	}
	if filter.Project != nil {
		query = buildStringCondition("job.project", filter.Project, query)
	}
	if filter.JobName != nil {
		query = buildMetaJsonCondition("jobName", filter.JobName, query)
	}
	if filter.Cluster != nil {
		query = buildStringCondition("job.cluster", filter.Cluster, query)
	}
	if filter.Partition != nil {
		query = buildStringCondition("job.cluster_partition", filter.Partition, query)
	}
	if filter.StartTime != nil {
		query = buildTimeCondition("job.start_time", filter.StartTime, query)
	}
	if filter.Duration != nil {
		query = buildIntCondition("job.duration", filter.Duration, query)
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
	if filter.Energy != nil {
		query = buildFloatCondition("job.energy", filter.Energy, query)
	}
	if filter.MetricStats != nil {
		for _, ms := range filter.MetricStats {
			query = buildFloatJsonCondition(ms.MetricName, ms.Range, query)
		}
	}
	return query
}

func buildIntCondition(field string, cond *schema.IntRange, query sq.SelectBuilder) sq.SelectBuilder {
	return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
}

func buildFloatCondition(field string, cond *model.FloatRange, query sq.SelectBuilder) sq.SelectBuilder {
	return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
}

func buildTimeCondition(field string, cond *schema.TimeRange, query sq.SelectBuilder) sq.SelectBuilder {
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
			log.Debugf("No known named timeRange: startTime.range = %s", cond.Range)
			return query
		}
		return query.Where(field+" BETWEEN ? AND ?", then, now)
	} else {
		return query
	}
}

func buildFloatJsonCondition(condName string, condRange *model.FloatRange, query sq.SelectBuilder) sq.SelectBuilder {
	// Verify and Search Only in Valid Jsons
	query = query.Where("JSON_VALID(footprint)")
	return query.Where("JSON_EXTRACT(footprint, \"$."+condName+"\") BETWEEN ? AND ?", condRange.From, condRange.To)
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
		copy(queryElements, cond.In)
		return query.Where(sq.Or{sq.Eq{field: queryElements}})
	}
	return query
}

func buildMetaJsonCondition(jsonField string, cond *model.StringInput, query sq.SelectBuilder) sq.SelectBuilder {
	// Verify and Search Only in Valid Jsons
	query = query.Where("JSON_VALID(meta_data)")
	// add "AND" Sql query Block for field match
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

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

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
