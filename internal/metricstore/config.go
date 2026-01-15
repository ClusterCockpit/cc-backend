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

type Checkpoints struct {
	FileFormat string `json:"file-format"`
	Interval   string `json:"interval"`
	RootDir    string `json:"directory"`
}

type Debug struct {
	DumpToFile string `json:"dump-to-file"`
	EnableGops bool   `json:"gops"`
}

type Archive struct {
	ArchiveInterval string `json:"interval"`
	RootDir         string `json:"directory"`
	DeleteInstead   bool   `json:"delete-instead"`
}

type Subscriptions []struct {
	// Channel name
	SubscribeTo string `json:"subscribe-to"`

	// Allow lines without a cluster tag, use this as default, optional
	ClusterTag string `json:"cluster-tag"`
}

type MetricStoreConfig struct {
	// Number of concurrent workers for checkpoint and archive operations.
	// If not set or 0, defaults to min(runtime.NumCPU()/2+1, 10)
	NumWorkers        int            `json:"num-workers"`
	RetentionInMemory string         `json:"retention-in-memory"`
	MemoryCap         int            `json:"memory-cap"`
	Checkpoints       Checkpoints    `json:"checkpoints"`
	Debug             *Debug         `json:"debug"`
	Archive           *Archive       `json:"archive"`
	Subscriptions     *Subscriptions `json:"nats-subscriptions"`
}

var Keys MetricStoreConfig = MetricStoreConfig{
	Checkpoints: Checkpoints{
		FileFormat: "avro",
		RootDir:    "./var/checkpoints",
	},
}

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
