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

func buildQueryConditions(filterList *model.JobFilterList) string {
	var conditions []string

	for _, condition := range filterList.List {
		if condition.JobID != nil {
			conditions = addStringCondition(conditions, `job_id`, condition.JobID)
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

	return strings.Join(conditions, " AND ")
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
	var qc, ob string

	if page != nil {
		limit = *page.ItemsPerPage
		offset = (*page.Page - 1) * limit
	} else {
		limit = 20
		offset = 0
	}

	if filterList != nil {
		qc = buildQueryConditions(filterList)

		if qc != "" {
			qc = `WHERE ` + qc
		}
	}

	if orderBy != nil {
		ob = fmt.Sprintf("ORDER BY %s %s", orderBy.Field, *orderBy.Order)
	}

	qstr := `SELECT * `
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
	var qc string

	if filterList != nil {
		qc = buildQueryConditions(filterList)

		if qc != "" {
			qc = `WHERE ` + qc
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

func (r *queryResolver) JobDataByID(
	ctx context.Context, jobId string) (*model.JobData, error) {

	// TODO: What to do with the suffix?
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

	jobData := new(model.JobData)
	err = json.Unmarshal(f, jobData)
	if err != nil {
		return nil, err
	}

	return jobData, nil
}

func (r *queryResolver) JobAvailableMetricsByID(
	ctx context.Context, jobId string) ([]*model.JobMetricWithName, error) {

	jobData, err := r.JobDataByID(ctx, jobId)
	if err != nil {
		return nil, err
	}

	var list []*model.JobMetricWithName

	/*
	 * GraphQL has no Map-Type, so
	 * this is the best i could come up with.
	 * This is only for testing anyways?
	 */

	if jobData.LoadOne != nil {
		list = append(list, &model.JobMetricWithName {
			"load_one", jobData.LoadOne })
	}
	if jobData.MemUsed != nil {
		list = append(list, &model.JobMetricWithName {
			"mem_used", jobData.MemUsed })
	}
	if jobData.MemBw != nil {
		list = append(list, &model.JobMetricWithName {
			"mem_bw", jobData.MemBw })
	}
	if jobData.FlopsAny != nil {
		list = append(list, &model.JobMetricWithName {
			"flops_any", jobData.FlopsAny })
	}
	if jobData.FlopsDp != nil {
		list = append(list, &model.JobMetricWithName {
			"flops_dp", jobData.FlopsDp })
	}
	if jobData.FlopsSp != nil {
		list = append(list, &model.JobMetricWithName {
			"flops_sp", jobData.FlopsSp })
	}
	if jobData.CpiAvg != nil {
		list = append(list, &model.JobMetricWithName {
			"cpi_avg", jobData.CpiAvg })
	}
	if jobData.ClockSpeed != nil {
		list = append(list, &model.JobMetricWithName {
			"clock_speed", jobData.ClockSpeed })
	}
	if jobData.TotalPower != nil {
		list = append(list, &model.JobMetricWithName {
			"total_power", jobData.TotalPower })
	}
	if jobData.TrafficReadEth != nil {
		list = append(list, &model.JobMetricWithName {
			"traffic_read_eth", jobData.TrafficReadEth })
	}
	if jobData.TrafficWriteEth != nil {
		list = append(list, &model.JobMetricWithName {
			"traffic_write_eth", jobData.TrafficWriteEth })
	}
	if jobData.TrafficReadLustre != nil {
		list = append(list, &model.JobMetricWithName {
			"traffic_read_lustre", jobData.TrafficReadLustre })
	}
	if jobData.TrafficWriteLustre != nil {
		list = append(list, &model.JobMetricWithName {
			"traffic_write_lustre", jobData.TrafficWriteLustre })
	}
	if jobData.RegReadLustre != nil {
		list = append(list, &model.JobMetricWithName {
			"reg_read_lustre", jobData.RegReadLustre })
	}
	if jobData.RegWriteLustre != nil {
		list = append(list, &model.JobMetricWithName {
			"reg_write_lustre", jobData.RegWriteLustre })
	}
	if jobData.InodesLustre != nil {
		list = append(list, &model.JobMetricWithName {
			"inodes_lustre", jobData.InodesLustre })
	}
	if jobData.PkgRateReadIb != nil {
		list = append(list, &model.JobMetricWithName {
			"pkg_rate_read_ib", jobData.PkgRateReadIb })
	}
	if jobData.PkgRateWriteIb != nil {
		list = append(list, &model.JobMetricWithName {
			"pkg_rate_write_ib", jobData.PkgRateWriteIb })
	}
	if jobData.CongestionIb != nil {
		list = append(list, &model.JobMetricWithName {
			"congestion_ib", jobData.CongestionIb })
	}

	return list, nil
}

func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
