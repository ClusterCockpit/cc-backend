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
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type MetricDataRepository interface {
	// Initialize this MetricDataRepository. One instance of
	// this interface will only ever be responsible for one cluster.
	Init(rawConfig json.RawMessage) error

	// Return the JobData for the given job, only with the requested metrics.
	LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error)

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

func GetMetricDataRepo(cluster string) (MetricDataRepository, error) {
	var err error
	repo, ok := metricDataRepos[cluster]

	if !ok {
		err = fmt.Errorf("METRICDATA/METRICDATA > no metric data repository configured for '%s'", cluster)
	}

	return repo, err
}
