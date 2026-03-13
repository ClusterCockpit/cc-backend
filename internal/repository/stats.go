// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file contains job statistics and histogram generation functionality for the JobRepository.
//
// # Job Statistics
//
// The statistics methods provide aggregated metrics about jobs including total jobs, users,
// walltime, and resource usage (nodes, cores, accelerators). Statistics can be computed:
//   - Overall (JobsStats): Single aggregate across all matching jobs
//   - Grouped (JobsStatsGrouped): Aggregated by user, project, cluster, or subcluster
//   - Counts (JobCountGrouped, AddJobCount): Simple job counts with optional filtering
//
// All statistics methods support filtering via JobFilter and respect security contexts.
//
// # Histograms
//
// Histogram methods generate distribution data for visualization:
//   - Duration, nodes, cores, accelerators (AddHistograms)
//   - Job metrics like CPU load, memory usage (AddMetricHistograms)
//
// Histograms use intelligent binning:
//   - Duration: Variable bin sizes (1m, 10m, 1h, 6h, 12h, 24h) with zero-padding
//   - Resources: Natural value-based bins
//   - Metrics: Normalized to peak values with configurable bin counts
//
// # Running vs. Completed Jobs
//
// Statistics handle running jobs specially:
//   - Duration calculated as (now - start_time) for running jobs
//   - Metric histograms for running jobs load data from metric backend instead of footprint
//   - Job state filtering distinguishes running/completed jobs
//
// # Performance Considerations
//
// - All queries use prepared statements via stmtCache
// - Complex aggregations use SQL for efficiency
// - Histogram pre-initialization ensures consistent bin ranges
// - Metric histogram queries limited to 5000 jobs for running job analysis

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
)

// groupBy2column maps GraphQL Aggregate enum values to their corresponding database column names.
// Used by JobsStatsGrouped and JobCountGrouped to translate user-facing grouping dimensions
// into SQL GROUP BY clauses. GraphQL validation ensures only valid enum values are accepted.
var groupBy2column = map[model.Aggregate]string{
	model.AggregateUser:       "job.hpc_user",
	model.AggregateProject:    "job.project",
	model.AggregateCluster:    "job.cluster",
	model.AggregateSubcluster: "job.subcluster",
}

// sortBy2column maps GraphQL SortByAggregate enum values to their corresponding computed column names.
// Used by JobsStatsGrouped to translate sort preferences into SQL ORDER BY clauses.
// Column names match the AS aliases used in buildStatsQuery.
var sortBy2column = map[model.SortByAggregate]string{
	model.SortByAggregateTotaljobs:      "totalJobs",
	model.SortByAggregateTotalusers:     "totalUsers",
	model.SortByAggregateTotalwalltime:  "totalWalltime",
	model.SortByAggregateTotalnodes:     "totalNodes",
	model.SortByAggregateTotalnodehours: "totalNodeHours",
	model.SortByAggregateTotalcores:     "totalCores",
	model.SortByAggregateTotalcorehours: "totalCoreHours",
	model.SortByAggregateTotalaccs:      "totalAccs",
	model.SortByAggregateTotalacchours:  "totalAccHours",
}

// buildCountQuery constructs a SQL query to count jobs with optional grouping and filtering.
//
// Parameters:
//   - filter: Job filters to apply (cluster, user, time range, etc.)
//   - kind: Special filter - "running" for running jobs only, "short" for jobs under threshold
//   - col: Column name to GROUP BY; empty string for total count without grouping
//
// Returns a SelectBuilder that produces either:
//   - Single count: COUNT(job.id) when col is empty
//   - Grouped counts: col, COUNT(job.id) when col is specified
//
// The kind parameter enables counting specific job categories:
//   - "running": Only jobs with job_state = 'running'
//   - "short": Only jobs with duration < ShortRunningJobsDuration config value
//   - empty: All jobs matching filters
func (r *JobRepository) buildCountQuery(
	filter []*model.JobFilter,
	kind string,
	col string,
) sq.SelectBuilder {
	var query sq.SelectBuilder

	if col != "" {
		query = sq.Select(col, "COUNT(*)").From("job").GroupBy(col)
	} else {
		query = sq.Select("COUNT(*)").From("job")
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

// buildStatsQuery constructs a SQL query to compute job statistics with optional grouping.
// Only requested columns are computed; unrequested columns select 0 as placeholder.
//
// Parameters:
//   - filter: Job filters to apply (cluster, user, time range, etc.)
//   - col: Column name to GROUP BY; empty string for overall statistics without grouping
//   - shortThreshold: Duration threshold in seconds for counting short-running jobs
//   - reqFields: Set of requested field names; nil means compute all fields
func (r *JobRepository) buildStatsQuery(
	filter []*model.JobFilter,
	col string,
	shortThreshold int,
	reqFields map[string]bool,
) sq.SelectBuilder {
	now := time.Now().Unix()

	// Helper: return real expression if field is requested (or reqFields is nil), else "0 as alias"
	need := func(field string) bool {
		return reqFields == nil || reqFields[field]
	}
	durationExpr := fmt.Sprintf(`(CASE WHEN job.job_state = 'running' THEN %d - job.start_time ELSE job.duration END)`, now)

	// Build column list
	columns := make([]string, 0, 14)

	if col != "" {
		columns = append(columns, col)
	}

	columns = append(columns, "COUNT(*) as totalJobs")

	if need("totalUsers") && col != "job.hpc_user" {
		columns = append(columns, "COUNT(DISTINCT job.hpc_user) AS totalUsers")
	} else {
		columns = append(columns, "0 AS totalUsers")
	}

	if need("totalWalltime") {
		columns = append(columns, fmt.Sprintf(`CAST(ROUND(SUM(%s) / 3600) as int) as totalWalltime`, durationExpr))
	} else {
		columns = append(columns, "0 as totalWalltime")
	}

	if need("totalNodes") {
		columns = append(columns, `CAST(SUM(job.num_nodes) as int) as totalNodes`)
	} else {
		columns = append(columns, "0 as totalNodes")
	}

	if need("totalNodeHours") {
		columns = append(columns, fmt.Sprintf(`CAST(ROUND(SUM(%s * job.num_nodes) / 3600) as int) as totalNodeHours`, durationExpr))
	} else {
		columns = append(columns, "0 as totalNodeHours")
	}

	if need("totalCores") {
		columns = append(columns, `CAST(SUM(job.num_hwthreads) as int) as totalCores`)
	} else {
		columns = append(columns, "0 as totalCores")
	}

	if need("totalCoreHours") {
		columns = append(columns, fmt.Sprintf(`CAST(ROUND(SUM(%s * job.num_hwthreads) / 3600) as int) as totalCoreHours`, durationExpr))
	} else {
		columns = append(columns, "0 as totalCoreHours")
	}

	if need("totalAccs") {
		columns = append(columns, `CAST(SUM(job.num_acc) as int) as totalAccs`)
	} else {
		columns = append(columns, "0 as totalAccs")
	}

	if need("totalAccHours") {
		columns = append(columns, fmt.Sprintf(`CAST(ROUND(SUM(%s * job.num_acc) / 3600) as int) as totalAccHours`, durationExpr))
	} else {
		columns = append(columns, "0 as totalAccHours")
	}

	if need("runningJobs") {
		columns = append(columns, `COUNT(CASE WHEN job.job_state = 'running' THEN 1 END) as runningJobs`)
	} else {
		columns = append(columns, "0 as runningJobs")
	}

	if need("shortJobs") {
		columns = append(columns, fmt.Sprintf(`COUNT(CASE WHEN job.duration < %d THEN 1 END) as shortJobs`, shortThreshold))
	} else {
		columns = append(columns, "0 as shortJobs")
	}

	query := sq.Select(columns...).From("job")
	if col != "" {
		query = query.GroupBy(col)
	}

	for _, f := range filter {
		query = BuildWhereClause(f, query)
	}

	return query
}

// JobsStatsGrouped computes job statistics grouped by a dimension (user, project, cluster, or subcluster).
// Only columns listed in reqFields are computed; others return 0. User display names are looked up
// in a separate lightweight query to avoid JOIN overhead on the main aggregation.
func (r *JobRepository) JobsStatsGrouped(
	ctx context.Context,
	filter []*model.JobFilter,
	page *model.PageRequest,
	sortBy *model.SortByAggregate,
	groupBy *model.Aggregate,
	reqFields map[string]bool,
) ([]*model.JobsStatistics, error) {
	col := groupBy2column[*groupBy]
	query := r.buildStatsQuery(filter, col, config.Keys.ShortRunningJobsDuration, reqFields)

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

	rows, err := query.RunWith(r.DB).QueryContext(ctx)
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}
	defer rows.Close()

	stats := make([]*model.JobsStatistics, 0, 100)

	for rows.Next() {
		var id sql.NullString
		var jobs, users, walltime, nodes, nodeHours, cores, coreHours, accs, accHours, runningJobs, shortJobs sql.NullInt64
		if err := rows.Scan(&id, &jobs, &users, &walltime, &nodes, &nodeHours, &cores, &coreHours, &accs, &accHours, &runningJobs, &shortJobs); err != nil {
			cclog.Warnf("Error while scanning rows: %s", err.Error())
			return nil, err
		}

		if id.Valid {
			stats = append(stats,
				&model.JobsStatistics{
					ID:             id.String,
					TotalJobs:      int(jobs.Int64),
					TotalUsers:     int(users.Int64),
					TotalWalltime:  int(walltime.Int64),
					TotalNodes:     int(nodes.Int64),
					TotalNodeHours: int(nodeHours.Int64),
					TotalCores:     int(cores.Int64),
					TotalCoreHours: int(coreHours.Int64),
					TotalAccs:      int(accs.Int64),
					TotalAccHours:  int(accHours.Int64),
					RunningJobs:    int(runningJobs.Int64),
					ShortJobs:      int(shortJobs.Int64),
				})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Post-query name lookup for user grouping (avoids LEFT JOIN on aggregation query)
	if col == "job.hpc_user" && len(stats) > 0 {
		usernames := make([]any, len(stats))
		for i, s := range stats {
			usernames[i] = s.ID
		}

		nameQuery := sq.Select("username", "name").From("hpc_user").Where(sq.Eq{"username": usernames})
		nameRows, err := nameQuery.RunWith(r.DB).QueryContext(ctx)
		if err != nil {
			cclog.Warnf("Error looking up user names: %s", err.Error())
			// Non-fatal: stats are still valid without display names
		} else {
			defer nameRows.Close()
			nameMap := make(map[string]string, len(stats))
			for nameRows.Next() {
				var username, name string
				if err := nameRows.Scan(&username, &name); err == nil {
					nameMap[username] = name
				}
			}
			for _, s := range stats {
				if name, ok := nameMap[s.ID]; ok {
					s.Name = name
				}
			}
		}
	}

	return stats, nil
}

// JobsStats computes overall job statistics across all matching jobs without grouping.
//
// This method provides a single aggregate view of job metrics, useful for dashboard
// summaries and overall system utilization reports.
//
// Parameters:
//   - ctx: Context for security checks and cancellation
//   - filter: Filters to apply (time range, cluster, job state, etc.)
//
// Returns a single-element slice containing aggregate statistics:
//   - totalJobs, totalUsers, totalWalltime
//   - totalNodeHours, totalCoreHours, totalAccHours
//
// Unlike JobsStatsGrouped, this returns overall totals without breaking down by dimension.
// Security checks are applied via SecurityCheck to respect user access levels.
func (r *JobRepository) JobsStats(
	ctx context.Context,
	filter []*model.JobFilter,
	reqFields map[string]bool,
) ([]*model.JobsStatistics, error) {
	start := time.Now()
	query := r.buildStatsQuery(filter, "", config.Keys.ShortRunningJobsDuration, reqFields)
	query, err := SecurityCheck(ctx, query)
	if err != nil {
		return nil, err
	}

	row := query.RunWith(r.DB).QueryRowContext(ctx)
	stats := make([]*model.JobsStatistics, 0, 1)

	var jobs, users, walltime, nodes, nodeHours, cores, coreHours, accs, accHours, runningJobs, shortJobs sql.NullInt64
	if err := row.Scan(&jobs, &users, &walltime, &nodes, &nodeHours, &cores, &coreHours, &accs, &accHours, &runningJobs, &shortJobs); err != nil {
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
				TotalUsers:     int(users.Int64),
				TotalWalltime:  int(walltime.Int64),
				TotalNodeHours: totalNodeHours,
				TotalCoreHours: totalCoreHours,
				TotalAccHours:  totalAccHours,
				RunningJobs:    int(runningJobs.Int64),
				ShortJobs:      int(shortJobs.Int64),
			})
	}

	cclog.Debugf("Timer JobStats %s", time.Since(start))
	return stats, nil
}

// LoadJobStat retrieves a specific statistic for a metric from a job's statistics.
// Returns 0.0 if the metric is not found or statType is invalid.
//
// Parameters:
//   - job: Job struct with populated Statistics field
//   - metric: Name of the metric to query (e.g., "cpu_load", "mem_used")
//   - statType: Type of statistic: "avg", "min", or "max"
//
// Returns the requested statistic value or 0.0 if not found.
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

// JobCountGrouped counts jobs grouped by a dimension without computing detailed statistics.
//
// This is a lightweight alternative to JobsStatsGrouped when only job counts are needed,
// avoiding the overhead of calculating walltime and resource usage metrics.
//
// Parameters:
//   - ctx: Context for security checks
//   - filter: Filters to apply
//   - groupBy: Grouping dimension (User, Project, Cluster, or SubCluster)
//
// Returns JobsStatistics with only ID and TotalJobs populated for each group.
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
	rows, err := query.RunWith(r.DB).QueryContext(ctx)
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}
	defer rows.Close()

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

	if err := rows.Err(); err != nil {
		return nil, err
	}

	cclog.Debugf("Timer JobCountGrouped %s", time.Since(start))
	return stats, nil
}

// AddJobCountGrouped augments existing statistics with additional job counts by category.
//
// This method enriches JobsStatistics returned by JobsStatsGrouped or JobCountGrouped
// with counts of running or short-running (based on ShortRunningJobsDuration) jobs, matched by group ID.
//
// Parameters:
//   - ctx: Context for security checks
//   - filter: Filters to apply
//   - groupBy: Grouping dimension (must match the dimension used for stats parameter)
//   - stats: Existing statistics to augment (modified in-place by ID matching)
//   - kind: "running" to add RunningJobs count, "short" to add ShortJobs count
//
// Returns the same stats slice with RunningJobs or ShortJobs fields populated per group.
// Groups without matching jobs will have 0 for the added field.
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
	rows, err := query.RunWith(r.DB).QueryContext(ctx)
	if err != nil {
		cclog.Warn("Error while querying DB for job statistics")
		return nil, err
	}
	defer rows.Close()

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

	if err := rows.Err(); err != nil {
		return nil, err
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

// AddJobCount augments existing overall statistics with additional job counts by category.
//
// Similar to AddJobCountGrouped but for ungrouped statistics. Applies the same count
// to all statistics entries (typically just one).
//
// Parameters:
//   - ctx: Context for security checks
//   - filter: Filters to apply
//   - stats: Existing statistics to augment (modified in-place)
//   - kind: "running" to add RunningJobs count, "short" to add ShortJobs count
//
// Returns the same stats slice with RunningJobs or ShortJobs fields set to the total count.
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
	var cnt sql.NullInt64
	if err := query.RunWith(r.DB).QueryRowContext(ctx).Scan(&cnt); err != nil {
		cclog.Warn("Error while querying DB for job count")
		return nil, err
	}

	count := int(cnt.Int64)

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

// AddHistograms augments statistics with distribution histograms for job properties.
//
// Generates histogram data for visualization of job duration, node count, core count,
// and accelerator count distributions. Duration histogram uses intelligent binning based
// on the requested resolution.
//
// Parameters:
//   - ctx: Context for security checks
//   - filter: Filters to apply to jobs included in histograms
//   - stat: Statistics struct to augment (modified in-place)
//   - durationBins: Bin size - "1m", "10m", "1h", "6h", "12h", or "24h" (default)
//
// Populates these fields in stat:
//   - HistDuration: Job duration distribution (zero-padded bins)
//   - HistNumNodes: Node count distribution
//   - HistNumCores: Core (hwthread) count distribution
//   - HistNumAccs: Accelerator count distribution
//
// Duration bins are pre-initialized with zeros to ensure consistent ranges for visualization.
// Bin size determines both the width and maximum duration displayed (e.g., "1h" = 48 bins × 1h = 48h max).
func (r *JobRepository) AddHistograms(
	ctx context.Context,
	filter []*model.JobFilter,
	stat *model.JobsStatistics,
	durationBins *string,
) (*model.JobsStatistics, error) {
	start := time.Now()

	var targetBinCount int
	var targetBinSize int
	switch *durationBins {
	case "1m": // 1 Minute Bins + Max 60 Bins -> Max 60 Minutes
		targetBinCount = 60
		targetBinSize = 60
	case "10m": // 10 Minute Bins + Max 72 Bins -> Max 12 Hours
		targetBinCount = 72
		targetBinSize = 600
	case "1h": // 1 Hour Bins + Max 48 Bins -> Max 48 Hours
		targetBinCount = 48
		targetBinSize = 3600
	case "6h": // 6 Hour Bins + Max 12 Bins -> Max 3 Days
		targetBinCount = 12
		targetBinSize = 21600
	case "12h": // 12 hour Bins + Max 14 Bins -> Max 7 Days
		targetBinCount = 14
		targetBinSize = 43200
	default: // 24h
		targetBinCount = 24
		targetBinSize = 3600
	}

	var err error
	// Return X-Values always as seconds, will be formatted into minutes and hours in frontend
	value := fmt.Sprintf(`CAST(ROUND(((CASE WHEN job.job_state = 'running' THEN %d - job.start_time ELSE job.duration END) / %d) + 1) as int) as value`, time.Now().Unix(), targetBinSize)
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

// AddMetricHistograms augments statistics with distribution histograms for job metrics.
//
// Generates histogram data for metrics like CPU load, memory usage, etc. Handles running
// and completed jobs differently: running jobs load data from metric backend, completed jobs
// use footprint data from database.
//
// Parameters:
//   - ctx: Context for security checks
//   - filter: Filters to apply (MUST contain State filter for running jobs)
//   - metrics: List of metric names to histogram (e.g., ["cpu_load", "mem_used"])
//   - stat: Statistics struct to augment (modified in-place)
//   - targetBinCount: Number of histogram bins (default: 10)
//
// Populates HistMetrics field in stat with MetricHistoPoints for each metric.
//
// Binning algorithm:
//   - Values normalized to metric's peak value from cluster configuration
//   - Bins evenly distributed from 0 to peak
//   - Pre-initialized with zeros for consistent visualization
//
// Limitations:
//   - Running jobs: Limited to 5000 jobs for performance
//   - Requires valid cluster configuration with metric peak values
//   - Uses footprint statistic (avg/max/min) configured per metric
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

// jobsStatisticsHistogram generates a simple histogram by grouping on a column value.
//
// Used for histograms where the column value directly represents the bin (e.g., node count, core count).
// Unlike duration/metric histograms, this doesn't pre-initialize bins with zeros.
//
// Parameters:
//   - value: SQL expression that produces the histogram value, aliased as "value"
//   - filters: Job filters to apply
//
// Returns histogram points with Value (from column) and Count (number of jobs).
func (r *JobRepository) jobsStatisticsHistogram(
	ctx context.Context,
	value string,
	filters []*model.JobFilter,
) ([]*model.HistoPoint, error) {
	start := time.Now()
	query, qerr := SecurityCheck(ctx,
		sq.Select(value, "COUNT(*) AS count").From("job"))

	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	rows, err := query.GroupBy("value").RunWith(r.DB).QueryContext(ctx)
	if err != nil {
		cclog.Error("Error while running query")
		return nil, err
	}
	defer rows.Close()

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

	if err := rows.Err(); err != nil {
		return nil, err
	}

	cclog.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return points, nil
}

// jobsDurationStatisticsHistogram generates a duration histogram with pre-initialized bins.
//
// Bins are zero-padded to provide consistent ranges for visualization, unlike simple
// histograms which only return bins with data. The value parameter should compute
// the bin number from job duration.
//
// Parameters:
//   - value: SQL expression computing bin number from duration, aliased as "value"
//   - filters: Job filters to apply
//   - binSizeSeconds: Width of each bin in seconds
//   - targetBinCount: Number of bins to pre-initialize
//
// Returns histogram points with Value (bin_number × binSizeSeconds) and Count.
// All bins from 1 to targetBinCount are returned, with Count=0 for empty bins.
//
// Algorithm:
//  1. Pre-initialize targetBinCount bins with zero counts
//  2. Query database for actual counts per bin
//  3. Match query results to pre-initialized bins by value
//  4. Bins without matches remain at zero
func (r *JobRepository) jobsDurationStatisticsHistogram(
	ctx context.Context,
	value string,
	filters []*model.JobFilter,
	binSizeSeconds int,
	targetBinCount *int,
) ([]*model.HistoPoint, error) {
	start := time.Now()
	query, qerr := SecurityCheck(ctx,
		sq.Select(value, "COUNT(*) AS count").From("job"))

	if qerr != nil {
		return nil, qerr
	}

	// Each bin represents a duration range: bin N = [N*binSizeSeconds, (N+1)*binSizeSeconds)
	// Example: binSizeSeconds=3600 (1 hour), bin 1 = 0-1h, bin 2 = 1-2h, etc.
	points := make([]*model.HistoPoint, 0, *targetBinCount)
	for i := 1; i <= *targetBinCount; i++ {
		point := model.HistoPoint{Value: i * binSizeSeconds, Count: 0}
		points = append(points, &point)
	}

	// Build a map from bin value (seconds) to slice index for O(1) lookup.
	binMap := make(map[int]int, len(points))
	for i, p := range points {
		binMap[p.Value] = i
	}

	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	rows, err := query.GroupBy("value").RunWith(r.DB).QueryContext(ctx)
	if err != nil {
		cclog.Error("Error while running query")
		return nil, err
	}
	defer rows.Close()

	// Match query results to pre-initialized bins using map lookup.
	// point.Value from query is the bin number; multiply by binSizeSeconds to get bin key.
	for rows.Next() {
		point := model.HistoPoint{}
		if err := rows.Scan(&point.Value, &point.Count); err != nil {
			cclog.Warn("Error while scanning rows")
			return nil, err
		}

		if idx, ok := binMap[point.Value*binSizeSeconds]; ok {
			points[idx].Count = point.Count
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	cclog.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return points, nil
}

// jobsMetricStatisticsHistogram generates a metric histogram using footprint data from completed jobs.
//
// Values are normalized to the metric's peak value and distributed into bins. The algorithm
// is based on SQL histogram generation techniques, extracting metric values from JSON footprint
// and computing bin assignments in SQL.
//
// Parameters:
//   - metric: Metric name (e.g., "cpu_load", "mem_used")
//   - filters: Job filters to apply
//   - bins: Number of bins to generate
//
// Returns MetricHistoPoints with metric name, unit, footprint stat type, and binned data.
//
// Algorithm:
//  1. Determine peak value from cluster configuration (filtered cluster or max across all)
//  2. Generate SQL that extracts footprint value, normalizes to [0,1], multiplies by bin count
//  3. Pre-initialize bins with min/max ranges based on peak value
//  4. Query database for counts per bin
//  5. Match results to pre-initialized bins
//
// Special handling: Values exactly equal to peak are forced into the last bin by multiplying
// peak by 0.999999999 to avoid creating an extra bin.
func (r *JobRepository) jobsMetricStatisticsHistogram(
	ctx context.Context,
	metric string,
	filters []*model.JobFilter,
	bins *int,
) (*model.MetricHistoPoints, error) {
	// Peak value defines the upper bound for binning: values are distributed across
	// bins from 0 to peak. First try to get peak from filtered cluster, otherwise
	// scan all clusters to find the maximum peak value.
	var metricConfig *schema.MetricConfig
	var peak float64
	var unit string
	var footprintStat string

	// Try to get metric config from filtered cluster
	for _, f := range filters {
		if f.Cluster != nil {
			metricConfig = archive.GetMetricConfig(*f.Cluster.Eq, metric)
			peak = metricConfig.Peak
			unit = metricConfig.Unit.Prefix + metricConfig.Unit.Base
			footprintStat = metricConfig.Footprint
			cclog.Debugf("Cluster %s filter found with peak %f for %s", *f.Cluster.Eq, peak, metric)
		}
	}

	// If no cluster filter or peak not found, find largest peak across all clusters
	// This ensures histogram can accommodate all possible values
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

	// Construct SQL histogram bins using normalized values.
	// Algorithm based on: https://jereze.com/code/sql-histogram/ (modified)
	start := time.Now()

	// Bin calculation formula:
	//   bin_number = CAST( (value / peak) * num_bins AS INTEGER ) + 1
	// Special case: value == peak would create bin N+1, so we test for equality
	// and multiply peak by 0.999999999 to force it into bin N.
	binQuery := fmt.Sprintf(`CAST(
		((case when json_extract(footprint, "$.%s") = %f then %f*0.999999999 else json_extract(footprint, "$.%s") end) / %f)
		* %v as INTEGER )`,
		(metric + "_" + footprintStat), peak, peak, (metric + "_" + footprintStat), peak, *bins)

	mainQuery := sq.Select(
		fmt.Sprintf(`%s + 1 as bin`, binQuery),
		`count(*) as count`,
	).From("job").Where(
		"JSON_VALID(footprint)",
	).Where(fmt.Sprintf(`json_extract(footprint, "$.%s") is not null and json_extract(footprint, "$.%s") <= %f`, (metric + "_" + footprintStat), (metric + "_" + footprintStat), peak))

	mainQuery, qerr := SecurityCheck(ctx, mainQuery)
	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		mainQuery = BuildWhereClause(f, mainQuery)
	}

	mainQuery = mainQuery.GroupBy("bin").OrderBy("bin")

	rows, err := mainQuery.RunWith(r.DB).QueryContext(ctx)
	if err != nil {
		cclog.Errorf("Error while running mainQuery: %s", err)
		return nil, err
	}
	defer rows.Close()

	// Pre-initialize bins with calculated min/max ranges.
	// Example: peak=1000, bins=10 -> bin 1=[0,100), bin 2=[100,200), ..., bin 10=[900,1000]
	points := make([]*model.MetricHistoPoint, 0, *bins)
	binStep := int(peak) / *bins
	for i := 1; i <= *bins; i++ {
		binMin := (binStep * (i - 1))
		binMax := (binStep * i)
		idx := i
		epoint := model.MetricHistoPoint{Bin: &idx, Count: 0, Min: &binMin, Max: &binMax}
		points = append(points, &epoint)
	}

	// Build a map from bin number to slice index for O(1) lookup.
	binMap := make(map[int]int, len(points))
	for i, p := range points {
		if p.Bin != nil {
			binMap[*p.Bin] = i
		}
	}

	// Match query results to pre-initialized bins using map lookup.
	for rows.Next() {
		rpoint := model.MetricHistoPoint{}
		if err := rows.Scan(&rpoint.Bin, &rpoint.Count); err != nil {
			cclog.Warnf("Error while scanning rows for %s", metric)
			return nil, err
		}

		if rpoint.Bin != nil {
			if idx, ok := binMap[*rpoint.Bin]; ok {
				points[idx].Count = rpoint.Count
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := model.MetricHistoPoints{Metric: metric, Unit: unit, Stat: &footprintStat, Data: points}

	cclog.Debugf("Timer jobsStatisticsHistogram %s", time.Since(start))
	return &result, nil
}

// runningJobsMetricStatisticsHistogram generates metric histograms for running jobs using live data.
//
// Unlike completed jobs which use footprint data from the database, running jobs require
// fetching current metric averages from the metric backend (via metricdispatch).
//
// Parameters:
//   - metrics: List of metric names
//   - filters: Job filters (should filter to running jobs only)
//   - bins: Number of histogram bins
//
// Returns slice of MetricHistoPoints, one per metric.
//
// Limitations:
//   - Maximum 5000 jobs (returns nil if more jobs match)
//   - Requires metric backend availability
//   - Bins based on metric peak values from cluster configuration
//
// Algorithm:
//  1. Query first 5001 jobs to check count limit
//  2. Load metric averages for all jobs via metricdispatch
//  3. For each metric, create bins based on peak value
//  4. Iterate averages and count jobs per bin
func (r *JobRepository) runningJobsMetricStatisticsHistogram(
	ctx context.Context,
	metrics []string,
	filters []*model.JobFilter,
	bins *int,
) []*model.MetricHistoPoints {
	// Get Jobs
	jobs, err := r.QueryJobs(ctx, filters, &model.PageRequest{Page: 1, ItemsPerPage: 5000 + 1}, nil)
	if err != nil {
		cclog.Errorf("Error while querying jobs for footprint: %s", err)
		return nil
	}
	if len(jobs) > 5000 {
		cclog.Errorf("too many jobs matched (max: %d)", 5000)
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

		if err := metricdispatch.LoadAverages(job, metrics, avgs, ctx); err != nil {
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
