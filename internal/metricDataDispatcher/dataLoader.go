// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricDataDispatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var cache *lrucache.Cache = lrucache.New(128 * 1024 * 1024)

func cacheKey(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
) string {
	// Duration and StartTime do not need to be in the cache key as StartTime is less unique than
	// job.ID and the TTL of the cache entry makes sure it does not stay there forever.
	return fmt.Sprintf("%d(%s):[%v],[%v]",
		job.ID, job.State, metrics, scopes)
}

// Fetches the metric data for a job.
func LoadData(job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
) (schema.JobData, error) {
	data := cache.Get(cacheKey(job, metrics, scopes), func() (_ interface{}, ttl time.Duration, size int) {
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

			jd, err = repo.LoadData(job, metrics, scopes, ctx)
			if err != nil {
				if len(jd) != 0 {
					log.Warnf("partial error: %s", err.Error())
					// return err, 0, 0 // Reactivating will block archiving on one partial error
				} else {
					log.Error("Error while loading job data from metric repository")
					return err, 0, 0
				}
			}
			size = jd.Size()
		} else {
			jd, err = archive.GetHandle().LoadJobData(job)
			if err != nil {
				log.Error("Error while loading job data from archive")
				return err, 0, 0
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
		// For /monitoring/job/<job> and some other places, flops_any and mem_bw need
		// to be available at the scope 'node'. If a job has a lot of nodes,
		// statisticsSeries should be available so that a min/median/max Graph can be
		// used instead of a lot of single lines.
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

		return jd, ttl, size
	})

	if err, ok := data.(error); ok {
		log.Error("Error in returned dataset")
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
		log.Errorf("Error while loading statistics for job %v (User %v, Project %v)", job.JobID, job.User, job.Project)
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

// Used for the node/system view. Returns a map of nodes to a map of metrics.
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
			log.Warnf("partial error: %s", err.Error())
		} else {
			log.Error("Error while loading node data from metric repository")
			return nil, err
		}
	}

	if data == nil {
		return nil, fmt.Errorf("METRICDATA/METRICDATA > the metric data repository for '%s' does not support this query", cluster)
	}

	return data, nil
}
