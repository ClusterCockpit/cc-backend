// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"fmt"
	"time"
)

const (
	DefaultMaxWorkers             = 10
	DefaultBufferCapacity         = 512
	DefaultGCTriggerInterval      = 100
	DefaultAvroWorkers            = 4
	DefaultCheckpointBufferMin    = 3
	DefaultAvroCheckpointInterval = time.Minute
)

type MetricStoreConfig struct {
	// Number of concurrent workers for checkpoint and archive operations.
	// If not set or 0, defaults to min(runtime.NumCPU()/2+1, 10)
	NumWorkers  int `json:"num-workers"`
	Checkpoints struct {
		FileFormat string `json:"file-format"`
		Interval   string `json:"interval"`
		RootDir    string `json:"directory"`
		Restore    string `json:"restore"`
	} `json:"checkpoints"`
	Debug struct {
		DumpToFile string `json:"dump-to-file"`
		EnableGops bool   `json:"gops"`
	} `json:"debug"`
	// Global default retention duration
	RetentionInMemory string `json:"retention-in-memory"`
	// Per-cluster retention overrides
	Clusters []struct {
		Cluster           string `json:"cluster"`
		RetentionInMemory string `json:"retention-in-memory"`
		// Per-subcluster retention overrides within this cluster
		SubClusters []struct {
			SubCluster        string `json:"subcluster"`
			RetentionInMemory string `json:"retention-in-memory"`
		} `json:"subclusters,omitempty"`
	} `json:"clusters,omitempty"`
	Archive struct {
		Interval      string `json:"interval"`
		RootDir       string `json:"directory"`
		DeleteInstead bool   `json:"delete-instead"`
	} `json:"archive"`
	Subscriptions []struct {
		// Channel name
		SubscribeTo string `json:"subscribe-to"`

		// Allow lines without a cluster tag, use this as default, optional
		ClusterTag string `json:"cluster-tag"`
	} `json:"subscriptions"`
}

var Keys MetricStoreConfig

type retentionConfig struct {
	global        time.Duration
	clusterMap    map[string]time.Duration
	subClusterMap map[string]map[string]time.Duration
}

var retentionLookup *retentionConfig

// AggregationStrategy for aggregation over multiple values at different cpus/sockets/..., not time!
type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

func AssignAggregationStrategy(str string) (AggregationStrategy, error) {
	switch str {
	case "":
		return NoAggregation, nil
	case "sum":
		return SumAggregation, nil
	case "avg":
		return AvgAggregation, nil
	default:
		return NoAggregation, fmt.Errorf("[METRICSTORE]> unknown aggregation strategy: %s", str)
	}
}

type MetricConfig struct {
	// Interval in seconds at which measurements are stored
	Frequency int64

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation AggregationStrategy

	// Private, used internally...
	offset int
}

var Metrics map[string]MetricConfig

func GetMetricFrequency(metricName string) (int64, error) {
	if metric, ok := Metrics[metricName]; ok {
		return metric.Frequency, nil
	}
	return 0, fmt.Errorf("[METRICSTORE]> metric %s not found", metricName)
}

// AddMetric adds logic to add metrics. Redundant metrics should be updated with max frequency.
// use metric.Name to check if the metric already exists.
// if not, add it to the Metrics map.
func AddMetric(name string, metric MetricConfig) error {
	if Metrics == nil {
		Metrics = make(map[string]MetricConfig, 0)
	}

	if existingMetric, ok := Metrics[name]; ok {
		if existingMetric.Frequency != metric.Frequency {
			if existingMetric.Frequency < metric.Frequency {
				existingMetric.Frequency = metric.Frequency
				Metrics[name] = existingMetric
			}
		}
	} else {
		Metrics[name] = metric
	}

	return nil
}

func GetRetentionDuration(cluster, subCluster string) (time.Duration, error) {
	if retentionLookup == nil {
		return 0, fmt.Errorf("[METRICSTORE]> retention configuration not initialized")
	}

	if subCluster != "" {
		if subMap, ok := retentionLookup.subClusterMap[cluster]; ok {
			if retention, ok := subMap[subCluster]; ok {
				return retention, nil
			}
		}
	}

	if retention, ok := retentionLookup.clusterMap[cluster]; ok {
		return retention, nil
	}

	return retentionLookup.global, nil
}

// GetShortestRetentionDuration returns the shortest configured retention duration
// across all levels (global, cluster, and subcluster configurations).
// Returns 0 if retentionLookup is not initialized or global retention is not set.
func GetShortestRetentionDuration() time.Duration {
	if retentionLookup == nil || retentionLookup.global <= 0 {
		return 0
	}

	shortest := retentionLookup.global

	// Check all cluster-level retention durations
	for _, clusterRetention := range retentionLookup.clusterMap {
		if clusterRetention > 0 && clusterRetention < shortest {
			shortest = clusterRetention
		}
	}

	// Check all subcluster-level retention durations
	for _, subClusterMap := range retentionLookup.subClusterMap {
		for _, scRetention := range subClusterMap {
			if scRetention > 0 && scRetention < shortest {
				shortest = scRetention
			}
		}
	}

	return shortest
}
