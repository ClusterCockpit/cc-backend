// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/memorystore"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/go-co-op/gocron/v2"
)

// RegisterMetricPullWorker registers a background worker that pulls metric data
// from upstream backends and populates the internal memorystore
func RegisterMetricPullWorker() {
	d, err := parseDuration(Keys.MetricPullWorker)
	if err != nil {
		cclog.Warnf("Could not parse duration for metric pull worker, using default 60s")
		d = 60 * time.Second
	}

	if d == 0 {
		cclog.Info("Metric pull worker disabled (interval is zero)")
		return
	}

	// Register one worker per cluster
	registered := 0
	for _, cluster := range config.Clusters {
		cluster := cluster // capture for closure

		_, err := s.NewJob(
			gocron.DurationJob(d),
			gocron.NewTask(func() {
				if err := pullMetricsForCluster(cluster.Name); err != nil {
					cclog.Errorf("Metric pull failed for cluster %s: %s", cluster.Name, err)
				}
			}),
			gocron.WithStartAt(gocron.WithStartImmediately()),
		)
		if err != nil {
			cclog.Errorf("Failed to register metric pull worker for cluster %s: %s", cluster.Name, err)
		} else {
			registered++
		}
	}

	if registered > 0 {
		cclog.Infof("Metric pull worker registered for %d clusters (interval: %s)", registered, d)
	}
}

// pullMetricsForCluster pulls metric data for all nodes in a cluster from the upstream backend
func pullMetricsForCluster(clusterName string) error {
	startTime := time.Now()

	// 1. Get cluster configuration (includes all nodes)
	cluster := archive.GetCluster(clusterName)
	if cluster == nil {
		return fmt.Errorf("cluster %s not found in configuration", clusterName)
	}

	// 2. Use nil for nodes to query all nodes in the cluster
	// The LoadNodeData implementation will default to all nodes when nil is passed
	var nodes []string = nil

	// 3. Get all metrics for this cluster
	metrics := []string{}
	for _, mc := range cluster.MetricConfig {
		metrics = append(metrics, mc.Name)
	}

	// 4. Determine time range (last 60 minutes)
	to := time.Now()
	from := to.Add(-60 * time.Minute)

	// 5. Get upstream backend repository (from separate config)
	upstreamRepo, err := metricdata.GetUpstreamMetricDataRepo(clusterName)
	if err != nil {
		cclog.Debugf("No upstream repository configured for cluster %s, skipping pull", clusterName)
		return nil
	}

	// 6. Query upstream backend for ALL scopes
	scopes := []schema.MetricScope{
		schema.MetricScopeNode,
		schema.MetricScopeCore,
		schema.MetricScopeHWThread,
		schema.MetricScopeAccelerator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	nodeData, err := upstreamRepo.LoadNodeData(
		clusterName, metrics, nodes, scopes, from, to, ctx,
	)
	if err != nil {
		cclog.Errorf("Failed to load node data for cluster %s: %s", clusterName, err)
		return err
	}

	// 7. Write data to memorystore
	ms := memorystore.GetMemoryStore()
	nodesWritten := 0
	metricsWritten := 0
	errorsEncountered := 0

	for node, metricMap := range nodeData {
		for metricName, jobMetrics := range metricMap {
			for _, jm := range jobMetrics {
				if err := writeMetricToMemoryStore(ms, clusterName, node, metricName, jm, from); err != nil {
					cclog.Warnf("Failed to write metric %s for node %s: %s", metricName, node, err)
					errorsEncountered++
				} else {
					metricsWritten++
				}
			}
		}
		nodesWritten++
	}

	duration := time.Since(startTime)
	if errorsEncountered > 0 {
		cclog.Infof("Pulled metrics for cluster %s: %d nodes, %d metrics, %d errors (took %s)",
			clusterName, nodesWritten, metricsWritten, errorsEncountered, duration)
	} else {
		cclog.Debugf("Pulled metrics for cluster %s: %d nodes, %d metrics (took %s)",
			clusterName, nodesWritten, metricsWritten, duration)
	}

	return nil
}

// writeMetricToMemoryStore converts a JobMetric from upstream backend
// into memorystore format and writes it
func writeMetricToMemoryStore(
	ms *memorystore.MemoryStore,
	cluster, hostname, metricName string,
	jm *schema.JobMetric,
	fromTime time.Time,
) error {
	// For each series (node-level or finer scope)
	for _, series := range jm.Series {
		// Build selector based on scope
		selector := buildSelector(cluster, hostname, series)

		// Convert series data to timestamped metric writes
		timestep := int64(jm.Timestep)
		baseTimestamp := fromTime.Unix()

		for i, value := range series.Data {
			ts := baseTimestamp + (int64(i) * timestep)

			metric := memorystore.Metric{
				Name:  metricName,
				Value: value,
			}

			if err := ms.Write(selector, ts, []memorystore.Metric{metric}); err != nil {
				return fmt.Errorf("writing to memorystore: %w", err)
			}
		}
	}
	return nil
}

// buildSelector constructs a selector path for the memorystore
// based on cluster, hostname, and series information
func buildSelector(cluster, hostname string, series schema.Series) []string {
	selector := []string{cluster, hostname}

	// Add scope-specific components based on series metadata
	// JobMetric.Series contains Id field that identifies the specific component
	// Examples:
	//   - Node scope: series.Id might be nil or empty
	//   - Core scope: series.Id might be "0", "1", etc.
	//   - HWThread scope: series.Id might be "0", "1", etc.
	//   - Accelerator scope: series.Id might be "0", "1", etc.

	if series.Id != nil && *series.Id != "" {
		// The selector format in memorystore is: [cluster, host, "type+id", "subtype+id"]
		// For example: ["emmy", "node001", "core0", "hwthread0"]
		//
		// The series.Id from upstream includes the type prefix (e.g., "core0", "hwthread1")
		// We can use it directly as a selector component
		selector = append(selector, *series.Id)
	}

	return selector
}
