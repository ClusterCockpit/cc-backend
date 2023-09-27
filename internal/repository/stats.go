// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
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
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	sq "github.com/Masterminds/squirrel"
)

// GraphQL validation should make sure that no unkown values can be specified.
var groupBy2column = map[model.Aggregate]string{
	model.AggregateUser:    "job.user",
	model.AggregateProject: "job.project",
	model.AggregateCluster: "job.cluster",
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
	col string) sq.SelectBuilder {

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
	col string) sq.SelectBuilder {

	var query sq.SelectBuilder
	castType := r.getCastType()

	// fmt.Sprintf(`CAST(ROUND((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) / 3600) as %s) as value`, time.Now().Unix(), castType)

	if col != "" {
		// Scan columns: id, totalJobs, totalWalltime, totalNodes, totalNodeHours, totalCores, totalCoreHours, totalAccs, totalAccHours
		query = sq.Select(col, "COUNT(job.id) as totalJobs",
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END)) / 3600) as %s) as totalWalltime`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_nodes) as %s) as totalNodes`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_nodes) / 3600) as %s) as totalNodeHours`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_hwthreads) as %s) as totalCores`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_hwthreads) / 3600) as %s) as totalCoreHours`, time.Now().Unix(), castType),
			fmt.Sprintf(`CAST(SUM(job.num_acc) as %s) as totalAccs`, castType),
			fmt.Sprintf(`CAST(ROUND(SUM((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) * job.num_acc) / 3600) as %s) as totalAccHours`, time.Now().Unix(), castType),
		).From("job").GroupBy(col)

	} else {
		// Scan columns: totalJobs, totalWalltime, totalNodes, totalNodeHours, totalCores, totalCoreHours, totalAccs, totalAccHours
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

func (r *JobRepository) getUserName(ctx context.Context, id string) string {
	user := GetUserFromContext(ctx)
	name, _ := r.FindColumnValue(user, id, "user", "name", "username", false)
	if name != "" {
		return name
	} else {
		return "-"
	}
}

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
	groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {

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
		log.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	stats := make([]*model.JobsStatistics, 0, 100)

	for rows.Next() {
		var id sql.NullString
		var jobs, walltime, nodes, nodeHours, cores, coreHours, accs, accHours sql.NullInt64
		if err := rows.Scan(&id, &jobs, &walltime, &nodes, &nodeHours, &cores, &coreHours, &accs, &accHours); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			var totalJobs, totalWalltime, totalNodes, totalNodeHours, totalCores, totalCoreHours, totalAccs, totalAccHours int

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

			if col == "job.user" {
				name := r.getUserName(ctx, id.String)
				stats = append(stats,
					&model.JobsStatistics{
						ID:             id.String,
						Name:           name,
						TotalJobs:      totalJobs,
						TotalWalltime:  totalWalltime,
						TotalNodes:     totalNodes,
						TotalNodeHours: totalNodeHours,
						TotalCores:     totalCores,
						TotalCoreHours: totalCoreHours,
						TotalAccs:      totalAccs,
						TotalAccHours:  totalAccHours})
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
						TotalAccHours:  totalAccHours})
			}
		}
	}

	log.Debugf("Timer JobsStatsGrouped %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) JobsStats(
	ctx context.Context,
	filter []*model.JobFilter) ([]*model.JobsStatistics, error) {

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
		log.Warn("Error while scanning rows")
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
				TotalAccHours:  totalAccHours})
	}

	log.Debugf("Timer JobStats %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) JobCountGrouped(
	ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {

	start := time.Now()
	col := groupBy2column[*groupBy]
	query := r.buildCountQuery(filter, "", col)
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		log.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	stats := make([]*model.JobsStatistics, 0, 100)

	for rows.Next() {
		var id sql.NullString
		var cnt sql.NullInt64
		if err := rows.Scan(&id, &cnt); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}
		if id.Valid {
			stats = append(stats,
				&model.JobsStatistics{
					ID:        id.String,
					TotalJobs: int(cnt.Int64)})
		}
	}

	log.Debugf("Timer JobCountGrouped %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) AddJobCountGrouped(
	ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate,
	stats []*model.JobsStatistics,
	kind string) ([]*model.JobsStatistics, error) {

	start := time.Now()
	col := groupBy2column[*groupBy]
	query := r.buildCountQuery(filter, kind, col)
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		log.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	counts := make(map[string]int)

	for rows.Next() {
		var id sql.NullString
		var cnt sql.NullInt64
		if err := rows.Scan(&id, &cnt); err != nil {
			log.Warn("Error while scanning rows")
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

	log.Debugf("Timer AddJobCountGrouped %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) AddJobCount(
	ctx context.Context,
	filter []*model.JobFilter,
	stats []*model.JobsStatistics,
	kind string) ([]*model.JobsStatistics, error) {

	start := time.Now()
	query := r.buildCountQuery(filter, kind, "")
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		log.Warn("Error while querying DB for job statistics")
		return nil, err
	}

	var count int

	for rows.Next() {
		var cnt sql.NullInt64
		if err := rows.Scan(&cnt); err != nil {
			log.Warn("Error while scanning rows")
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

	log.Debugf("Timer AddJobCount %s", time.Since(start))
	return stats, nil
}

func (r *JobRepository) AddHistograms(
	ctx context.Context,
	filter []*model.JobFilter,
	stat *model.JobsStatistics) (*model.JobsStatistics, error) {
	start := time.Now()

	castType := r.getCastType()
	var err error
	value := fmt.Sprintf(`CAST(ROUND((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) / 3600) as %s) as value`, time.Now().Unix(), castType)
	stat.HistDuration, err = r.jobsStatisticsHistogram(ctx, value, filter)
	if err != nil {
		log.Warn("Error while loading job statistics histogram: running jobs")
		return nil, err
	}

	stat.HistNumNodes, err = r.jobsStatisticsHistogram(ctx, "job.num_nodes as value", filter)
	if err != nil {
		log.Warn("Error while loading job statistics histogram: num nodes")
		return nil, err
	}

	stat.HistNumCores, err = r.jobsStatisticsHistogram(ctx, "job.num_hwthreads as value", filter)
	if err != nil {
		log.Warn("Error while loading job statistics histogram: num hwthreads")
		return nil, err
	}

	stat.HistNumAccs, err = r.jobsStatisticsHistogram(ctx, "job.num_acc as value", filter)
	if err != nil {
		log.Warn("Error while loading job statistics histogram: num acc")
		return nil, err
	}

	log.Debugf("Timer AddHistograms %s", time.Since(start))
	return stat, nil
}

// `value` must be the column grouped by, but renamed to "value"
func (r *JobRepository) jobsStatisticsHistogram(
	ctx context.Context,
	value string,
	filters []*model.JobFilter) ([]*model.HistoPoint, error) {

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
		log.Error("Error while running query")
		return nil, err
	}

	points := make([]*model.HistoPoint, 0)
	for rows.Next() {
		point := model.HistoPoint{}
		if err := rows.Scan(&point.Value, &point.Count); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}

		points = append(points, &point)
	}
	log.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return points, nil
}
