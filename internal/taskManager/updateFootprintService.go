// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"context"
	"math"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/go-co-op/gocron/v2"
)

func RegisterFootprintWorker() {
	var frequency string
	if config.Keys.CronFrequency != nil && config.Keys.CronFrequency.FootprintWorker != "" {
		frequency = config.Keys.CronFrequency.FootprintWorker
	} else {
		frequency = "10m"
	}
	d, _ := time.ParseDuration(frequency)
	log.Infof("Register Footprint Update service with %s interval", frequency)

	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				s := time.Now()
				c := 0
				ce := 0
				cl := 0
				log.Printf("Update Footprints started at %s", s.Format(time.RFC3339))

				for _, cluster := range archive.Clusters {
					s_cluster := time.Now()
					jobs, err := jobRepo.FindRunningJobs(cluster.Name)
					if err != nil {
						continue
					}
					allMetrics := make([]string, 0)
					metricConfigs := archive.GetCluster(cluster.Name).MetricConfig
					for _, mc := range metricConfigs {
						allMetrics = append(allMetrics, mc.Name)
					}

					repo, err := metricdata.GetMetricDataRepo(cluster.Name)
					if err != nil {
						log.Errorf("no metric data repository configured for '%s'", cluster.Name)
						continue
					}

					pendingStatements := []sq.UpdateBuilder{}

					for _, job := range jobs {
						log.Debugf("Prepare job %d", job.JobID)
						cl++

						s_job := time.Now()

						jobStats, err := repo.LoadStats(job, allMetrics, context.Background())
						if err != nil {
							log.Errorf("error wile loading job data stats for footprint update: %v", err)
							ce++
							continue
						}

						jobMeta := &schema.JobMeta{
							BaseJob:    job.BaseJob,
							StartTime:  job.StartTime.Unix(),
							Statistics: make(map[string]schema.JobStatistics),
						}

						for _, metric := range allMetrics {
							avg, min, max := 0.0, 0.0, 0.0
							data, ok := jobStats[metric] // Metric:[Hostname:Stats]
							if ok {
								for _, res := range job.Resources {
									hostStats, ok := data[res.Hostname]
									if ok {
										avg += hostStats.Avg
										min = math.Min(min, hostStats.Min)
										max = math.Max(max, hostStats.Max)
									}
									// else {
									// 	log.Debugf("no stats data return for host %s in job %d, metric %s", res.Hostname, job.JobID, metric)
									// }
								}
							}
							// else {
							// 	log.Debugf("no stats data return for job %d, metric %s", job.JobID, metric)
							// }

							// Add values rounded to 2 digits
							jobMeta.Statistics[metric] = schema.JobStatistics{
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
						stmt, err = jobRepo.UpdateFootprint(stmt, jobMeta)
						if err != nil {
							log.Errorf("update job (dbid: %d) statement build failed at footprint step: %s", job.ID, err.Error())
							ce++
							continue
						}
						stmt, err = jobRepo.UpdateEnergy(stmt, jobMeta)
						if err != nil {
							log.Errorf("update job (dbid: %d) statement build failed at energy step: %s", job.ID, err.Error())
							ce++
							continue
						}
						stmt = stmt.Where("job.id = ?", job.ID)

						pendingStatements = append(pendingStatements, stmt)
						log.Debugf("Job %d took %s", job.JobID, time.Since(s_job))
					}

					t, err := jobRepo.TransactionInit()
					if err != nil {
						log.Errorf("failed TransactionInit %v", err)
					}

					for idx, ps := range pendingStatements {

						query, args, err := ps.ToSql()
						if err != nil {
							log.Errorf("failed in ToSQL conversion: %v", err)
							ce++
						} else {
							// Args: JSON, JSON, ENERGY, JOBID
							log.Debugf("add transaction on index %d", idx)
							jobRepo.TransactionAdd(t, query, args...)
							c++
						}
					}

					jobRepo.TransactionEnd(t)
					log.Debugf("Finish Cluster %s, took %s", cluster.Name, time.Since(s_cluster))
				}
				log.Printf("Updating %d (of %d; Skipped %d) Footprints is done and took %s", c, cl, ce, time.Since(s))
			}))
}
