package graph

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/graph/model"
	"github.com/ClusterCockpit/cc-backend/log"
	"github.com/ClusterCockpit/cc-backend/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	DB *sqlx.DB

	findJobByIdStmt         *sqlx.Stmt
	findJobByIdWithUserStmt *sqlx.Stmt
}

func (r *Resolver) Init() error {
	findJobById, _, err := sq.Select(schema.JobColumns...).From("job").Where("job.id = ?", nil).ToSql()
	if err != nil {
		return err
	}

	r.findJobByIdStmt, err = r.DB.Preparex(findJobById)
	if err != nil {
		return err
	}

	findJobByIdWithUser, _, err := sq.Select(schema.JobColumns...).From("job").Where("job.id = ?", nil).Where("job.user = ?").ToSql()
	if err != nil {
		return err
	}

	r.findJobByIdWithUserStmt, err = r.DB.Preparex(findJobByIdWithUser)
	if err != nil {
		return err
	}

	return nil
}

// Helper function for the `jobs` GraphQL-Query. Is also used elsewhere when a list of jobs is needed.
func (r *Resolver) queryJobs(ctx context.Context, filters []*model.JobFilter, page *model.PageRequest, order *model.OrderByInput) ([]*schema.Job, int, error) {
	query := sq.Select(schema.JobColumns...).From("job")
	query = securityCheck(ctx, query)

	if order != nil {
		field := toSnakeCase(order.Field)
		if order.Order == model.SortDirectionEnumAsc {
			query = query.OrderBy(fmt.Sprintf("job.%s ASC", field))
		} else if order.Order == model.SortDirectionEnumDesc {
			query = query.OrderBy(fmt.Sprintf("job.%s DESC", field))
		} else {
			return nil, 0, errors.New("invalid sorting order")
		}
	}

	if page != nil {
		limit := uint64(page.ItemsPerPage)
		query = query.Offset((uint64(page.Page) - 1) * limit).Limit(limit)
	} else {
		query = query.Limit(50)
	}

	for _, f := range filters {
		query = buildWhereClause(f, query)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, 0, err
	}

	log.Debugf("SQL query: `%s`, args: %#v", sql, args)
	rows, err := r.DB.Queryx(sql, args...)
	if err != nil {
		return nil, 0, err
	}

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := schema.ScanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, job)
	}

	// count all jobs:
	query = sq.Select("count(*)").From("job")
	for _, f := range filters {
		query = buildWhereClause(f, query)
	}
	var count int
	if err := query.RunWith(r.DB).Scan(&count); err != nil {
		return nil, 0, err
	}

	return jobs, count, nil
}

func securityCheck(ctx context.Context, query sq.SelectBuilder) sq.SelectBuilder {
	user := auth.GetUser(ctx)
	if user == nil || user.HasRole(auth.RoleAdmin) {
		return query
	}

	return query.Where("job.user = ?", user.Username)
}

// Build a sq.SelectBuilder out of a schema.JobFilter.
func buildWhereClause(filter *model.JobFilter, query sq.SelectBuilder) sq.SelectBuilder {
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
		query = buildIntCondition("job.duration", filter.Duration, query)
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
	return query
}

func buildIntCondition(field string, cond *model.IntRange, query sq.SelectBuilder) sq.SelectBuilder {
	return query.Where(field+" BETWEEN ? AND ?", cond.From, cond.To)
}

func buildTimeCondition(field string, cond *model.TimeRange, query sq.SelectBuilder) sq.SelectBuilder {
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
	if cond.StartsWith != nil {
		return query.Where(field+" LIKE ?", fmt.Sprint(*cond.StartsWith, "%"))
	}
	if cond.EndsWith != nil {
		return query.Where(field+" LIKE ?", fmt.Sprint("%", *cond.EndsWith))
	}
	if cond.Contains != nil {
		return query.Where(field+" LIKE ?", fmt.Sprint("%", *cond.Contains, "%"))
	}
	return query
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	str = strings.ReplaceAll(str, "'", "")
	str = strings.ReplaceAll(str, "\\", "")
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
