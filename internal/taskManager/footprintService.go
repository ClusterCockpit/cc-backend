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
	"github.com/go-co-op/gocron/v2"
)

func registerFootprintWorker() {
	log.Info("Register Footprint Update service")
	d, _ := time.ParseDuration("10m")
	s.NewJob(gocron.DurationJob(d),
		gocron.NewTask(
			func() {
				t := time.Now()
				log.Printf("Update Footprints started at %s", t.Format(time.RFC3339))
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

						if err := jobRepo.UpdateFootprint(jobMeta); err != nil {
							log.Errorf("Update job (dbid: %d) failed at update Footprint step: %s", job.ID, err.Error())
							continue
						}
						if err := jobRepo.UpdateEnergy(jobMeta); err != nil {
							log.Errorf("Update job (dbid: %d) failed at update Energy step: %s", job.ID, err.Error())
							continue
						}
					}
				}
				log.Print("Update Footprints done")
			}))
}
