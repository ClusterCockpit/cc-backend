package graph

//go:generate go run github.com/99designs/gqlgen
import (
	"context"
	"fmt"
	"log"
	"strings"
	"os"
	"strconv"
	"encoding/json"

	"github.com/ClusterCockpit/cc-jobarchive/graph/generated"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/jmoiron/sqlx"
)

type Resolver struct {
	DB *sqlx.DB
}

func NewRootResolvers(db *sqlx.DB) generated.Config {
	c := generated.Config{
		Resolvers: &Resolver{
			DB: db,
		},
	}

	return c
}

// Helper functions
func addStringCondition(conditions []string, field string, input *model.StringInput) []string {
	if input.Eq != nil {
		conditions = append(conditions, fmt.Sprintf("%s='%s'", field, *input.Eq))
	}
	if input.StartsWith != nil {
		conditions = append(conditions, fmt.Sprintf("%s LIKE '%s%%'", field, *input.StartsWith))
	}
	if input.Contains != nil {
		conditions = append(conditions, fmt.Sprintf("%s LIKE '%%%s%%'", field, *input.Contains))
	}
	if input.EndsWith != nil {
		conditions = append(conditions, fmt.Sprintf("%s LIKE '%%%s'", field, *input.EndsWith))
	}

	return conditions
}

func addIntCondition(conditions []string, field string, input *model.IntRange) []string {
	conditions = append(conditions, fmt.Sprintf("%s BETWEEN %d AND %d", field, input.From, input.To))
	return conditions
}

func addTimeCondition(conditions []string, field string, input *model.TimeRange) []string {
	conditions = append(conditions, fmt.Sprintf("%s BETWEEN %d AND %d", field, input.From.Unix(), input.To.Unix()))
	return conditions
}

func buildQueryConditions(filterList *model.JobFilterList) (string, string) {
	var conditions []string
	var join string

	joinJobtags := `
	JOIN jobtag ON jobtag.job_id = job.id
	JOIN tag ON tag.id = jobtag.tag_id
	`

	for _, condition := range filterList.List {
		if condition.TagName != nil {
			conditions = append(conditions, fmt.Sprintf("tag.tag_name = '%s'", *condition.TagName))
			join = joinJobtags
		}
		if condition.TagType != nil {
			conditions = append(conditions, fmt.Sprintf("tag.tag_type = '%s'", *condition.TagType))
			join = joinJobtags
		}
		if condition.JobID != nil {
			conditions = addStringCondition(conditions, `job.job_id`, condition.JobID)
		}
		if condition.UserID != nil {
			conditions = addStringCondition(conditions, `user_id`, condition.UserID)
		}
		if condition.ProjectID != nil {
			conditions = addStringCondition(conditions, `project_id`, condition.ProjectID)
		}
		if condition.ClusterID != nil {
			conditions = addStringCondition(conditions, `cluster_id`, condition.ClusterID)
		}
		if condition.StartTime != nil {
			conditions = addTimeCondition(conditions, `start_time`, condition.StartTime)
		}
		if condition.Duration != nil {
			conditions = addIntCondition(conditions, `duration`, condition.Duration)
		}
		if condition.NumNodes != nil {
			conditions = addIntCondition(conditions, `num_nodes`, condition.NumNodes)
		}
	}

	return strings.Join(conditions, " AND "), join
}

func readJobDataFile(jobId string) ([]byte, error) {
	// TODO: Use suffix as cluster-id!
	jobId = strings.Split(jobId, ".")[0]
	id, err := strconv.Atoi(jobId)
	if err != nil {
		return nil, err
	}

	lvl1, lvl2 := id / 1000, id % 1000
	filepath := fmt.Sprintf("./job-data/%d/%03d/data.json", lvl1, lvl2)
	f, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func contains(s []*string, e string) bool {
	for _, a := range s {
		if a != nil && *a == e {
			return true
		}
	}
	return false
}

// Queries

func (r *queryResolver) JobByID(
	ctx context.Context,
	jobID string) (*model.Job, error) {
	var job model.Job
	qstr := `SELECT * from job `
	qstr += fmt.Sprintf("WHERE id=%s", jobID)

	row := r.DB.QueryRowx(qstr)
	err := row.StructScan(&job)
	if err != nil {
		return nil, err
	}

	return &job, nil
}

func (r *queryResolver) Jobs(
	ctx context.Context,
	filterList *model.JobFilterList,
	page *model.PageRequest,
	orderBy *model.OrderByInput) (*model.JobResultList, error) {

	var jobs []*model.Job
	var limit, offset int
	var qc, ob, jo string

	if page != nil {
		limit = *page.ItemsPerPage
		offset = (*page.Page - 1) * limit
	} else {
		limit = 20
		offset = 0
	}

	if filterList != nil {
		qc, jo = buildQueryConditions(filterList)

		if qc != "" {
			qc = `WHERE ` + qc
		}

		if jo != "" {
			qc = jo + qc
		}
	}

	if orderBy != nil {
		ob = fmt.Sprintf("ORDER BY %s %s", orderBy.Field, *orderBy.Order)
	}

	qstr := `SELECT job.* `
	qstr += fmt.Sprintf("FROM job %s %s LIMIT %d OFFSET %d", qc, ob, limit, offset)
	log.Printf("%s", qstr)

	rows, err := r.DB.Queryx(qstr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var job model.Job
		err := rows.StructScan(&job)
		if err != nil {
			fmt.Println(err)
		}
		jobs = append(jobs, &job)
	}

	var count int
	qstr = fmt.Sprintf("SELECT COUNT(*) FROM job %s", qc)
	row := r.DB.QueryRow(qstr)
	err = row.Scan(&count)
	if err != nil {
		return nil, err
	}

	returnValue := model.JobResultList{
		jobs,
		&offset, &limit,
		&count}

	return &returnValue, nil
}

func (r *queryResolver) JobsStatistics(
	ctx context.Context,
	filterList *model.JobFilterList) (*model.JobsStatistics, error) {
	var qc, jo string

	if filterList != nil {
		qc, jo = buildQueryConditions(filterList)

		if qc != "" {
			qc = `WHERE ` + qc
		}

		if jo != "" {
			qc = jo + qc
		}
	}

	// TODO Change current node hours to core hours
	qstr := `SELECT COUNT(*), SUM(duration)/3600, SUM(duration*num_nodes)/3600  `
	qstr += fmt.Sprintf("FROM job %s ", qc)
	log.Printf("%s", qstr)

	var stats model.JobsStatistics
	row := r.DB.QueryRow(qstr)
	err := row.Scan(&stats.TotalJobs, &stats.TotalWalltime, &stats.TotalCoreHours)
	if err != nil {
		return nil, err
	}

	qstr = `SELECT COUNT(*) `
	qstr += fmt.Sprintf("FROM job %s AND duration < 120", qc)
	log.Printf("%s", qstr)
	row = r.DB.QueryRow(qstr)
	err = row.Scan(&stats.ShortJobs)
	if err != nil {
		return nil, err
	}

	var histogram []*model.HistoPoint
	// Node histogram
	qstr = `SELECT num_nodes, COUNT(*) `
	qstr += fmt.Sprintf("FROM job %s GROUP BY 1", qc)
	log.Printf("%s", qstr)

	rows, err := r.DB.Query(qstr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var point model.HistoPoint
		rows.Scan(&point.Count, &point.Value)
		histogram = append(histogram, &point)
	}
	stats.HistNumNodes = histogram

	// Node histogram
	qstr = `SELECT duration/3600, COUNT(*) `
	qstr += fmt.Sprintf("FROM job %s GROUP BY 1", qc)
	log.Printf("%s", qstr)

	rows, err = r.DB.Query(qstr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	histogram = nil

	for rows.Next() {
		var point model.HistoPoint
		rows.Scan(&point.Count, &point.Value)
		histogram = append(histogram, &point)
	}
	stats.HistWalltime = histogram

	return &stats, nil
}

func (r *queryResolver) JobMetrics(
	ctx context.Context, jobId string,
	metrics []*string) ([]*model.JobMetricWithName, error) {

	f, err := readJobDataFile(jobId)
	if err != nil {
		return nil, err
	}

	/*
	 * GraphQL has no Map-Type, so
	 * this is the best i could come up with.
	 * This is only for testing anyways?
	 */
	var list []*model.JobMetricWithName
	var metricMap map[string]*model.JobMetric

	err = json.Unmarshal(f, &metricMap)
	if err != nil {
		return nil, err
	}

	for name, metric := range metricMap {
		if metrics == nil || contains(metrics, name) {
			list = append(list, &model.JobMetricWithName{ name, metric })
		}
	}

	return list, nil
}

func (r *queryResolver) Tags(
	ctx context.Context, jobId *string) ([]*model.JobTag, error) {

	if jobId == nil {
		rows, err := r.DB.Queryx("SELECT * FROM tag")
		if err != nil {
			return nil, err
		}

		tags := []*model.JobTag{}
		for rows.Next() {
			var tag model.JobTag
			err = rows.StructScan(&tag)
			if err != nil {
				return nil, err
			}
			tags = append(tags, &tag)
		}
		return tags, nil
	}

	/* TODO: Use cluster id? */
	query := `
	SELECT tag.id, tag.tag_name, tag.tag_type FROM tag
	JOIN jobtag ON tag.id = jobtag.tag_id
	JOIN job ON job.id = jobtag.job_id
	WHERE job.job_id = $1
	`
	rows, err := r.DB.Queryx(query, jobId)
	if err != nil {
		return nil, err
	}

	tags := []*model.JobTag{}
	for rows.Next() {
		var tag model.JobTag
		err = rows.StructScan(&tag)
		if err != nil {
			return nil, err
		}
		tags = append(tags, &tag)
	}

	return tags, nil
}

func (r *jobResolver) Tags(ctx context.Context, job *model.Job) ([]*model.JobTag, error) {
	query := `
	SELECT tag.id, tag.tag_name, tag.tag_type FROM tag
	JOIN jobtag ON tag.id = jobtag.tag_id
	WHERE jobtag.job_id = $1
	`
	rows, err := r.DB.Queryx(query, job.ID)
	if err != nil {
		return nil, err
	}

	tags := []*model.JobTag{}
	for rows.Next() {
		var tag model.JobTag
		err = rows.StructScan(&tag)
		if err != nil {
			return nil, err
		}
		tags = append(tags, &tag)
	}

	return tags, nil
}

func (r *Resolver) Job() generated.JobResolver { return &jobResolver{r} }
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type jobResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
