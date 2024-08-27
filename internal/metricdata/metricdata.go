// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type MetricDataRepository interface {
	// Initialize this MetricDataRepository. One instance of
	// this interface will only ever be responsible for one cluster.
	Init(rawConfig json.RawMessage) error

	// Return the JobData for the given job, only with the requested metrics.
	LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error)

	// Return a map of metrics to a map of nodes to the metric statistics of the job. node scope assumed for now.
	LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error)

	// Return a map of hosts to a map of metrics at the requested scopes for that node.
	LoadNodeData(cluster string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error)
}

var metricDataRepos map[string]MetricDataRepository = map[string]MetricDataRepository{}

func Init() error {
	for _, cluster := range config.Keys.Clusters {
		if cluster.MetricDataRepository != nil {
			var kind struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(cluster.MetricDataRepository, &kind); err != nil {
				log.Warn("Error while unmarshaling raw json MetricDataRepository")
				return err
			}

			var mdr MetricDataRepository
			switch kind.Kind {
			case "cc-metric-store":
				mdr = &CCMetricStore{}
			case "influxdb":
				mdr = &InfluxDBv2DataRepository{}
			case "prometheus":
				mdr = &PrometheusDataRepository{}
			case "test":
				mdr = &TestMetricDataRepository{}
			default:
				return fmt.Errorf("METRICDATA/METRICDATA > Unknown MetricDataRepository %v for cluster %v", kind.Kind, cluster.Name)
			}

			if err := mdr.Init(cluster.MetricDataRepository); err != nil {
				log.Errorf("Error initializing MetricDataRepository %v for cluster %v", kind.Kind, cluster.Name)
				return err
			}
			metricDataRepos[cluster.Name] = mdr
		}
	}
	return nil
}

func GetMetricDataRepo(cluster string) MetricDataRepository {
	repo, ok := metricDataRepos[cluster]

	if !ok {
		return fmt.Errorf("METRICDATA/METRICDATA > no metric data repository configured for '%s'", job.Cluster), 0, 0
	}

	return repo
}

// Used for the jobsFootprint GraphQL-Query. TODO: Rename/Generalize.
func LoadAverages(
	job *schema.Job,
	metrics []string,
	data [][]schema.Float,
	ctx context.Context,
) error {
	if job.State != schema.JobStateRunning && useArchive {
		return archive.LoadAveragesFromArchive(job, metrics, data) // #166 change also here?
	}

	repo, ok := metricDataRepos[job.Cluster]
	if !ok {
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
	repo, ok := metricDataRepos[cluster]
	if !ok {
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

// For /monitoring/job/<job> and some other places, flops_any and mem_bw need
// to be available at the scope 'node'. If a job has a lot of nodes,
// statisticsSeries should be available so that a min/median/max Graph can be
// used instead of a lot of single lines.
func prepareJobData(
	jobData schema.JobData,
	scopes []schema.MetricScope,
) {
	const maxSeriesSize int = 15
	for _, scopes := range jobData {
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
		jobData.AddNodeScope("flops_any")
		jobData.AddNodeScope("mem_bw")
	}
}
