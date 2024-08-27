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
			!config.Keys.DisableArchive {

			repo, ok := metricdata.GetMetricDataRepo(job.Cluster)

			if !ok {
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

		prepareJobData(jd, scopes)

		return jd, ttl, size
	})

	if err, ok := data.(error); ok {
		log.Error("Error in returned dataset")
		return nil, err
	}

	return data.(schema.JobData), nil
}
