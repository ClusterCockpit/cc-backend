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

	"github.com/99designs/gqlgen/graphql"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
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

// Helper function for the jobsStatistics GraphQL query placed here so that schema.resolvers.go is not too full.
func (r *JobRepository) JobsStatistics(ctx context.Context,
	filter []*model.JobFilter,
	groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {

	start := time.Now()
	// In case `groupBy` is nil (not used), the model.JobsStatistics used is at the key '' (empty string)
	stats := map[string]*model.JobsStatistics{}
	var castType string

	switch r.driver {
	case "sqlite3":
		castType = "int"
	case "mysql":
		castType = "unsigned"
	}

	// `socketsPerNode` and `coresPerSocket` can differ from cluster to cluster, so we need to explicitly loop over those.
	for _, cluster := range archive.Clusters {
		for _, subcluster := range cluster.SubClusters {
			corehoursCol := fmt.Sprintf("CAST(ROUND(SUM(job.duration * job.num_nodes * %d * %d) / 3600) as %s)", subcluster.SocketsPerNode, subcluster.CoresPerSocket, castType)
			var rawQuery sq.SelectBuilder
			if groupBy == nil {
				rawQuery = sq.Select(
					"''",
					"COUNT(job.id)",
					fmt.Sprintf("CAST(ROUND(SUM(job.duration) / 3600) as %s)", castType),
					corehoursCol,
				).From("job")
			} else {
				col := groupBy2column[*groupBy]
				rawQuery = sq.Select(
					col,
					"COUNT(job.id)",
					fmt.Sprintf("CAST(ROUND(SUM(job.duration) / 3600) as %s)", castType),
					corehoursCol,
				).From("job").GroupBy(col)
			}

			rawQuery = rawQuery.
				Where("job.cluster = ?", cluster.Name).
				Where("job.subcluster = ?", subcluster.Name)

			query, qerr := SecurityCheck(ctx, rawQuery)

			if qerr != nil {
				return nil, qerr
			}

			for _, f := range filter {
				query = BuildWhereClause(f, query)
			}

			rows, err := query.RunWith(r.DB).Query()
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

				if id.Valid {
					if s, ok := stats[id.String]; ok {
						s.TotalJobs += int(jobs.Int64)
						s.TotalWalltime += int(walltime.Int64)
						s.TotalCoreHours += int(corehours.Int64)
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

	if groupBy == nil {

		query := sq.Select("COUNT(job.id)").From("job").Where("job.duration < ?", config.Keys.ShortRunningJobsDuration)
		query, qerr := SecurityCheck(ctx, query)

		if qerr != nil {
			return nil, qerr
		}

		for _, f := range filter {
			query = BuildWhereClause(f, query)
		}
		if err := query.RunWith(r.DB).QueryRow().Scan(&(stats[""].ShortJobs)); err != nil {
			log.Warn("Error while scanning rows for short job stats")
			return nil, err
		}
	} else {
		col := groupBy2column[*groupBy]

		query := sq.Select(col, "COUNT(job.id)").From("job").Where("job.duration < ?", config.Keys.ShortRunningJobsDuration)

		query, qerr := SecurityCheck(ctx, query)

		if qerr != nil {
			return nil, qerr
		}

		for _, f := range filter {
			query = BuildWhereClause(f, query)
		}
		rows, err := query.RunWith(r.DB).Query()
		if err != nil {
			log.Warn("Error while querying jobs for short jobs")
			return nil, err
		}

		for rows.Next() {
			var id sql.NullString
			var shortJobs sql.NullInt64
			if err := rows.Scan(&id, &shortJobs); err != nil {
				log.Warn("Error while scanning rows for short jobs")
				return nil, err
			}

			if id.Valid {
				stats[id.String].ShortJobs = int(shortJobs.Int64)
			}
		}

		if col == "job.user" {
			for id := range stats {
				emptyDash := "-"
				user := auth.GetUser(ctx)
				name, _ := r.FindColumnValue(user, id, "user", "name", "username", false)
				if name != "" {
					stats[id].Name = &name
				} else {
					stats[id].Name = &emptyDash
				}
			}
		}
	}

	// Calculating the histogram data is expensive, so only do it if needed.
	// An explicit resolver can not be used because we need to know the filters.
	histogramsNeeded := false
	fields := graphql.CollectFieldsCtx(ctx, nil)
	for _, col := range fields {
		if col.Name == "histDuration" || col.Name == "histNumNodes" {
			histogramsNeeded = true
		}
	}

	res := make([]*model.JobsStatistics, 0, len(stats))
	for _, stat := range stats {
		res = append(res, stat)
		id, col := "", ""
		if groupBy != nil {
			id = stat.ID
			col = groupBy2column[*groupBy]
		}

		if histogramsNeeded {
			var err error
			value := fmt.Sprintf(`CAST(ROUND((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) / 3600) as %s) as value`, time.Now().Unix(), castType)
			stat.HistDuration, err = r.jobsStatisticsHistogram(ctx, value, filter, id, col)
			if err != nil {
				log.Warn("Error while loading job statistics histogram: running jobs")
				return nil, err
			}

			stat.HistNumNodes, err = r.jobsStatisticsHistogram(ctx, "job.num_nodes as value", filter, id, col)
			if err != nil {
				log.Warn("Error while loading job statistics histogram: num nodes")
				return nil, err
			}
		}
	}

	log.Infof("Timer JobStatistics %s", time.Since(start))
	return res, nil
}

// `value` must be the column grouped by, but renamed to "value". `id` and `col` can optionally be used
// to add a condition to the query of the kind "<col> = <id>".
func (r *JobRepository) jobsStatisticsHistogram(ctx context.Context,
	value string, filters []*model.JobFilter, id, col string) ([]*model.HistoPoint, error) {

	start := time.Now()
	query, qerr := SecurityCheck(ctx, sq.Select(value, "COUNT(job.id) AS count").From("job"))

	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	if len(id) != 0 && len(col) != 0 {
		query = query.Where(col+" = ?", id)
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
