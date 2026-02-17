// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	ccms "github.com/ClusterCockpit/cc-backend/internal/metricstoreclient"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type MetricDataRepository interface {
	// Return the JobData for the given job, only with the requested metrics.
	LoadData(job *schema.Job,
		metrics []string,
		scopes []schema.MetricScope,
		ctx context.Context,
		resolution int) (schema.JobData, error)

	// Return a map of metrics to a map of nodes to the metric statistics of the job. node scope only.
	LoadStats(job *schema.Job,
		metrics []string,
		ctx context.Context) (map[string]map[string]schema.MetricStatistics, error)

	// Return a map of metrics to a map of scopes to the scoped metric statistics of the job.
	LoadScopedStats(job *schema.Job,
		metrics []string,
		scopes []schema.MetricScope,
		ctx context.Context) (schema.ScopedJobStats, error)

	// Return a map of hosts to a map of metrics at the requested scopes (currently only node) for that node.
	LoadNodeData(cluster string,
		metrics, nodes []string,
		scopes []schema.MetricScope,
		from, to time.Time,
		ctx context.Context) (map[string]map[string][]*schema.JobMetric, error)

	// Return a map of hosts to a map of metrics to a map of scopes for multiple nodes.
	LoadNodeListData(cluster, subCluster string,
		nodes []string,
		metrics []string,
		scopes []schema.MetricScope,
		resolution int,
		from, to time.Time,
		ctx context.Context) (map[string]schema.JobData, error)

	// HealthCheck evaluates the monitoring state for a set of nodes against expected metrics.
	HealthCheck(cluster string,
		nodes []string,
		metrics []string) (map[string]metricstore.HealthCheckResult, error)
}

type CCMetricStoreConfig struct {
	Scope string `json:"scope"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

var metricDataRepos map[string]MetricDataRepository = map[string]MetricDataRepository{}

func Init(rawConfig json.RawMessage) error {
	if rawConfig != nil {
		var configs []CCMetricStoreConfig
		config.Validate(configSchema, rawConfig)
		dec := json.NewDecoder(bytes.NewReader(rawConfig))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&configs); err != nil {
			return fmt.Errorf("[METRICDISPATCH]> Metric Store Config Init: Could not decode config file '%s' Error: %s", rawConfig, err.Error())
		}

		if len(configs) == 0 {
			return fmt.Errorf("[METRICDISPATCH]> No metric store configurations found in config file")
		}

		for _, config := range configs {
			metricDataRepos[config.Scope] = ccms.NewCCMetricStore(config.URL, config.Token)
		}
	}

	return nil
}

func GetMetricDataRepo(cluster string, subcluster string) (MetricDataRepository, error) {
	var repo MetricDataRepository
	var ok bool

	key := cluster + "-" + subcluster
	repo, ok = metricDataRepos[key]

	if !ok {
		repo, ok = metricDataRepos[cluster]

		if !ok {
			repo, ok = metricDataRepos["*"]

			if !ok {
				if metricstore.MetricStoreHandle == nil {
					return nil, fmt.Errorf("[METRICDISPATCH]> no metric data repository configured '%s'", key)
				}

				repo = metricstore.MetricStoreHandle
				cclog.Debugf("[METRICDISPATCH]> Using internal metric data repository for '%s'", key)
			}
		}
	}

	return repo, nil
}

// GetHealthCheckRepo returns the MetricDataRepository for performing health checks on a cluster.
// It uses the same fallback logic as GetMetricDataRepo: cluster → wildcard → internal.
func GetHealthCheckRepo(cluster string) (MetricDataRepository, error) {
	return GetMetricDataRepo(cluster, "")
}
