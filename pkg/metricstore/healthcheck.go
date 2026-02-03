// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type HeathCheckResponse struct {
	Status schema.MonitoringState
	Error  error
	list   List
}

type List struct {
	StaleNodeMetricList       []string
	StaleHardwareMetricList   map[string][]string
	MissingNodeMetricList     []string
	MissingHardwareMetricList map[string][]string
}

// MaxMissingDataPoints is a threshold that allows a node to be healthy with certain number of data points missing.
// Suppose a node does not receive last 5 data points, then healthCheck endpoint will still say a
// node is healthy. Anything more than 5 missing points in metrics of the node will deem the node unhealthy.
const MaxMissingDataPoints int64 = 5

func (b *buffer) healthCheck() bool {
	// Check if the buffer is empty
	if b.data == nil {
		return true
	}

	bufferEnd := b.start + b.frequency*int64(len(b.data))
	t := time.Now().Unix()

	// Check if the buffer is too old
	if t-bufferEnd > MaxMissingDataPoints*b.frequency {
		return true
	}

	return false
}

// healthCheck recursively examines a level and all its children to identify stale or missing metrics.
//
// This routine performs a two-phase check:
//
// Phase 1 - Check metrics at current level (node-level metrics):
//   - Iterates through all configured metrics in m.Metrics
//   - For each metric, checks if a buffer exists at l.metrics[mc.offset]
//   - If buffer exists: calls buffer.healthCheck() to verify data freshness
//   - Stale buffer (data older than MaxMissingDataPoints * frequency) → StaleNodeMetricList
//   - Fresh buffer → healthy, no action
//   - If buffer is nil: metric was never written → MissingNodeMetricList
//
// Phase 2 - Recursively check child levels (hardware-level metrics):
//   - Iterates through l.children (e.g., "cpu0", "gpu0", "socket0")
//   - Recursively calls healthCheck() on each child level
//   - Aggregates child results into hardware-specific lists:
//   - Child's StaleNodeMetricList → parent's StaleHardwareMetricList[childName]
//   - Child's MissingNodeMetricList → parent's MissingHardwareMetricList[childName]
//
// The recursive nature means:
//   - Calling on a host level checks: host metrics + all CPU/GPU/socket metrics
//   - Calling on a socket level checks: socket metrics + all core metrics
//   - Leaf levels (e.g., individual cores) only check their own metrics
//
// Parameters:
//   - m: MemoryStore containing the global metric configuration (m.Metrics)
//
// Returns:
//   - List: Categorized lists of stale and missing metrics at this level and below
//   - error: Non-nil only for internal errors during recursion
//
// Concurrency:
//   - Acquires read lock (RLock) to safely access l.metrics and l.children
//   - Lock held for entire duration including recursive calls
//
// Example for host level with structure: host → [cpu0, cpu1]:
//   - Checks host-level metrics (load, memory) → StaleNodeMetricList / MissingNodeMetricList
//   - Recursively checks cpu0 metrics → results in StaleHardwareMetricList["cpu0"]
//   - Recursively checks cpu1 metrics → results in StaleHardwareMetricList["cpu1"]
func (l *Level) healthCheck(m *MemoryStore) (List, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	list := List{
		StaleNodeMetricList:       make([]string, 0),
		StaleHardwareMetricList:   make(map[string][]string, 0),
		MissingNodeMetricList:     make([]string, 0),
		MissingHardwareMetricList: make(map[string][]string, 0),
	}

	// Phase 1: Check metrics at this level
	for metricName, mc := range m.Metrics {
		if b := l.metrics[mc.offset]; b != nil {
			if b.healthCheck() {
				list.StaleNodeMetricList = append(list.StaleNodeMetricList, metricName)
			}
		} else {
			list.MissingNodeMetricList = append(list.MissingNodeMetricList, metricName)
		}
	}

	// Phase 2: Recursively check child levels (hardware components)
	for hardwareMetricName, lvl := range l.children {
		l, err := lvl.healthCheck(m)
		if err != nil {
			return List{}, err
		}

		if len(l.StaleNodeMetricList) != 0 {
			list.StaleHardwareMetricList[hardwareMetricName] = l.StaleNodeMetricList
		}
		if len(l.MissingNodeMetricList) != 0 {
			list.MissingHardwareMetricList[hardwareMetricName] = l.MissingNodeMetricList
		}
	}

	return list, nil
}

// HealthCheck performs a health check on a specific node in the metric store.
//
// This routine checks whether metrics for a given node are being received and are up-to-date.
// It examines both node-level metrics (e.g., load, memory) and hardware-level metrics
// (e.g., CPU, GPU, network) to determine the monitoring state.
//
// Parameters:
//   - selector: Hierarchical path to the target node, typically []string{cluster, hostname}.
//     Example: []string{"emmy", "node001"} navigates to the "node001" host in the "emmy" cluster.
//     The selector must match the hierarchy used during metric ingestion (see Level.findLevelOrCreate).
//   - subcluster: Subcluster name (currently unused, reserved for future filtering)
//
// Returns:
//   - *HeathCheckResponse: Health status with detailed lists of stale/missing metrics
//   - error: Non-nil only for internal errors (not for unhealthy nodes)
//
// Health States:
//   - MonitoringStateFull: All expected metrics are present and up-to-date
//   - MonitoringStatePartial: Some metrics are stale (data older than MaxMissingDataPoints * frequency)
//   - MonitoringStateFailed: Host not found, or metrics are completely missing
//
// The response includes detailed lists:
//   - StaleNodeMetricList: Node-level metrics with stale data
//   - StaleHardwareMetricList: Hardware-level metrics with stale data (grouped by component)
//   - MissingNodeMetricList: Expected node-level metrics that have no data
//   - MissingHardwareMetricList: Expected hardware-level metrics that have no data (grouped by component)
//
// Example usage:
//
//	selector := []string{"emmy", "node001"}
//	response, err := ms.HealthCheck(selector, "")
//	if err != nil {
//	    // Internal error
//	}
//	switch response.Status {
//	case schema.MonitoringStateFull:
//	    // All metrics healthy
//	case schema.MonitoringStatePartial:
//	    // Check response.list.StaleNodeMetricList for details
//	case schema.MonitoringStateFailed:
//	    // Check response.Error or response.list.MissingNodeMetricList
//	}
func (m *MemoryStore) HealthCheck(selector []string, subcluster string) (*HeathCheckResponse, error) {
	response := HeathCheckResponse{
		Status: schema.MonitoringStateFull,
	}

	lvl := m.root.findLevel(selector)
	if lvl == nil {
		response.Status = schema.MonitoringStateFailed
		response.Error = fmt.Errorf("[METRICSTORE]> error while HealthCheck, host not found: %#v", selector)
		return &response, nil
	}

	var err error

	response.list, err = lvl.healthCheck(m)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Response: %#v\n", response)

	if len(response.list.StaleNodeMetricList) != 0 ||
		len(response.list.StaleHardwareMetricList) != 0 {
		response.Status = schema.MonitoringStatePartial
		return &response, nil
	}

	if len(response.list.MissingHardwareMetricList) != 0 ||
		len(response.list.MissingNodeMetricList) != 0 {
		response.Status = schema.MonitoringStateFailed
		return &response, nil
	}

	return &response, nil
}

// isBufferHealthy checks if a buffer has received data for the last MaxMissingDataPoints.
//
// Returns true if the buffer is healthy (recent data within threshold), false otherwise.
// A nil buffer or empty buffer is considered unhealthy.
func (b *buffer) isBufferHealthy() bool {
	// Check if the buffer is empty
	if b == nil || b.data == nil {
		return false
	}

	bufferEnd := b.start + b.frequency*int64(len(b.data))
	t := time.Now().Unix()

	// Check if the buffer has recent data (within MaxMissingDataPoints threshold)
	if t-bufferEnd > MaxMissingDataPoints*b.frequency {
		return false
	}

	return true
}

// countMissingValues counts the number of NaN (missing) values in the most recent data points.
//
// Examines the last MaxMissingDataPoints*2 values in the buffer and counts how many are NaN.
// We check twice the threshold to allow detecting when more than MaxMissingDataPoints are missing.
// If the buffer has fewer values, examines all available values.
//
// Returns:
//   - int: Number of NaN values found in the examined range
func (b *buffer) countMissingValues() int {
	if b == nil || b.data == nil || len(b.data) == 0 {
		return 0
	}

	// Check twice the threshold to detect degraded metrics
	checkCount := min(int(MaxMissingDataPoints)*2, len(b.data))

	// Count NaN values in the most recent data points
	missingCount := 0
	startIdx := len(b.data) - checkCount
	for i := startIdx; i < len(b.data); i++ {
		if b.data[i].IsNaN() {
			missingCount++
		}
	}

	return missingCount
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
func (l *Level) getHealthyMetrics(m *MemoryStore) ([]string, []string, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	healthyList := make([]string, 0)
	degradedList := make([]string, 0)

	// Phase 1: Check metrics at this level
	for metricName, mc := range m.Metrics {
		b := l.metrics[mc.offset]
		if b.isBufferHealthy() {
			// Buffer has recent data, now check for missing values
			missingCount := b.countMissingValues()
			if missingCount > int(MaxMissingDataPoints) {
				degradedList = append(degradedList, metricName)
			} else {
				healthyList = append(healthyList, metricName)
			}
		}
	}

	// Phase 2: Recursively check child levels (hardware components)
	for _, lvl := range l.children {
		childHealthy, childDegraded, err := lvl.getHealthyMetrics(m)
		if err != nil {
			return nil, nil, err
		}

		// Merge child metrics into flat lists
		healthyList = append(healthyList, childHealthy...)
		degradedList = append(degradedList, childDegraded...)
	}

	return healthyList, degradedList, nil
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
func (m *MemoryStore) GetHealthyMetrics(selector []string) ([]string, []string, error) {
	lvl := m.root.findLevel(selector)
	if lvl == nil {
		return nil, nil, fmt.Errorf("[METRICSTORE]> error while GetHealthyMetrics, host not found: %#v", selector)
	}

	healthyList, degradedList, err := lvl.getHealthyMetrics(m)
	if err != nil {
		return nil, nil, err
	}

	return healthyList, degradedList, nil
}

// HealthCheckAlt performs health checks on multiple nodes and returns their monitoring states.
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
//	healthStates, err := ms.HealthCheckAlt(cluster, nodes, expectedMetrics)
//	if err != nil {
//	    return err
//	}
//	for hostname, state := range healthStates {
//	    fmt.Printf("Node %s: %s\n", hostname, state)
//	}
//
// Note: This routine is optimized for batch operations where you need to check
// the same set of metrics across multiple nodes. For single-node checks with
// all configured metrics, use HealthCheck() instead.
func (m *MemoryStore) HealthCheckAlt(cluster string,
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
		healthyList, degradedList, err := m.GetHealthyMetrics(selector)
		if err != nil {
			// Node not found or internal error
			results[hostname] = schema.MonitoringStateFailed
			continue
		}

		// Create sets for fast lookup
		healthySet := make(map[string]bool, len(healthyList))
		for _, metric := range healthyList {
			healthySet[metric] = true
		}
		degradedSet := make(map[string]bool, len(degradedList))
		for _, metric := range degradedList {
			degradedSet[metric] = true
		}

		// Classify each expected metric
		for _, metric := range expectedMetrics {
			if healthySet[metric] {
				healthyCount++
			} else if degradedSet[metric] {
				degradedCount++
			} else {
				missingCount++
			}
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
