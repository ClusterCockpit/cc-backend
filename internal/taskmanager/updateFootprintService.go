// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"context"
	"math"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/go-co-op/gocron/v2"
)

func RegisterFootprintWorker() {
	var frequency string
	if Keys.FootprintWorker != "" {
		frequency = Keys.FootprintWorker
	} else {
		frequency = "10m"
	}

	d, err := parseDuration(frequency)
	if err != nil {
		cclog.Errorf("RegisterFootprintWorker: %v", err)
		return
	}

	cclog.Infof("Register Footprint Update service with %s interval", frequency)

	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				s := time.Now()
				c := 0
				ce := 0
				cl := 0
				cclog.Infof("Update Footprints started at %s", s.Format(time.RFC3339))

				for _, cluster := range archive.Clusters {
					sCluster := time.Now()
					jobs, err := jobRepo.FindRunningJobs(cluster.Name)
					if err != nil {
						continue
					}
					// NOTE: Additional Subcluster Loop Could Allow For Limited List Of Footprint-Metrics Only.
					//       - Chunk-Size Would Then Be 'SubCluster' (Running Jobs, Transactions) as Lists Can Change Within SCs
					//       - Would Require Review of 'updateFootprint' Usage (Logic Could Possibly Be Included Here Completely)
					allMetrics := make([]string, 0)
					metricConfigs := archive.GetCluster(cluster.Name).MetricConfig
					for _, mc := range metricConfigs {
						allMetrics = append(allMetrics, mc.Name)
					}

					pendingStatements := []sq.UpdateBuilder{}

					for _, job := range jobs {
						cclog.Debugf("Prepare job %d", job.JobID)
						cl++

						sJob := time.Now()

						ms, err := metricdispatch.GetMetricDataRepo(job.Cluster, job.SubCluster)
						if err != nil {
							cclog.Errorf("failed to access metricDataRepo for cluster %s-%s: %s",
								job.Cluster, job.SubCluster, err.Error())
							continue
						}

						jobStats, err := ms.LoadStats(job, allMetrics, context.Background())
						if err != nil {
							cclog.Errorf("error wile loading job data stats for footprint update: %v", err)
							ce++
							continue
						}

						job.Statistics = make(map[string]schema.JobStatistics)

						for _, metric := range allMetrics {
							avg, min, max := 0.0, 0.0, 0.0
							data, ok := jobStats[metric] // JobStats[Metric1:[Hostname1:[Stats], Hostname2:[Stats], ...], Metric2[...] ...]
							if ok {
								for _, res := range job.Resources {
									hostStats, ok := data[res.Hostname]
									if ok {
										avg += hostStats.Avg
										min = math.Min(min, hostStats.Min)
										max = math.Max(max, hostStats.Max)
									}

								}
							}

							// Add values rounded to 2 digits: repo.LoadStats may return unrounded
							job.Statistics[metric] = schema.JobStatistics{
								Unit: schema.Unit{
									Prefix: archive.GetMetricConfig(job.Cluster, metric).Unit.Prefix,
									Base:   archive.GetMetricConfig(job.Cluster, metric).Unit.Base,
								},
								Avg: (math.Round((avg/float64(job.NumNodes))*100) / 100),
								Min: (math.Round(min*100) / 100),
								Max: (math.Round(max*100) / 100),
							}
						}

						// Build Statement per Job, Add to Pending Array
						stmt := sq.Update("job")
						stmt, err = jobRepo.UpdateFootprint(stmt, job)
						if err != nil {
							cclog.Errorf("update job (dbid: %d) statement build failed at footprint step: %s", *job.ID, err.Error())
							ce++
							continue
						}
						stmt = stmt.Where("job.id = ?", job.ID)

						pendingStatements = append(pendingStatements, stmt)
						cclog.Debugf("Job %d took %s", job.JobID, time.Since(sJob))
					}

					t, err := jobRepo.TransactionInit()
					if err != nil {
						cclog.Errorf("failed TransactionInit %v", err)
						cclog.Errorf("skipped %d transactions for cluster %s", len(pendingStatements), cluster.Name)
						ce += len(pendingStatements)
					} else {
						for _, ps := range pendingStatements {
							query, args, err := ps.ToSql()
							if err != nil {
								cclog.Errorf("failed in ToSQL conversion: %v", err)
								ce++
							} else {
								// args...: Footprint-JSON, Energyfootprint-JSON, TotalEnergy, JobID
								jobRepo.TransactionAdd(t, query, args...)
								c++
							}
						}
						jobRepo.TransactionEnd(t)
					}
					cclog.Debugf("Finish Cluster %s, took %s\n", cluster.Name, time.Since(sCluster))
				}
				cclog.Infof("Updating %d (of %d; Skipped %d) Footprints is done and took %s", c, cl, ce, time.Since(s))
			}))
}
