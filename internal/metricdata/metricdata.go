// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
)

type MetricDataRepository interface {
	// Initialize this MetricDataRepository. One instance of
	// this interface will only ever be responsible for one cluster.
	Init(rawConfig json.RawMessage) error

	// Return the JobData for the given job, only with the requested metrics.
	LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error)

	// Return a map of metrics to a map of nodes to the metric statistics of the job. node scope only.
	LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error)

	// Return a map of metrics to a map of scopes to the scoped metric statistics of the job.
	LoadScopedStats(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.ScopedJobStats, error)

	// Return a map of hosts to a map of metrics at the requested scopes (currently only node) for that node.
	LoadNodeData(cluster string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error)

	// Return a map of hosts to a map of metrics to a map of scopes for multiple nodes.
	LoadNodeListData(cluster, subCluster, nodeFilter string, metrics []string, scopes []schema.MetricScope, resolution int, from, to time.Time, page *model.PageRequest, ctx context.Context) (map[string]schema.JobData, int, bool, error)
}

var metricDataRepos map[string]MetricDataRepository = map[string]MetricDataRepository{}

func Init() error {
	for _, cluster := range config.Clusters {
		if cluster.MetricDataRepository != nil {
			var kind struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(cluster.MetricDataRepository, &kind); err != nil {
				cclog.Warn("Error while unmarshaling raw json MetricDataRepository")
				return err
			}

			var mdr MetricDataRepository
			switch kind.Kind {
			case "cc-metric-store":
				mdr = &CCMetricStore{}
			case "cc-metric-store-internal":
				mdr = &CCMetricStoreInternal{}
				config.InternalCCMSFlag = true
			case "prometheus":
				mdr = &PrometheusDataRepository{}
			case "test":
				mdr = &TestMetricDataRepository{}
			default:
				return fmt.Errorf("METRICDATA/METRICDATA > Unknown MetricDataRepository %v for cluster %v", kind.Kind, cluster.Name)
			}

			if err := mdr.Init(cluster.MetricDataRepository); err != nil {
				cclog.Errorf("Error initializing MetricDataRepository %v for cluster %v", kind.Kind, cluster.Name)
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
