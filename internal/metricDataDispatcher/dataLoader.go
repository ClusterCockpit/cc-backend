// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricDataDispatcher

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/lrucache"
	"github.com/ClusterCockpit/cc-lib/resampler"
	"github.com/ClusterCockpit/cc-lib/schema"
)

var cache *lrucache.Cache = lrucache.New(128 * 1024 * 1024)

func cacheKey(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
) string {
	// Duration and StartTime do not need to be in the cache key as StartTime is less unique than
	// job.ID and the TTL of the cache entry makes sure it does not stay there forever.
	return fmt.Sprintf("%d(%s):[%v],[%v]-%d",
		job.ID, job.State, metrics, scopes, resolution)
}

// Fetches the metric data for a job.
func LoadData(job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int,
) (schema.JobData, error) {
	data := cache.Get(cacheKey(job, metrics, scopes, resolution), func() (_ any, ttl time.Duration, size int) {
		var jd schema.JobData
		var err error

		if job.State == schema.JobStateRunning ||
			job.MonitoringStatus == schema.MonitoringStatusRunningOrArchiving ||
			config.Keys.DisableArchive {

			repo, err := metricdata.GetMetricDataRepo(job.Cluster)
			if err != nil {
				return fmt.Errorf("METRICDATA/METRICDATA > no metric data repository configured for '%s'", job.Cluster), 0, 0
			}

			if scopes == nil {
				scopes = append(scopes, schema.MetricScopeNode)
			}

			if metrics == nil {
				cluster := archive.GetCluster(job.Cluster)
				for _, mc := range cluster.MetricConfig {
					metrics = append(metrics, mc.Name)
				}
			}

			jd, err = repo.LoadData(job, metrics, scopes, ctx, resolution)
			if err != nil {
				if len(jd) != 0 {
					cclog.Warnf("partial error: %s", err.Error())
					// return err, 0, 0 // Reactivating will block archiving on one partial error
				} else {
					cclog.Error("Error while loading job data from metric repository")
					return err, 0, 0
				}
			}
			size = jd.Size()
		} else {
			var jd_temp schema.JobData
			jd_temp, err = archive.GetHandle().LoadJobData(job)
			if err != nil {
				cclog.Error("Error while loading job data from archive")
				return err, 0, 0
			}

			// Deep copy the cached archive hashmap
			jd = metricdata.DeepCopy(jd_temp)

			// Resampling for archived data.
			// Pass the resolution from frontend here.
			for _, v := range jd {
				for _, v_ := range v {
					timestep := int64(0)
					for i := 0; i < len(v_.Series); i += 1 {
						v_.Series[i].Data, timestep, err = resampler.LargestTriangleThreeBucket(v_.Series[i].Data, int64(v_.Timestep), int64(resolution))
						if err != nil {
							return err, 0, 0
						}
					}
					v_.Timestep = int(timestep)
				}
			}

			// Avoid sending unrequested data to the client:
			if metrics != nil || scopes != nil {
				if metrics == nil {
					metrics = make([]string, 0, len(jd))
					for k := range jd {
						metrics = append(metrics, k)
					}
				}

				res := schema.JobData{}
				for _, metric := range metrics {
					if perscope, ok := jd[metric]; ok {
						if len(perscope) > 1 {
							subset := make(map[schema.MetricScope]*schema.JobMetric)
							for _, scope := range scopes {
								if jm, ok := perscope[scope]; ok {
									subset[scope] = jm
								}
							}

							if len(subset) > 0 {
								perscope = subset
							}
						}

						res[metric] = perscope
					}
				}
				jd = res
			}
			size = jd.Size()
		}

		ttl = 5 * time.Hour
		if job.State == schema.JobStateRunning {
			ttl = 2 * time.Minute
		}

		// FIXME: Review: Is this really necessary or correct.
		// Note: Lines 147-170 formerly known as prepareJobData(jobData, scopes)
		// For /monitoring/job/<job> and some other places, flops_any and mem_bw need
		// to be available at the scope 'node'. If a job has a lot of nodes,
		// statisticsSeries should be available so that a min/median/max Graph can be
		// used instead of a lot of single lines.
		// NOTE: New StatsSeries will always be calculated as 'min/median/max'
		//       Existing (archived) StatsSeries can be 'min/mean/max'!
		const maxSeriesSize int = 15
		for _, scopes := range jd {
			for _, jm := range scopes {
				if jm.StatisticsSeries != nil || len(jm.Series) <= maxSeriesSize {
					continue
				}

				jm.AddStatisticsSeries()
			}
		}

		nodeScopeRequested := false
		for _, scope := range scopes {
			if scope == schema.MetricScopeNode {
				nodeScopeRequested = true
			}
		}

		if nodeScopeRequested {
			jd.AddNodeScope("flops_any")
			jd.AddNodeScope("mem_bw")
		}

		// Round Resulting Stat Values
		jd.RoundMetricStats()

		return jd, ttl, size
	})

	if err, ok := data.(error); ok {
		cclog.Error("Error in returned dataset")
		return nil, err
	}

	return data.(schema.JobData), nil
}

// Used for the jobsFootprint GraphQL-Query. TODO: Rename/Generalize.
func LoadAverages(
	job *schema.Job,
	metrics []string,
	data [][]schema.Float,
	ctx context.Context,
) error {
	if job.State != schema.JobStateRunning && !config.Keys.DisableArchive {
		return archive.LoadAveragesFromArchive(job, metrics, data) // #166 change also here?
	}

	repo, err := metricdata.GetMetricDataRepo(job.Cluster)
	if err != nil {
		return fmt.Errorf("METRICDATA/METRICDATA > no metric data repository configured for '%s'", job.Cluster)
	}

	stats, err := repo.LoadStats(job, metrics, ctx) // #166 how to handle stats for acc normalizazion?
	if err != nil {
		cclog.Errorf("Error while loading statistics for job %v (User %v, Project %v)", job.JobID, job.User, job.Project)
		return err
	}

	for i, m := range metrics {
		nodes, ok := stats[m]
		if !ok {
			data[i] = append(data[i], schema.NaN)
			continue
		}

		sum := 0.0
		for _, node := range nodes {
			sum += node.Avg
		}
		data[i] = append(data[i], schema.Float(sum))
	}

	return nil
}

// Used for statsTable in frontend: Return scoped statistics by metric.
func LoadScopedJobStats(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
) (schema.ScopedJobStats, error) {
	if job.State != schema.JobStateRunning && !config.Keys.DisableArchive {
		return archive.LoadScopedStatsFromArchive(job, metrics, scopes)
	}

	repo, err := metricdata.GetMetricDataRepo(job.Cluster)
	if err != nil {
		return nil, fmt.Errorf("job %d: no metric data repository configured for '%s'", job.JobID, job.Cluster)
	}

	scopedStats, err := repo.LoadScopedStats(job, metrics, scopes, ctx)
	if err != nil {
		cclog.Errorf("error while loading scoped statistics for job %d (User %s, Project %s)", job.JobID, job.User, job.Project)
		return nil, err
	}

	return scopedStats, nil
}

// Used for polar plots in frontend: Aggregates statistics for all nodes to single values for job per metric.
func LoadJobStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context,
) (map[string]schema.MetricStatistics, error) {
	if job.State != schema.JobStateRunning && !config.Keys.DisableArchive {
		return archive.LoadStatsFromArchive(job, metrics)
	}

	data := make(map[string]schema.MetricStatistics, len(metrics))
	repo, err := metricdata.GetMetricDataRepo(job.Cluster)
	if err != nil {
		return data, fmt.Errorf("job %d: no metric data repository configured for '%s'", job.JobID, job.Cluster)
	}

	stats, err := repo.LoadStats(job, metrics, ctx)
	if err != nil {
		cclog.Errorf("error while loading statistics for job %d (User %s, Project %s)", job.JobID, job.User, job.Project)
		return data, err
	}

	for _, m := range metrics {
		sum, avg, min, max := 0.0, 0.0, 0.0, 0.0
		nodes, ok := stats[m]
		if !ok {
			data[m] = schema.MetricStatistics{Min: min, Avg: avg, Max: max}
			continue
		}

		for _, node := range nodes {
			sum += node.Avg
			min = math.Min(min, node.Min)
			max = math.Max(max, node.Max)
		}

		data[m] = schema.MetricStatistics{
			Avg: (math.Round((sum/float64(job.NumNodes))*100) / 100),
			Min: (math.Round(min*100) / 100),
			Max: (math.Round(max*100) / 100),
		}
	}

	return data, nil
}

// Used for the classic node/system view. Returns a map of nodes to a map of metrics.
func LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context,
) (map[string]map[string][]*schema.JobMetric, error) {
	repo, err := metricdata.GetMetricDataRepo(cluster)
	if err != nil {
		return nil, fmt.Errorf("METRICDATA/METRICDATA > no metric data repository configured for '%s'", cluster)
	}

	if metrics == nil {
		for _, m := range archive.GetCluster(cluster).MetricConfig {
			metrics = append(metrics, m.Name)
		}
	}

	data, err := repo.LoadNodeData(cluster, metrics, nodes, scopes, from, to, ctx)
	if err != nil {
		if len(data) != 0 {
			cclog.Warnf("partial error: %s", err.Error())
		} else {
			cclog.Error("Error while loading node data from metric repository")
			return nil, err
		}
	}

	if data == nil {
		return nil, fmt.Errorf("METRICDATA/METRICDATA > the metric data repository for '%s' does not support this query", cluster)
	}

	return data, nil
}

func LoadNodeListData(
	cluster, subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	ctx context.Context,
) (map[string]schema.JobData, error) {
	repo, err := metricdata.GetMetricDataRepo(cluster)
	if err != nil {
		return nil, fmt.Errorf("METRICDATA/METRICDATA > no metric data repository configured for '%s'", cluster)
	}

	if metrics == nil {
		for _, m := range archive.GetCluster(cluster).MetricConfig {
			metrics = append(metrics, m.Name)
		}
	}

	data, err := repo.LoadNodeListData(cluster, subCluster, nodes, metrics, scopes, resolution, from, to, ctx)
	if err != nil {
		if len(data) != 0 {
			cclog.Warnf("partial error: %s", err.Error())
		} else {
			cclog.Error("Error while loading node data from metric repository")
			return nil, err
		}
	}

	// NOTE: New StatsSeries will always be calculated as 'min/median/max'
	const maxSeriesSize int = 8
	for _, jd := range data {
		for _, scopes := range jd {
			for _, jm := range scopes {
				if jm.StatisticsSeries != nil || len(jm.Series) < maxSeriesSize {
					continue
				}
				jm.AddStatisticsSeries()
			}
		}
	}

	if data == nil {
		return nil, fmt.Errorf("METRICDATA/METRICDATA > the metric data repository for '%s' does not support this query", cluster)
	}

	return data, nil
}
