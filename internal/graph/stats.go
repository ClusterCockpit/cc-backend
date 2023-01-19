// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package graph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
)

// GraphQL validation should make sure that no unkown values can be specified.
var groupBy2column = map[model.Aggregate]string{
	model.AggregateUser:    "job.user",
	model.AggregateProject: "job.project",
	model.AggregateCluster: "job.cluster",
}

const ShortJobDuration int = 5 * 60

// Helper function for the jobsStatistics GraphQL query placed here so that schema.resolvers.go is not too full.
func (r *queryResolver) jobsStatistics(ctx context.Context, filter []*model.JobFilter, groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {
	// In case `groupBy` is nil (not used), the model.JobsStatistics used is at the key '' (empty string)
	stats := map[string]*model.JobsStatistics{}

	// `socketsPerNode` and `coresPerSocket` can differ from cluster to cluster, so we need to explicitly loop over those.
	for _, cluster := range archive.Clusters {
		for _, subcluster := range cluster.SubClusters {
			corehoursCol := fmt.Sprintf("CAST(ROUND(SUM(job.duration * job.num_nodes * %d * %d) / 3600) as int)", subcluster.SocketsPerNode, subcluster.CoresPerSocket)
			var query sq.SelectBuilder
			if groupBy == nil {
				query = sq.Select(
					"''",
					"COUNT(job.id)",
					"CAST(ROUND(SUM(job.duration) / 3600) as int)",
					corehoursCol,
				).From("job")
			} else {
				col := groupBy2column[*groupBy]
				query = sq.Select(
					col,
					"COUNT(job.id)",
					"CAST(ROUND(SUM(job.duration) / 3600) as int)",
					corehoursCol,
				).From("job").GroupBy(col)
			}

			query = query.
				Where("job.cluster = ?", cluster.Name).
				Where("job.subcluster = ?", subcluster.Name)

			query = repository.SecurityCheck(ctx, query)
			for _, f := range filter {
				query = repository.BuildWhereClause(f, query)
			}

			rows, err := query.RunWith(r.DB).Query()
			if err != nil {
				return nil, err
			}

			for rows.Next() {
				var id sql.NullString
				var jobs, walltime, corehours sql.NullInt64
				if err := rows.Scan(&id, &jobs, &walltime, &corehours); err != nil {
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
		query := sq.Select("COUNT(job.id)").From("job").Where("job.duration < ?", ShortJobDuration)
		query = repository.SecurityCheck(ctx, query)
		for _, f := range filter {
			query = repository.BuildWhereClause(f, query)
		}
		if err := query.RunWith(r.DB).QueryRow().Scan(&(stats[""].ShortJobs)); err != nil {
			return nil, err
		}
	} else {
		col := groupBy2column[*groupBy]
		query := sq.Select(col, "COUNT(job.id)").From("job").Where("job.duration < ?", ShortJobDuration)
		query = repository.SecurityCheck(ctx, query)
		for _, f := range filter {
			query = repository.BuildWhereClause(f, query)
		}
		rows, err := query.RunWith(r.DB).Query()
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var id sql.NullString
			var shortJobs sql.NullInt64
			if err := rows.Scan(&id, &shortJobs); err != nil {
				return nil, err
			}

			if id.Valid {
				stats[id.String].ShortJobs = int(shortJobs.Int64)
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
			value := fmt.Sprintf(`CAST(ROUND((CASE WHEN job.job_state = "running" THEN %d - job.start_time ELSE job.duration END) / 3600) as int) as value`, time.Now().Unix())
			stat.HistDuration, err = r.jobsStatisticsHistogram(ctx, value, filter, id, col)
			if err != nil {
				return nil, err
			}

			stat.HistNumNodes, err = r.jobsStatisticsHistogram(ctx, "job.num_nodes as value", filter, id, col)
			if err != nil {
				return nil, err
			}
		}
	}

	return res, nil
}

// `value` must be the column grouped by, but renamed to "value". `id` and `col` can optionally be used
// to add a condition to the query of the kind "<col> = <id>".
func (r *queryResolver) jobsStatisticsHistogram(ctx context.Context, value string, filters []*model.JobFilter, id, col string) ([]*model.HistoPoint, error) {
	query := sq.Select(value, "COUNT(job.id) AS count").From("job")
	query = repository.SecurityCheck(ctx, query)
	for _, f := range filters {
		query = repository.BuildWhereClause(f, query)
	}

	if len(id) != 0 && len(col) != 0 {
		query = query.Where(col+" = ?", id)
	}

	rows, err := query.GroupBy("value").RunWith(r.DB).Query()
	if err != nil {
		return nil, err
	}

	points := make([]*model.HistoPoint, 0)
	for rows.Next() {
		point := model.HistoPoint{}
		if err := rows.Scan(&point.Value, &point.Count); err != nil {
			return nil, err
		}

		points = append(points, &point)
	}
	return points, nil
}

const MAX_JOBS_FOR_ANALYSIS = 500

// Helper function for the rooflineHeatmap GraphQL query placed here so that schema.resolvers.go is not too full.
func (r *queryResolver) rooflineHeatmap(
	ctx context.Context,
	filter []*model.JobFilter,
	rows int, cols int,
	minX float64, minY float64, maxX float64, maxY float64) ([][]float64, error) {

	jobs, err := r.Repo.QueryJobs(ctx, filter, &model.PageRequest{Page: 1, ItemsPerPage: MAX_JOBS_FOR_ANALYSIS + 1}, nil)
	if err != nil {
		return nil, err
	}
	if len(jobs) > MAX_JOBS_FOR_ANALYSIS {
		return nil, fmt.Errorf("GRAPH/STATS > too many jobs matched (max: %d)", MAX_JOBS_FOR_ANALYSIS)
	}

	fcols, frows := float64(cols), float64(rows)
	minX, minY, maxX, maxY = math.Log10(minX), math.Log10(minY), math.Log10(maxX), math.Log10(maxY)
	tiles := make([][]float64, rows)
	for i := range tiles {
		tiles[i] = make([]float64, cols)
	}

	for _, job := range jobs {
		if job.MonitoringStatus == schema.MonitoringStatusDisabled || job.MonitoringStatus == schema.MonitoringStatusArchivingFailed {
			continue
		}

		jobdata, err := metricdata.LoadData(job, []string{"flops_any", "mem_bw"}, []schema.MetricScope{schema.MetricScopeNode}, ctx)
		if err != nil {
			return nil, err
		}

		flops_, membw_ := jobdata["flops_any"], jobdata["mem_bw"]
		if flops_ == nil && membw_ == nil {
			return nil, fmt.Errorf("GRAPH/STATS > 'flops_any' or 'mem_bw' missing for job %d", job.ID)
		}

		flops, ok1 := flops_["node"]
		membw, ok2 := membw_["node"]
		if !ok1 || !ok2 {
			// TODO/FIXME:
			return nil, errors.New("GRAPH/STATS > todo: rooflineHeatmap() query not implemented for where flops_any or mem_bw not available at 'node' level")
		}

		for n := 0; n < len(flops.Series); n++ {
			flopsSeries, membwSeries := flops.Series[n], membw.Series[n]
			for i := 0; i < len(flopsSeries.Data); i++ {
				if i >= len(membwSeries.Data) {
					break
				}

				x, y := math.Log10(float64(flopsSeries.Data[i]/membwSeries.Data[i])), math.Log10(float64(flopsSeries.Data[i]))
				if math.IsNaN(x) || math.IsNaN(y) || x < minX || x >= maxX || y < minY || y > maxY {
					continue
				}

				x, y = math.Floor(((x-minX)/(maxX-minX))*fcols), math.Floor(((y-minY)/(maxY-minY))*frows)
				if x < 0 || x >= fcols || y < 0 || y >= frows {
					continue
				}

				tiles[int(y)][int(x)] += 1
			}
		}
	}

	return tiles, nil
}

// Helper function for the jobsFootprints GraphQL query placed here so that schema.resolvers.go is not too full.
func (r *queryResolver) jobsFootprints(ctx context.Context, filter []*model.JobFilter, metrics []string) (*model.Footprints, error) {
	jobs, err := r.Repo.QueryJobs(ctx, filter, &model.PageRequest{Page: 1, ItemsPerPage: MAX_JOBS_FOR_ANALYSIS + 1}, nil)
	if err != nil {
		return nil, err
	}
	if len(jobs) > MAX_JOBS_FOR_ANALYSIS {
		return nil, fmt.Errorf("GRAPH/STATS > too many jobs matched (max: %d)", MAX_JOBS_FOR_ANALYSIS)
	}

	avgs := make([][]schema.Float, len(metrics))
	for i := range avgs {
		avgs[i] = make([]schema.Float, 0, len(jobs))
	}

	nodehours := make([]schema.Float, 0, len(jobs))
	for _, job := range jobs {
		if job.MonitoringStatus == schema.MonitoringStatusDisabled || job.MonitoringStatus == schema.MonitoringStatusArchivingFailed {
			continue
		}

		if err := metricdata.LoadAverages(job, metrics, avgs, ctx); err != nil {
			return nil, err
		}

		nodehours = append(nodehours, schema.Float(float64(job.Duration)/60.0*float64(job.NumNodes)))
	}

	res := make([]*model.MetricFootprints, len(avgs))
	for i, arr := range avgs {
		res[i] = &model.MetricFootprints{
			Metric: metrics[i],
			Data:   arr,
		}
	}

	return &model.Footprints{
		Nodehours: nodehours,
		Metrics:   res,
	}, nil
}
