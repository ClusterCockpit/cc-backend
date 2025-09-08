// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricDataDispatcher"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	sq "github.com/Masterminds/squirrel"
)

// GraphQL validation should make sure that no unkown values can be specified.
var groupBy2column = map[model.Aggregate]string{
	model.AggregateUser:    "job.hpc_user",
	model.AggregateProject: "job.project",
	model.AggregateCluster: "job.hpc_cluster",
}

var sortBy2column = map[model.SortByAggregate]string{
	model.SortByAggregateTotaljobs:      "totalJobs",
	model.SortByAggregateTotalwalltime:  "totalWalltime",
	model.SortByAggregateTotalnodes:     "totalNodes",
	model.SortByAggregateTotalnodehours: "totalNodeHours",
	model.SortByAggregateTotalcores:     "totalCores",
	model.SortByAggregateTotalcorehours: "totalCoreHours",
	model.SortByAggregateTotalaccs:      "totalAccs",
	model.SortByAggregateTotalacchours:  "totalAccHours",
}

func (r *JobRepository) buildCountQuery(
	filter []*model.JobFilter,
	kind string,
	col string,
) sq.SelectBuilder {
	var query sq.SelectBuilder

	if col != "" {
		// Scan columns: id, cnt
		query = sq.Select(col, "COUNT(job.id)").From("job").GroupBy(col)
	} else {
		// Scan columns:  cnt
		query = sq.Select("COUNT(job.id)").From("job")
	}

	switch kind {
	case "running":
		query = query.Where("job.job_state = ?", "running")
	case "short":
		query = query.Where("job.duration < ?", config.Keys.ShortRunningJobsDuration)
	}

	for _, f := range filter {
		query = BuildWhereClause(f, query)
	}

	return query
}

func (r *JobRepository) buildStatsQuery(
	filter []*model.JobFilter,
	col string,
) sq.SelectBuilder {
	var query sq.SelectBuilder
	castType := r.getCastType()

	// fmt.Sprintf(`CAST(ROUND((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) / 3600) as %s) as value`, time.Now().Unix(), castType)

	if col != "" {
		// Scan columns: id, totalJobs, name, totalWalltime, totalNodes, totalNodeHours, totalCores, totalCoreHours, totalAccs, totalAccHours
		query = sq.Select(col, "COUNT(job.id) as totalJobs", "name",
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END)) / 3600) as %s) as totalWalltime`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_nodes) as %s) as totalNodes`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_nodes) / 3600) as %s) as totalNodeHours`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_hwthreads) as %s) as totalCores`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_hwthreads) / 3600) as %s) as totalCoreHours`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_acc) as %s) as totalAccs`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_acc) / 3600) as %s) as totalAccHours`, time.Now().Unix(), castType),
		).From("job").LeftJoin("hpc_user ON hpc_user.username = job.hpc_user").GroupBy(col)
	} else {
		// Scan columns: totalJobs, name, totalWalltime, totalNodes, totalNodeHours, totalCores, totalCoreHours, totalAccs, totalAccHours
		query = sq.Select("COUNT(job.id)",
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END)) / 3600) as %s)`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_nodes) as %s)`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_nodes) / 3600) as %s)`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_hwthreads) as %s)`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_hwthreads) / 3600) as %s)`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_acc) as %s)`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_acc) / 3600) as %s)`, time.Now().Unix(), castType),
		).From("job")
	}

	for _, f := range filter {
		query = BuildWhereClause(f, query)
	}

	return query
}

// func (r *JobRepository) getUserName(ctx context.Context, id string) string {
// 	user := GetUserFromContext(ctx)
// 	name, _ := r.FindColumnValue(user, id, "hpc_user", "name", "username", false)
// 	if name != "" {
// 		return name
// 	} else {
// 		return "-"
// 	}
// }

func (r *JobRepository) getCastType() string {
	var castType string

	switch r.driver {
	case "sqlite3":
		castType = "int"
	case "mysql":
		castType = "unsigned"
	default:
		castType = ""
	}

	return castType
}

func (r *JobRepository) JobsStatsGrouped(
	ctx context.Context,
	filter []*model.JobFilter,
	page *model.PageRequest,
	sortBy *model.SortByAggregate,
	groupBy *model.Aggregate,
) ([]*model.JobsStatistics, error) {
	start := time.Now()
	col := groupBy2column[*groupBy]
	query := r.buildStatsQuery(filter, col)

	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}

	if sortBy != nil {
		sortBy := sortBy2column[*sortBy]
		query = query.OrderBy(fmt.Sprintf("%s DESC", sortBy))
	}
	if page != nil && page.ItemsPerPage != -1 {
		limit := uint64(page.ItemsPerPage)
		query = query.Offset((uint64(page.Page) - 1) * limit).Limit(limit)
	}

	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	stats := make([]*model.JobsStatistics, 0, 100)

	for rows.Next() {
		var id sql.NullString
		var name sql.NullString
		var jobs, walltime, nodes, nodeHours, cores, coreHours, accs, accHours sql.NullInt64
		if err := rows.Scan(&id, &jobs, &name, &walltime, &nodes, &nodeHours, &cores, &coreHours, &accs, &accHours); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			var totalJobs, totalWalltime, totalNodes, totalNodeHours, totalCores, totalCoreHours, totalAccs, totalAccHours int
			var personName string

			if name.Valid {
				personName = name.String
			}

			if jobs.Valid {
				totalJobs = int(jobs.Int64)
			}

			if walltime.Valid {
				totalWalltime = int(walltime.Int64)
			}

			if nodes.Valid {
				totalNodes = int(nodes.Int64)
			}
			if cores.Valid {
				totalCores = int(cores.Int64)
			}
			if accs.Valid {
				totalAccs = int(accs.Int64)
			}

			if nodeHours.Valid {
				totalNodeHours = int(nodeHours.Int64)
			}
			if coreHours.Valid {
				totalCoreHours = int(coreHours.Int64)
			}
			if accHours.Valid {
				totalAccHours = int(accHours.Int64)
			}

			if col == "job.hpc_user" {
				// name := r.getUserName(ctx, id.String)
				stats = append(stats,
					&model.JobsStatistics{
						ID:             id.String,
						Name:           personName,
						TotalJobs:      totalJobs,
						TotalWalltime:  totalWalltime,
						TotalNodes:     totalNodes,
						TotalNodeHours: totalNodeHours,
						TotalCores:     totalCores,
						TotalCoreHours: totalCoreHours,
						TotalAccs:      totalAccs,
						TotalAccHours:  totalAccHours,
					})
			} else {
				stats = append(stats,
					&model.JobsStatistics{
						ID:             id.String,
						TotalJobs:      int(jobs.Int64),
						TotalWalltime:  int(walltime.Int64),
						TotalNodes:     totalNodes,
						TotalNodeHours: totalNodeHours,
						TotalCores:     totalCores,
						TotalCoreHours: totalCoreHours,
						TotalAccs:      totalAccs,
						TotalAccHours:  totalAccHours,
					})
			}
		}
	}

	cclog.Debugf("Timer JobsStatsGrouped %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) JobsStats(
	ctx context.Context,
	filter []*model.JobFilter,
) ([]*model.JobsStatistics, error) {
	start := time.Now()
	query := r.buildStatsQuery(filter, "")
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}

	row := query.RunWith(r.DB).QueryRow()
	stats := make([]*model.JobsStatistics, 0, 1)

	var jobs, walltime, nodes, nodeHours, cores, coreHours, accs, accHours sql.NullInt64
	if err := row.Scan(&jobs, &walltime, &nodes, &nodeHours, &cores, &coreHours, &accs, &accHours); err != nil {
		cclog.Warn("Error while scanning rows")
		return nil, err
	}

	if jobs.Valid {
		var totalNodeHours, totalCoreHours, totalAccHours int

		if nodeHours.Valid {
			totalNodeHours = int(nodeHours.Int64)
		}
		if coreHours.Valid {
			totalCoreHours = int(coreHours.Int64)
		}
		if accHours.Valid {
			totalAccHours = int(accHours.Int64)
		}
		stats = append(stats,
			&model.JobsStatistics{
				TotalJobs:      int(jobs.Int64),
				TotalWalltime:  int(walltime.Int64),
				TotalNodeHours: totalNodeHours,
				TotalCoreHours: totalCoreHours,
				TotalAccHours:  totalAccHours,
			})
	}

	cclog.Debugf("Timer JobStats %s", time.Since(start))
	return stats, nil
}

func LoadJobStat(job *schema.Job, metric string, statType string) float64 {
	if stats, ok := job.Statistics[metric]; ok {
		switch statType {
		case "avg":
			return stats.Avg
		case "max":
			return stats.Max
		case "min":
			return stats.Min
		default:
			cclog.Errorf("Unknown stat type %s", statType)
		}
	}

	return 0.0
}

func (r *JobRepository) JobCountGrouped(
	ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate,
) ([]*model.JobsStatistics, error) {
	start := time.Now()
	col := groupBy2column[*groupBy]
	query := r.buildCountQuery(filter, "", col)
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	stats := make([]*model.JobsStatistics, 0, 100)

	for rows.Next() {
		var id sql.NullString
		var cnt sql.NullInt64
		if err := rows.Scan(&id, &cnt); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}
		if id.Valid {
			stats = append(stats,
				&model.JobsStatistics{
					ID:        id.String,
					TotalJobs: int(cnt.Int64),
				})
		}
	}

	cclog.Debugf("Timer JobCountGrouped %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) AddJobCountGrouped(
	ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate,
	stats []*model.JobsStatistics,
	kind string,
) ([]*model.JobsStatistics, error) {
	start := time.Now()
	col := groupBy2column[*groupBy]
	query := r.buildCountQuery(filter, kind, col)
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	counts := make(map[string]int)

	for rows.Next() {
		var id sql.NullString
		var cnt sql.NullInt64
		if err := rows.Scan(&id, &cnt); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}
		if id.Valid {
			counts[id.String] = int(cnt.Int64)
		}
	}

	switch kind {
	case "running":
		for _, s := range stats {
			s.RunningJobs = counts[s.ID]
		}
	case "short":
		for _, s := range stats {
			s.ShortJobs = counts[s.ID]
		}
	}

	cclog.Debugf("Timer AddJobCountGrouped %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) AddJobCount(
	ctx context.Context,
	filter []*model.JobFilter,
	stats []*model.JobsStatistics,
	kind string,
) ([]*model.JobsStatistics, error) {
	start := time.Now()
	query := r.buildCountQuery(filter, kind, "")
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	var count int

	for rows.Next() {
		var cnt sql.NullInt64
		if err := rows.Scan(&cnt); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		count = int(cnt.Int64)
	}

	switch kind {
	case "running":
		for _, s := range stats {
			s.RunningJobs = count
		}
	case "short":
		for _, s := range stats {
			s.ShortJobs = count
		}
	}

	cclog.Debugf("Timer AddJobCount %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) AddHistograms(
	ctx context.Context,
	filter []*model.JobFilter,
	stat *model.JobsStatistics,
	durationBins *string,
) (*model.JobsStatistics, error) {
	start := time.Now()

	var targetBinCount int
	var targetBinSize int
	switch {
	case *durationBins == "1m": // 1 Minute Bins + Max 60 Bins -> Max 60 Minutes
		targetBinCount = 60
		targetBinSize = 60
	case *durationBins == "10m": // 10 Minute Bins + Max 72 Bins -> Max 12 Hours
		targetBinCount = 72
		targetBinSize = 600
	case *durationBins == "1h": // 1 Hour Bins + Max 48 Bins -> Max 48 Hours
		targetBinCount = 48
		targetBinSize = 3600
	case *durationBins == "6h": // 6 Hour Bins + Max 12 Bins -> Max 3 Days
		targetBinCount = 12
		targetBinSize = 21600
	case *durationBins == "12h": // 12 hour Bins + Max 14 Bins -> Max 7 Days
		targetBinCount = 14
		targetBinSize = 43200
	default: // 24h
		targetBinCount = 24
		targetBinSize = 3600
	}

	castType := r.getCastType()
	var err error
	// Return X-Values always as seconds, will be formatted into minutes and hours in frontend
	value := fmt.Sprintf(`CAST(ROUND(((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) / %d) + 1) as %s) as value`, time.Now().Unix(), targetBinSize, castType)
	stat.HistDuration, err = r.jobsDurationStatisticsHistogram(ctx, value, filter, targetBinSize, &targetBinCount)
	if err != nil {
		cclog.Warn("Error while loading job statistics histogram: job duration")
		return nil, err
	}

	stat.HistNumNodes, err = r.jobsStatisticsHistogram(ctx, "job.num_nodes as value", filter)
	if err != nil {
		cclog.Warn("Error while loading job statistics histogram: num nodes")
		return nil, err
	}

	stat.HistNumCores, err = r.jobsStatisticsHistogram(ctx, "job.num_hwthreads as value", filter)
	if err != nil {
		cclog.Warn("Error while loading job statistics histogram: num hwthreads")
		return nil, err
	}

	stat.HistNumAccs, err = r.jobsStatisticsHistogram(ctx, "job.num_acc as value", filter)
	if err != nil {
		cclog.Warn("Error while loading job statistics histogram: num acc")
		return nil, err
	}

	cclog.Debugf("Timer AddHistograms %s", time.Since(start))
	return stat, nil
}

// Requires thresholds for metric from config for cluster? Of all clusters and use largest? split to 10 + 1 for artifacts?
func (r *JobRepository) AddMetricHistograms(
	ctx context.Context,
	filter []*model.JobFilter,
	metrics []string,
	stat *model.JobsStatistics,
	targetBinCount *int,
) (*model.JobsStatistics, error) {
	start := time.Now()

	// Running Jobs Only: First query jobdata from sqlite, then query data and make bins
	for _, f := range filter {
		if f.State != nil {
			if len(f.State) == 1 && f.State[0] == "running" {
				stat.HistMetrics = r.runningJobsMetricStatisticsHistogram(ctx, metrics, filter, targetBinCount)
				cclog.Debugf("Timer AddMetricHistograms %s", time.Since(start))
				return stat, nil
			}
		}
	}

	// All other cases: Query and make bins in sqlite directly
	for _, m := range metrics {
		metricHisto, err := r.jobsMetricStatisticsHistogram(ctx, m, filter, targetBinCount)
		if err != nil {
			cclog.Warnf("Error while loading job metric statistics histogram: %s", m)
			continue
		}
		stat.HistMetrics = append(stat.HistMetrics, metricHisto)
	}

	cclog.Debugf("Timer AddMetricHistograms %s", time.Since(start))
	return stat, nil
}

// `value` must be the column grouped by, but renamed to "value"
func (r *JobRepository) jobsStatisticsHistogram(
	ctx context.Context,
	value string,
	filters []*model.JobFilter,
) ([]*model.HistoPoint, error) {
	start := time.Now()
	query, qerr := SecurityCheck(ctx,
		sq.Select(value, "COUNT(job.id) AS count").From("job"))

	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	rows, err := query.GroupBy("value").RunWith(r.DB).Query()
	if err != nil {
		cclog.Error("Error while running query")
		return nil, err
	}

	points := make([]*model.HistoPoint, 0)
	// is it possible to introduce zero values here? requires info about bincount
	for rows.Next() {
		point := model.HistoPoint{}
		if err := rows.Scan(&point.Value, &point.Count); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		points = append(points, &point)
	}
	cclog.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return points, nil
}

func (r *JobRepository) jobsDurationStatisticsHistogram(
	ctx context.Context,
	value string,
	filters []*model.JobFilter,
	binSizeSeconds int,
	targetBinCount *int,
) ([]*model.HistoPoint, error) {
	start := time.Now()
	query, qerr := SecurityCheck(ctx,
		sq.Select(value, "COUNT(job.id) AS count").From("job"))

	if qerr != nil {
		return nil, qerr
	}

	// Setup Array
	points := make([]*model.HistoPoint, 0)
	for i := 1; i <= *targetBinCount; i++ {
		point := model.HistoPoint{Value: i * binSizeSeconds, Count: 0}
		points = append(points, &point)
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	rows, err := query.GroupBy("value").RunWith(r.DB).Query()
	if err != nil {
		cclog.Error("Error while running query")
		return nil, err
	}

	// Fill Array at matching $Value
	for rows.Next() {
		point := model.HistoPoint{}
		if err := rows.Scan(&point.Value, &point.Count); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		for _, e := range points {
			if e.Value == (point.Value * binSizeSeconds) {
				// Note:
				//  Matching on unmodified integer value (and multiplying point.Value by binSizeSeconds after match)
				//  causes frontend to loop into highest targetBinCount, due to zoom condition instantly being fullfilled (cause unknown)
				e.Count = point.Count
				break
			}
		}
	}

	cclog.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return points, nil
}

func (r *JobRepository) jobsMetricStatisticsHistogram(
	ctx context.Context,
	metric string,
	filters []*model.JobFilter,
	bins *int,
) (*model.MetricHistoPoints, error) {
	// Get specific Peak or largest Peak
	var metricConfig *schema.MetricConfig
	var peak float64
	var unit string
	var footprintStat string

	for _, f := range filters {
		if f.Cluster != nil {
			metricConfig = archive.GetMetricConfig(*f.Cluster.Eq, metric)
			peak = metricConfig.Peak
			unit = metricConfig.Unit.Prefix + metricConfig.Unit.Base
			footprintStat = metricConfig.Footprint
			cclog.Debugf("Cluster %s filter found with peak %f for %s", *f.Cluster.Eq, peak, metric)
		}
	}

	if peak == 0.0 {
		for _, c := range archive.Clusters {
			for _, m := range c.MetricConfig {
				if m.Name == metric {
					if m.Peak > peak {
						peak = m.Peak
					}
					if unit == "" {
						unit = m.Unit.Prefix + m.Unit.Base
					}
					if footprintStat == "" {
						footprintStat = m.Footprint
					}
				}
			}
		}
	}

	// cclog.Debugf("Metric %s, Peak %f, Unit %s", metric, peak, unit)
	// Make bins, see https://jereze.com/code/sql-histogram/ (Modified here)
	start := time.Now()

	// Find Jobs' Value Bin Number: Divide Value by Peak, Multiply by RequestedBins, then CAST to INT: Gets Bin-Number of Job
	binQuery := fmt.Sprintf(`CAST(
		((case when json_extract(footprint, "$.%s") = %f then %f*0.999999999 else json_extract(footprint, "$.%s") end) / %f)
		* %v as INTEGER )`,
		(metric + "_" + footprintStat), peak, peak, (metric + "_" + footprintStat), peak, *bins)

	mainQuery := sq.Select(
		fmt.Sprintf(`%s + 1 as bin`, binQuery),
		`count(*) as count`,
		// For Debug: // fmt.Sprintf(`CAST((%f / %d) as INTEGER ) * %s as min`, peak, *bins, binQuery),
		// For Debug: // fmt.Sprintf(`CAST((%f / %d) as INTEGER ) * (%s + 1) as max`, peak, *bins, binQuery),
	).From("job").Where(
		"JSON_VALID(footprint)",
	).Where(fmt.Sprintf(`json_extract(footprint, "$.%s") is not null and json_extract(footprint, "$.%s") <= %f`, (metric + "_" + footprintStat), (metric + "_" + footprintStat), peak))

	// Only accessible Jobs...
	mainQuery, qerr := SecurityCheck(ctx, mainQuery)
	if qerr != nil {
		return nil, qerr
	}

	// Filters...
	for _, f := range filters {
		mainQuery = BuildWhereClause(f, mainQuery)
	}

	// Finalize query with Grouping and Ordering
	mainQuery = mainQuery.GroupBy("bin").OrderBy("bin")

	rows, err := mainQuery.RunWith(r.DB).Query()
	if err != nil {
		cclog.Errorf("Error while running mainQuery: %s", err)
		return nil, err
	}

	// Setup Return Array With Bin-Numbers for Match and Min/Max based on Peak
	points := make([]*model.MetricHistoPoint, 0)
	binStep := int(peak) / *bins
	for i := 1; i <= *bins; i++ {
		binMin := (binStep * (i - 1))
		binMax := (binStep * i)
		epoint := model.MetricHistoPoint{Bin: &i, Count: 0, Min: &binMin, Max: &binMax}
		points = append(points, &epoint)
	}

	for rows.Next() { // Fill Count if Bin-No. Matches (Not every Bin exists in DB!)
		rpoint := model.MetricHistoPoint{}
		if err := rows.Scan(&rpoint.Bin, &rpoint.Count); err != nil { // Required for Debug: &rpoint.Min, &rpoint.Max
			cclog.Warnf("Error while scanning rows for %s", metric)
			return nil, err // FIXME: Totally bricks cc-backend if returned and if all metrics requested?
		}

		for _, e := range points {
			if e.Bin != nil && rpoint.Bin != nil {
				if *e.Bin == *rpoint.Bin {
					e.Count = rpoint.Count
					// Only Required For Debug: Check DB returned Min/Max against Backend Init above
					// if rpoint.Min != nil {
					// 	cclog.Warnf(">>>> Bin %d Min Set For %s to %d (Init'd with: %d)", *e.Bin, metric, *rpoint.Min, *e.Min)
					// }
					// if rpoint.Max != nil {
					// 	cclog.Warnf(">>>> Bin %d Max Set For %s to %d (Init'd with: %d)", *e.Bin, metric, *rpoint.Max, *e.Max)
					// }
					break
				}
			}
		}
	}

	result := model.MetricHistoPoints{Metric: metric, Unit: unit, Stat: &footprintStat, Data: points}

	cclog.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return &result, nil
}

func (r *JobRepository) runningJobsMetricStatisticsHistogram(
	ctx context.Context,
	metrics []string,
	filters []*model.JobFilter,
	bins *int,
) []*model.MetricHistoPoints {
	// Get Jobs
	jobs, err := r.QueryJobs(ctx, filters, &model.PageRequest{Page: 1, ItemsPerPage: 500 + 1}, nil)
	if err != nil {
		cclog.Errorf("Error while querying jobs for footprint: %s", err)
		return nil
	}
	if len(jobs) > 500 {
		cclog.Errorf("too many jobs matched (max: %d)", 500)
		return nil
	}

	// Get AVGs from metric repo
	avgs := make([][]schema.Float, len(metrics))
	for i := range avgs {
		avgs[i] = make([]schema.Float, 0, len(jobs))
	}

	for _, job := range jobs {
		if job.MonitoringStatus == schema.MonitoringStatusDisabled || job.MonitoringStatus == schema.MonitoringStatusArchivingFailed {
			continue
		}

		if err := metricDataDispatcher.LoadAverages(job, metrics, avgs, ctx); err != nil {
			cclog.Errorf("Error while loading averages for histogram: %s", err)
			return nil
		}
	}

	// Iterate metrics to fill endresult
	data := make([]*model.MetricHistoPoints, 0)
	for idx, metric := range metrics {
		// Get specific Peak or largest Peak
		var metricConfig *schema.MetricConfig
		var peak float64
		var unit string

		for _, f := range filters {
			if f.Cluster != nil {
				metricConfig = archive.GetMetricConfig(*f.Cluster.Eq, metric)
				peak = metricConfig.Peak
				unit = metricConfig.Unit.Prefix + metricConfig.Unit.Base
			}
		}

		if peak == 0.0 {
			for _, c := range archive.Clusters {
				for _, m := range c.MetricConfig {
					if m.Name == metric {
						if m.Peak > peak {
							peak = m.Peak
						}
						if unit == "" {
							unit = m.Unit.Prefix + m.Unit.Base
						}
					}
				}
			}
		}

		// Make and fill bins
		peakBin := int(peak) / *bins

		points := make([]*model.MetricHistoPoint, 0)
		for b := 0; b < *bins; b++ {
			count := 0
			bindex := b + 1
			bmin := peakBin * b
			bmax := peakBin * (b + 1)

			// Iterate AVG values for indexed metric and count for bins
			for _, val := range avgs[idx] {
				if int(val) >= bmin && int(val) < bmax {
					count += 1
				}
			}

			// Append Bin to Metric Result Array
			point := model.MetricHistoPoint{Bin: &bindex, Count: count, Min: &bmin, Max: &bmax}
			points = append(points, &point)
		}

		// Append Metric Result Array to final results array
		result := model.MetricHistoPoints{Metric: metric, Unit: unit, Data: points}
		data = append(data, &result)
	}

	return data
}
