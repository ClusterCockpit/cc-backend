// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// HealthCheckResponse represents the result of a health check operation.
//
// Status indicates the monitoring state (Full, Partial, Failed).
// Error contains any error encountered during the health check.
type HealthCheckResponse struct {
	Status schema.MonitoringState
	Error  error
}

// MaxMissingDataPoints is a threshold that allows a node to be healthy with certain number of data points missing.
// Suppose a node does not receive last 5 data points, then healthCheck endpoint will still say a
// node is healthy. Anything more than 5 missing points in metrics of the node will deem the node unhealthy.
const MaxMissingDataPoints int64 = 5

// isBufferHealthy checks if a buffer has received data for the last MaxMissingDataPoints.
//
// Returns true if the buffer is healthy (recent data within threshold), false otherwise.
// A nil buffer or empty buffer is considered unhealthy.
func (b *buffer) bufferExists() bool {
	// Check if the buffer is empty
	if b == nil || b.data == nil || len(b.data) == 0 {
		return false
	}

	return true
}

// isBufferHealthy checks if a buffer has received data for the last MaxMissingDataPoints.
//
// Returns true if the buffer is healthy (recent data within threshold), false otherwise.
// A nil buffer or empty buffer is considered unhealthy.
func (b *buffer) isBufferHealthy() bool {
	// Get the last endtime of the buffer
	bufferEnd := b.start + b.frequency*int64(len(b.data))
	t := time.Now().Unix()

	// Check if the buffer has recent data (within MaxMissingDataPoints threshold)
	if t-bufferEnd > MaxMissingDataPoints*b.frequency {
		return false
	}

	return true
}

// MergeUniqueSorted merges two lists, sorts them, and removes duplicates.
// Requires 'cmp.Ordered' because we need to sort the data.
func mergeList[string cmp.Ordered](list1, list2 []string) []string {
	// 1. Combine both lists
	result := append(list1, list2...)

	// 2. Sort the combined list
	slices.Sort(result)

	// 3. Compact removes consecutive duplicates (standard in Go 1.21+)
	// e.g. [1, 1, 2, 3, 3] -> [1, 2, 3]
	result = slices.Compact(result)

	return result
}

// getHealthyMetrics recursively collects healthy and degraded metrics at this level and below.
//
// A metric is considered:
//   - Healthy: buffer has recent data within MaxMissingDataPoints threshold AND has few/no NaN values
//   - Degraded: buffer exists and has recent data, but contains more than MaxMissingDataPoints NaN values
//
// This routine walks the entire subtree starting from the current level.
//
// Parameters:
//   - m: MemoryStore containing the global metric configuration
//
// Returns:
//   - []string: Flat list of healthy metric names from this level and all children
//   - []string: Flat list of degraded metric names (exist but have too many missing values)
//   - error: Non-nil only for internal errors during recursion
//
// The routine mirrors healthCheck() but provides more granular classification:
//   - healthCheck() finds problems (stale/missing)
//   - getHealthyMetrics() separates healthy from degraded metrics
func (l *Level) getHealthyMetrics(m *MemoryStore, expectedMetrics []string) ([]string, []string, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	globalMetrics := m.Metrics

	missingList := make([]string, 0)
	degradedList := make([]string, 0)

	// Phase 1: Check metrics at this level
	for _, metricName := range expectedMetrics {
		offset := globalMetrics[metricName].offset
		b := l.metrics[offset]

		if !b.bufferExists() {
			missingList = append(missingList, metricName)
		} else if !b.isBufferHealthy() {
			degradedList = append(degradedList, metricName)
		}
	}

	// Phase 2: Recursively check child levels
	for _, lvl := range l.children {
		childMissing, childDegraded, err := lvl.getHealthyMetrics(m, expectedMetrics)
		if err != nil {
			return nil, nil, err
		}

		missingList = mergeList(missingList, childMissing)
		degradedList = mergeList(degradedList, childDegraded)
	}

	return missingList, degradedList, nil
}

// GetHealthyMetrics returns healthy and degraded metrics for a specific node as flat lists.
//
// This routine walks the metric tree starting from the specified node selector
// and collects all metrics that have received data within the last MaxMissingDataPoints
// (default: 5 data points). Metrics are classified into two categories:
//
//   - Healthy: Buffer has recent data AND contains few/no NaN (missing) values
//   - Degraded: Buffer has recent data BUT contains more than MaxMissingDataPoints NaN values
//
// The returned lists include both node-level metrics (e.g., "load", "mem_used") and
// hardware-level metrics (e.g., "cpu_user", "gpu_temp") in flat slices.
//
// Parameters:
//   - selector: Hierarchical path to the target node, typically []string{cluster, hostname}.
//     Example: []string{"emmy", "node001"} navigates to the "node001" host in the "emmy" cluster.
//     The selector must match the hierarchy used during metric ingestion.
//
// Returns:
//   - []string: Flat list of healthy metric names (recent data, few missing values)
//   - []string: Flat list of degraded metric names (recent data, many missing values)
//   - error: Non-nil if the node is not found or internal errors occur
//
// Example usage:
//
//	selector := []string{"emmy", "node001"}
//	healthyMetrics, degradedMetrics, err := ms.GetHealthyMetrics(selector)
//	if err != nil {
//	    // Node not found or internal error
//	    return err
//	}
//	fmt.Printf("Healthy metrics: %v\n", healthyMetrics)
//	// Output: ["load", "mem_used", "cpu_user", ...]
//	fmt.Printf("Degraded metrics: %v\n", degradedMetrics)
//	// Output: ["gpu_temp", "network_rx", ...] (metrics with many NaN values)
//
// Note: This routine provides more granular classification than HealthCheck:
//   - HealthCheck reports stale/missing metrics (problems)
//   - GetHealthyMetrics separates fully healthy from degraded metrics (quality levels)
func (m *MemoryStore) GetHealthyMetrics(selector []string, expectedMetrics []string) ([]string, []string, error) {
	lvl := m.root.findLevel(selector)
	if lvl == nil {
		return nil, nil, fmt.Errorf("[METRICSTORE]> error while GetHealthyMetrics, host not found: %#v", selector)
	}

	missingList, degradedList, err := lvl.getHealthyMetrics(m, expectedMetrics)
	if err != nil {
		return nil, nil, err
	}

	return missingList, degradedList, nil
}

// HealthCheck performs health checks on multiple nodes and returns their monitoring states.
//
// This routine provides a batch health check interface that evaluates multiple nodes
// against a specific set of expected metrics. For each node, it determines the overall
// monitoring state based on which metrics are healthy, degraded, or missing.
//
// Health Status Classification:
//   - MonitoringStateFull: All expected metrics are healthy (recent data, few missing values)
//   - MonitoringStatePartial: Some metrics are degraded (many missing values) or missing
//   - MonitoringStateFailed: Node not found or all expected metrics are missing/stale
//
// Parameters:
//   - cluster: Cluster name (first element of selector path)
//   - nodes: List of node hostnames to check
//   - expectedMetrics: List of metric names that should be present on each node
//
// Returns:
//   - map[string]schema.MonitoringState: Map keyed by hostname containing monitoring state for each node
//   - error: Non-nil only for internal errors (individual node failures are captured as MonitoringStateFailed)
//
// Example usage:
//
//	cluster := "emmy"
//	nodes := []string{"node001", "node002", "node003"}
//	expectedMetrics := []string{"load", "mem_used", "cpu_user", "cpu_system"}
//	healthStates, err := ms.HealthCheck(cluster, nodes, expectedMetrics)
//	if err != nil {
//	    return err
//	}
//	for hostname, state := range healthStates {
//	    fmt.Printf("Node %s: %s\n", hostname, state)
//	}
//
// Note: This routine is optimized for batch operations where you need to check
// the same set of metrics across multiple nodes.
func (m *MemoryStore) HealthCheck(cluster string,
	nodes []string, expectedMetrics []string,
) (map[string]schema.MonitoringState, error) {
	results := make(map[string]schema.MonitoringState, len(nodes))

	// Create a set of expected metrics for fast lookup
	expectedSet := make(map[string]bool, len(expectedMetrics))
	for _, metric := range expectedMetrics {
		expectedSet[metric] = true
	}

	// Check each node
	for _, hostname := range nodes {
		selector := []string{cluster, hostname}
		status := schema.MonitoringStateFull
		healthyCount := 0
		degradedCount := 0
		missingCount := 0

		// Get healthy and degraded metrics for this node
		missingList, degradedList, err := m.GetHealthyMetrics(selector, expectedMetrics)
		if err != nil {
			// Node not found or internal error
			results[hostname] = schema.MonitoringStateFailed
			continue
		}

		missingCount = len(missingList)
		degradedCount = len(degradedList)
		healthyCount = len(expectedMetrics) - (missingCount + degradedCount)

		// Debug log missing and degraded metrics
		if missingCount > 0 {
			cclog.ComponentDebug("metricstore", "HealthCheck: node", hostname, "missing metrics:", missingList)
		}
		if degradedCount > 0 {
			cclog.ComponentDebug("metricstore", "HealthCheck: node", hostname, "degraded metrics:", degradedList)
		}

		// Determine overall health status
		if missingCount > 0 || degradedCount > 0 {
			if healthyCount == 0 {
				// No healthy metrics at all
				status = schema.MonitoringStateFailed
			} else {
				// Some healthy, some degraded/missing
				status = schema.MonitoringStatePartial
			}
		}
		// else: all metrics healthy, status remains MonitoringStateFull

		results[hostname] = status
	}

	return results, nil
}
