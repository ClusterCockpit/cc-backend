package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/auth"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	DB *sqlx.DB
}

var JobTableCols []string = []string{
	"id", "job_id", "cluster", "start_time",
	"user", "project", "partition", "array_job_id", "duration", "job_state", "resources",
	"num_nodes", "num_hwthreads", "num_acc", "smt", "exclusive", "monitoring_status",
	"load_avg", "mem_used_max", "flops_any_avg", "mem_bw_avg", "net_bw_avg", "file_bw_avg",
}

type Scannable interface {
	Scan(dest ...interface{}) error
}

// Helper function for scanning jobs with the `jobTableCols` columns selected.
func ScanJob(row Scannable) (*model.Job, error) {
	job := &model.Job{}

	var rawResources []byte
	if err := row.Scan(
		&job.ID, &job.JobID, &job.Cluster, &job.StartTime,
		&job.User, &job.Project, &job.Partition, &job.ArrayJobID, &job.Duration, &job.State, &rawResources,
		&job.NumNodes, &job.NumHWThreads, &job.NumAcc, &job.Smt, &job.Exclusive, &job.MonitoringStatus,
		&job.LoadAvg, &job.MemUsedMax, &job.FlopsAnyAvg, &job.MemBwAvg, &job.NetBwAvg, &job.FileBwAvg); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(rawResources, &job.Resources); err != nil {
		return nil, err
	}

	if job.Duration == 0 && job.State == model.JobStateRunning {
		job.Duration = int(time.Since(job.StartTime).Seconds())
	}

	return job, nil
}

// Helper function for the `jobs` GraphQL-Query. Is also used elsewhere when a list of jobs is needed.
func (r *Resolver) queryJobs(ctx context.Context, filters []*model.JobFilter, page *model.PageRequest, order *model.OrderByInput) ([]*model.Job, int, error) {
	query := sq.Select(JobTableCols...).From("job")
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

	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	jobs := make([]*model.Job, 0, 50)
	for rows.Next() {
		job, err := ScanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, job)
	}

	query = sq.Select("count(*)").From("job")
	for _, f := range filters {
		query = buildWhereClause(f, query)
	}
	rows, err = query.RunWith(r.DB).Query()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var count int
	rows.Next()
	if err := rows.Scan(&count); err != nil {
		return nil, 0, err
	}

	return jobs, count, nil
}

func securityCheck(ctx context.Context, query sq.SelectBuilder) sq.SelectBuilder {
	val := ctx.Value(auth.ContextUserKey)
	if val == nil {
		return query
	}

	user := val.(*auth.User)
	if user.IsAdmin {
		return query
	}

	return query.Where("job.user_id = ?", user.Username)
}

// Build a sq.SelectBuilder out of a model.JobFilter.
func buildWhereClause(filter *model.JobFilter, query sq.SelectBuilder) sq.SelectBuilder {
	if filter.Tags != nil {
		query = query.Join("jobtag ON jobtag.job_id = job.id").Where("jobtag.tag_id IN ?", filter.Tags)
	}
	if filter.JobID != nil {
		query = buildStringCondition("job.job_id", filter.JobID, query)
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
	if filter.StartTime != nil {
		query = buildTimeCondition("job.start_time", filter.StartTime, query)
	}
	if filter.Duration != nil {
		query = buildIntCondition("job.duration", filter.Duration, query)
	}
	if filter.JobState != nil {
		query = query.Where("job.job_state IN ?", filter.JobState)
	}
	if filter.NumNodes != nil {
		query = buildIntCondition("job.num_nodes", filter.NumNodes, query)
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
		return query.Where(field+"LIKE ?", fmt.Sprint(*cond.StartsWith, "%"))
	}
	if cond.EndsWith != nil {
		return query.Where(field+"LIKE ?", fmt.Sprint("%", *cond.StartsWith))
	}
	if cond.Contains != nil {
		return query.Where(field+"LIKE ?", fmt.Sprint("%", *cond.StartsWith, "%"))
	}
	return query
}

func toSnakeCase(str string) string {
	matchFirstCap := regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
