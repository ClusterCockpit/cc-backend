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

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	sq "github.com/Masterminds/squirrel"
)

// GraphQL validation should make sure that no unkown values can be specified.
var groupBy2column = map[model.Aggregate]string{
	model.AggregateUser:    "job.user",
	model.AggregateProject: "job.project",
	model.AggregateCluster: "job.cluster",
}

func (r *JobRepository) buildJobsStatsQuery(
	filter []*model.JobFilter,
	col string) sq.SelectBuilder {

	var query sq.SelectBuilder
	castType := r.getCastType()

	if col != "" {
		// Scan columns: id, totalJobs, totalWalltime
		query = sq.Select(col, "COUNT(job.id)",
			fmt.Sprintf("CAST(ROUND(SUM(job.duration) / 3600) as %s)", castType),
		).From("job").GroupBy(col)
	} else {
		// Scan columns: totalJobs, totalWalltime
		query = sq.Select("COUNT(job.id)",
			fmt.Sprintf("CAST(ROUND(SUM(job.duration) / 3600) as %s)", castType),
		).From("job")
	}

	for _, f := range filter {
		query = BuildWhereClause(f, query)
	}

	return query
}

func (r *JobRepository) getUserName(ctx context.Context, id string) string {
	user := auth.GetUser(ctx)
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

// with groupBy and without coreHours
func (r *JobRepository) JobsStatsNoCoreH(
	ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {

	start := time.Now()
	col := groupBy2column[*groupBy]
	query := r.buildJobsStatsQuery(filter, col)
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
		var jobs, walltime sql.NullInt64
		if err := rows.Scan(&id, &jobs, &walltime); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}

		if id.Valid {
			if col == "job.user" {
				name := r.getUserName(ctx, id.String)
				stats = append(stats,
					&model.JobsStatistics{
						ID:            id.String,
						Name:          &name,
						TotalJobs:     int(jobs.Int64),
						TotalWalltime: int(walltime.Int64)})
			} else {
				stats = append(stats,
					&model.JobsStatistics{
						ID:            id.String,
						TotalJobs:     int(jobs.Int64),
						TotalWalltime: int(walltime.Int64)})
			}
		}
	}

	log.Infof("Timer JobStatistics %s", time.Since(start))
	return stats, nil
}

// without groupBy and without coreHours
func (r *JobRepository) JobsStatsPlainNoCoreH(
	ctx context.Context,
	filter []*model.JobFilter) ([]*model.JobsStatistics, error) {

	start := time.Now()
	query := r.buildJobsStatsQuery(filter, "")
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}

	row := query.RunWith(r.DB).QueryRow()
	stats := make([]*model.JobsStatistics, 0, 1)
	var jobs, walltime sql.NullInt64
	if err := row.Scan(&jobs, &walltime); err != nil {
		log.Warn("Error while scanning rows")
		return nil, err
	}

	if jobs.Valid {
		stats = append(stats,
			&model.JobsStatistics{
				TotalJobs:     int(jobs.Int64),
				TotalWalltime: int(walltime.Int64)})
	}

	log.Infof("Timer JobStatistics %s", time.Since(start))
	return stats, nil
}

// without groupBy and with coreHours
func (r *JobRepository) JobsStatsPlain(
	ctx context.Context,
	filter []*model.JobFilter) ([]*model.JobsStatistics, error) {

	start := time.Now()
	query := r.buildJobsStatsQuery(filter, "")
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}

	castType := r.getCastType()
	var totalJobs, totalWalltime, totalCoreHours int64

	for _, cluster := range archive.Clusters {
		for _, subcluster := range cluster.SubClusters {

			scQuery := query.Column(fmt.Sprintf(
				"CAST(ROUND(SUM(job.duration * job.num_nodes * %d * %d) / 3600) as %s)",
				subcluster.SocketsPerNode, subcluster.CoresPerSocket, castType))
			scQuery = scQuery.Where("job.cluster = ?", cluster.Name).
				Where("job.subcluster = ?", subcluster.Name)

			row := scQuery.RunWith(r.DB).QueryRow()
			var jobs, walltime, corehours sql.NullInt64
			if err := row.Scan(&jobs, &walltime, &corehours); err != nil {
				log.Warn("Error while scanning rows")
				return nil, err
			}

			if jobs.Valid {
				totalJobs += jobs.Int64
				totalWalltime += walltime.Int64
				totalCoreHours += corehours.Int64
			}
		}
	}
	stats := make([]*model.JobsStatistics, 0, 1)
	stats = append(stats,
		&model.JobsStatistics{
			TotalJobs:      int(totalJobs),
			TotalWalltime:  int(totalWalltime),
			TotalCoreHours: int(totalCoreHours)})

	log.Infof("Timer JobStatistics %s", time.Since(start))
	return stats, nil
}

// with groupBy and with coreHours
func (r *JobRepository) JobsStats(
	ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {

	start := time.Now()

	stats := map[string]*model.JobsStatistics{}
	col := groupBy2column[*groupBy]
	query := r.buildJobsStatsQuery(filter, col)
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}

	castType := r.getCastType()

	for _, cluster := range archive.Clusters {
		for _, subcluster := range cluster.SubClusters {

			scQuery := query.Column(fmt.Sprintf(
				"CAST(ROUND(SUM(job.duration * job.num_nodes * %d * %d) / 3600) as %s)",
				subcluster.SocketsPerNode, subcluster.CoresPerSocket, castType))

			scQuery = scQuery.Where("job.cluster = ?", cluster.Name).
				Where("job.subcluster = ?", subcluster.Name)

			rows, err := scQuery.RunWith(r.DB).Query()
			if err != nil {
				log.Warn("Error while querying DB for job statistics")
				return nil, err
			}

			for rows.Next() {
				var id sql.NullString
				var jobs, walltime, corehours sql.NullInt64
				if err := rows.Scan(&id, &jobs, &walltime, &corehours); err != nil {
					log.Warn("Error while scanning rows")
					return nil, err
				}

				if s, ok := stats[id.String]; ok {
					s.TotalJobs += int(jobs.Int64)
					s.TotalWalltime += int(walltime.Int64)
					s.TotalCoreHours += int(corehours.Int64)
				} else {
					if col == "job.user" {
						name := r.getUserName(ctx, id.String)
						stats[id.String] = &model.JobsStatistics{
							ID:             id.String,
							Name:           &name,
							TotalJobs:      int(jobs.Int64),
							TotalWalltime:  int(walltime.Int64),
							TotalCoreHours: int(corehours.Int64),
						}
					} else {
						stats[id.String] = &model.JobsStatistics{
							ID:             id.String,
							TotalJobs:      int(jobs.Int64),
							TotalWalltime:  int(walltime.Int64),
							TotalCoreHours: int(corehours.Int64),
						}
					}
				}
			}
		}
	}

	res := make([]*model.JobsStatistics, 0, len(stats))
	for _, stat := range stats {
		res = append(res, stat)
	}

	log.Infof("Timer JobStatistics %s", time.Since(start))
	return res, nil
}

func (r *JobRepository) AddHistograms(
	ctx context.Context,
	filter []*model.JobFilter,
	stat *model.JobsStatistics) (*model.JobsStatistics, error) {

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
	log.Infof("Timer jobsStatisticsHistogram %s", time.Since(start))
	return points, nil
}
