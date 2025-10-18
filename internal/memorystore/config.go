// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
)

var InternalCCMSFlag bool = false

type MetricStoreConfig struct {
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
	RetentionInMemory string `json:"retention-in-memory"`
	Archive           struct {
		Interval      string `json:"interval"`
		RootDir       string `json:"directory"`
		DeleteInstead bool   `json:"delete-instead"`
	} `json:"archive"`
	Nats []*NatsConfig `json:"nats"`
}

type NatsConfig struct {
	// Address of the nats server
	Address string `json:"address"`

	// Username/Password, optional
	Username string `json:"username"`
	Password string `json:"password"`

	// Creds file path
	Credsfilepath string `json:"creds-file-path"`

	Subscriptions []struct {
		// Channel name
		SubscribeTo string `json:"subscribe-to"`

		// Allow lines without a cluster tag, use this as default, optional
		ClusterTag string `json:"cluster-tag"`
	} `json:"subscriptions"`
}

var Keys MetricStoreConfig

// AggregationStrategy for aggregation over multiple values at different cpus/sockets/..., not time!
type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

func AssignAggregationStratergy(str string) (AggregationStrategy, error) {
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

func InitMetricStore(rawConfig json.RawMessage) {
	if rawConfig != nil {
		config.Validate(configSchema, rawConfig)
		dec := json.NewDecoder(bytes.NewReader(rawConfig))
		// dec.DisallowUnknownFields()
		if err := dec.Decode(&Keys); err != nil {
			cclog.Abortf("[METRICSTORE]> Metric Store Config Init: Could not decode config file '%s'.\nError: %s\n", rawConfig, err.Error())
		}
	}
}

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
