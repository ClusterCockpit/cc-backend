// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstore provides config.go: Configuration structures and metric management.
//
// # Configuration Hierarchy
//
// The metricstore package uses nested configuration structures:
//
//	MetricStoreConfig (Keys)
//	├─ NumWorkers: Parallel checkpoint/archive workers
//	├─ RetentionInMemory: How long to keep data in RAM
//	├─ MemoryCap: Memory limit in bytes (triggers forceFree)
//	├─ Checkpoints: Persistence configuration
//	│  ├─ FileFormat: "avro" or "json"
//	│  ├─ Interval: How often to save (e.g., "1h")
//	│  └─ RootDir: Checkpoint storage path
//	├─ Cleanup: Long-term storage configuration
//	│  ├─ Interval: How often to delete/archive
//	│  ├─ RootDir: Archive storage path
//	│  └─ Mode: "delete" or "archive"
//	├─ Debug: Development/debugging options
//	└─ Subscriptions: NATS topic subscriptions for metric ingestion
//
// # Metric Configuration
//
// Each metric (e.g., "cpu_load", "mem_used") has a MetricConfig entry in the global
// Metrics map, defining:
//
//   - Frequency: Measurement interval in seconds
//   - Aggregation: How to combine values (sum/avg/none) when transforming scopes
//   - offset: Internal index into Level.metrics slice (assigned during Init)
//
// # AggregationStrategy
//
// Determines how to combine metric values when aggregating from finer to coarser scopes:
//
//   - NoAggregation: Do not combine (incompatible scopes)
//   - SumAggregation: Add values (e.g., power consumption: core→socket)
//   - AvgAggregation: Average values (e.g., temperature: core→socket)
package metricstore

import (
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

const (
	DefaultMaxWorkers                 = 10
	DefaultBufferCapacity             = 512
	DefaultGCTriggerInterval          = 100
	DefaultAvroWorkers                = 4
	DefaultCheckpointBufferMin        = 3
	DefaultAvroCheckpointInterval     = time.Minute
	DefaultMemoryUsageTrackerInterval = 1 * time.Hour
)

// Checkpoints configures periodic persistence of in-memory metric data.
//
// Fields:
//   - FileFormat: "avro" (default, binary, compact) or "json" (human-readable, slower)
//   - Interval:   Duration string (e.g., "1h", "30m") between checkpoint saves
//   - RootDir:    Filesystem path for checkpoint files (created if missing)
type Checkpoints struct {
	FileFormat string `json:"file-format"`
	Interval   string `json:"interval"`
	RootDir    string `json:"directory"`
}

// Debug provides development and profiling options.
//
// Fields:
//   - DumpToFile: Path to dump checkpoint data for inspection (empty = disabled)
//   - EnableGops: Enable gops agent for live runtime debugging (https://github.com/google/gops)
type Debug struct {
	DumpToFile string `json:"dump-to-file"`
	EnableGops bool   `json:"gops"`
}

// Archive configures long-term storage of old metric data.
//
// Data older than RetentionInMemory is archived to disk or deleted.
//
// Fields:
//   - ArchiveInterval: Duration string (e.g., "24h") between archive operations
//   - RootDir:         Filesystem path for archived data (created if missing)
//   - DeleteInstead:   If true, delete old data instead of archiving (saves disk space)
type Cleanup struct {
	Interval string `json:"interval"`
	RootDir  string `json:"directory"`
	Mode     string `json:"mode"`
}

// Subscriptions defines NATS topics to subscribe to for metric ingestion.
//
// Each subscription receives metrics via NATS messaging, enabling real-time
// data collection from compute nodes.
//
// Fields:
//   - SubscribeTo: NATS subject/channel name (e.g., "metrics.compute.*")
//   - ClusterTag:  Default cluster name for metrics without cluster tag (optional)
type Subscriptions []struct {
	// Channel name
	SubscribeTo string `json:"subscribe-to"`

	// Allow lines without a cluster tag, use this as default, optional
	ClusterTag string `json:"cluster-tag"`
}

// MetricStoreConfig defines the main configuration for the metricstore.
//
// Loaded from cc-backend's config.json "metricstore" section. Controls memory usage,
// persistence, archiving, and metric ingestion.
//
// Fields:
//   - NumWorkers:        Parallel workers for checkpoint/archive (0 = auto: min(NumCPU/2+1, 10))
//   - RetentionInMemory: Duration string (e.g., "48h") for in-memory data retention
//   - MemoryCap:         Max bytes for buffer data (0 = unlimited); triggers forceFree when exceeded
//   - Checkpoints:       Periodic persistence configuration
//   - Debug:             Development/profiling options (nil = disabled)
//   - Archive:           Long-term storage configuration (nil = disabled)
//   - Subscriptions:     NATS topics for metric ingestion (nil = polling only)
type MetricStoreConfig struct {
	// Number of concurrent workers for checkpoint and archive operations.
	// If not set or 0, defaults to min(runtime.NumCPU()/2+1, 10)
	NumWorkers        int            `json:"num-workers"`
	RetentionInMemory string         `json:"retention-in-memory"`
	MemoryCap         int            `json:"memory-cap"`
	Checkpoints       Checkpoints    `json:"checkpoints"`
	Debug             *Debug         `json:"debug"`
	Cleanup           *Cleanup       `json:"cleanup"`
	Subscriptions     *Subscriptions `json:"nats-subscriptions"`
}

// Keys is the global metricstore configuration instance.
//
// Initialized with defaults, then overwritten by cc-backend's config.json.
// Accessed by Init(), Checkpointing(), and other lifecycle functions.
var Keys MetricStoreConfig = MetricStoreConfig{
	Checkpoints: Checkpoints{
		FileFormat: "avro",
		RootDir:    "./var/checkpoints",
	},
	Cleanup: &Cleanup{
		Mode: "delete",
	},
}

// AggregationStrategy defines how to combine metric values across hierarchy levels.
//
// Used when transforming data from finer-grained scopes (e.g., core) to coarser scopes
// (e.g., socket). This is SPATIAL aggregation, not TEMPORAL (time-based) aggregation.
//
// Values:
//   - NoAggregation:  Do not aggregate (incompatible scopes or non-aggregatable metrics)
//   - SumAggregation: Add values (e.g., power: sum core power → socket power)
//   - AvgAggregation: Average values (e.g., temperature: average core temps → socket temp)
type AggregationStrategy int

const (
	NoAggregation  AggregationStrategy = iota // Do not aggregate
	SumAggregation                            // Sum values (e.g., power, energy)
	AvgAggregation                            // Average values (e.g., temperature, utilization)
)

// AssignAggregationStrategy parses a string into an AggregationStrategy value.
//
// Used when loading metric configurations from JSON/YAML files.
//
// Parameters:
//   - str: "sum", "avg", or "" (empty string for NoAggregation)
//
// Returns:
//   - AggregationStrategy: Parsed value
//   - error:               Non-nil if str is unrecognized
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

// MetricConfig defines configuration for a single metric type.
//
// Stored in the global Metrics map, keyed by metric name (e.g., "cpu_load").
//
// Fields:
//   - Frequency:   Measurement interval in seconds (e.g., 60 for 1-minute granularity)
//   - Aggregation: How to combine values across hierarchy levels (sum/avg/none)
//   - offset:      Internal index into Level.metrics slice (assigned during Init)
type MetricConfig struct {
	// Interval in seconds at which measurements are stored
	Frequency int64

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation AggregationStrategy

	// Private, used internally...
	offset int
}

func BuildMetricList() map[string]MetricConfig {
	var metrics map[string]MetricConfig = make(map[string]MetricConfig)

	addMetric := func(name string, metric MetricConfig) error {
		if metrics == nil {
			metrics = make(map[string]MetricConfig, 0)
		}

		if existingMetric, ok := metrics[name]; ok {
			if existingMetric.Frequency != metric.Frequency {
				if existingMetric.Frequency < metric.Frequency {
					existingMetric.Frequency = metric.Frequency
					metrics[name] = existingMetric
				}
			}
		} else {
			metrics[name] = metric
		}

		return nil
	}

	// Helper function to add metric configuration
	addMetricConfig := func(mc *schema.MetricConfig) {
		agg, err := AssignAggregationStrategy(mc.Aggregation)
		if err != nil {
			cclog.Warnf("Could not find aggregation strategy for metric config '%s': %s", mc.Name, err.Error())
		}

		addMetric(mc.Name, MetricConfig{
			Frequency:   int64(mc.Timestep),
			Aggregation: agg,
		})
	}
	for _, c := range archive.Clusters {
		for _, mc := range c.MetricConfig {
			addMetricConfig(mc)
		}

		for _, sc := range c.SubClusters {
			for _, mc := range sc.MetricConfig {
				addMetricConfig(mc)
			}
		}
	}

	return metrics
}
