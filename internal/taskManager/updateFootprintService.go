// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"context"
	"math"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/metricDataDispatcher"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/go-co-op/gocron/v2"
)

func RegisterFootprintWorker() {
	log.Info("Register Footprint Update service")
	d, _ := time.ParseDuration("10m")
	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				s := time.Now()
				log.Printf("Update Footprints started at %s", s.Format(time.RFC3339))

				t, err := jobRepo.TransactionInit()
				if err != nil {
					log.Errorf("Failed TransactionInit %v", err)
				}

				for _, cluster := range archive.Clusters {
					jobs, err := jobRepo.FindRunningJobs(cluster.Name)
					if err != nil {
						continue
					}
					allMetrics := make([]string, 0)
					metricConfigs := archive.GetCluster(cluster.Name).MetricConfig
					for _, mc := range metricConfigs {
						allMetrics = append(allMetrics, mc.Name)
					}

					scopes := []schema.MetricScope{schema.MetricScopeNode}
					scopes = append(scopes, schema.MetricScopeCore)
					scopes = append(scopes, schema.MetricScopeAccelerator)

					for _, job := range jobs {
						jobData, err := metricDataDispatcher.LoadData(job, allMetrics, scopes, context.Background())
						if err != nil {
							log.Error("Error wile loading job data for footprint update")
							continue
						}

						jobMeta := &schema.JobMeta{
							BaseJob:    job.BaseJob,
							StartTime:  job.StartTime.Unix(),
							Statistics: make(map[string]schema.JobStatistics),
						}

						for metric, data := range jobData {
							avg, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32
							nodeData, ok := data["node"]
							if !ok {
								// This should never happen ?
								continue
							}

							for _, series := range nodeData.Series {
								avg += series.Statistics.Avg
								min = math.Min(min, series.Statistics.Min)
								max = math.Max(max, series.Statistics.Max)
							}

							jobMeta.Statistics[metric] = schema.JobStatistics{
								Unit: schema.Unit{
									Prefix: archive.GetMetricConfig(job.Cluster, metric).Unit.Prefix,
									Base:   archive.GetMetricConfig(job.Cluster, metric).Unit.Base,
								},
								Avg: avg / float64(job.NumNodes),
								Min: min,
								Max: max,
							}
						}

						stmt := sq.Update("job").Where("job.id = ?", job.ID)
						stmt, err = jobRepo.UpdateFootprint(stmt, jobMeta)
						if err != nil {
							log.Errorf("Update job (dbid: %d) failed at update Footprint step: %s", job.ID, err.Error())
							continue
						}

						stmt, err = jobRepo.UpdateEnergy(stmt, jobMeta)
						if err != nil {
							log.Errorf("Update job (dbid: %d) failed at update Energy step: %s", job.ID, err.Error())
							continue
						}

						query, args, err := stmt.ToSql()
						if err != nil {
							log.Errorf("Failed in ToSQL conversion %v", err)
							continue
						}
						jobRepo.TransactionAdd(t, query, args)
						// if err := jobRepo.Execute(stmt); err != nil {
						// 	log.Errorf("Update job (dbid: %d) failed at db execute: %s", job.ID, err.Error())
						// 	continue
						// }
					}

					jobRepo.TransactionCommit(t)
				}
				jobRepo.TransactionEnd(t)
				log.Printf("Update Footprints is done and took %s", time.Since(s))
			}))
}
