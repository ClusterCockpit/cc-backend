// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricsync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type MetricDataSource interface {
	// Initialize this MetricDataRepository. One instance of
	// this interface will only ever be responsible for one cluster.
	Init(rawConfig json.RawMessage) error

	// Return a map of hosts to a map of metrics at the requested scopes (currently only node) for that node.
	Pull(cluster string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error)
}

var metricDataSourceRepos map[string]MetricDataSource = map[string]MetricDataSource{}

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

			var mdr MetricDataSource
			switch kind.Kind {
			case "cc-metric-store":
			case "prometheus":
				// mdr = &PrometheusDataRepository{}
			case "test":
				// mdr = &TestMetricDataRepository{}
			default:
				return fmt.Errorf("METRICDATA/METRICDATA > Unknown MetricDataRepository %v for cluster %v", kind.Kind, cluster.Name)
			}

			if err := mdr.Init(cluster.MetricDataRepository); err != nil {
				cclog.Errorf("Error initializing MetricDataRepository %v for cluster %v", kind.Kind, cluster.Name)
				return err
			}
			metricDataSourceRepos[cluster.Name] = mdr
		}
	}
	return nil
}
