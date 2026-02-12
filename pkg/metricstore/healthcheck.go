// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"encoding/json"
	"fmt"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// HealthCheckResponse represents the result of a health check operation.
type HealthCheckResponse struct {
	Status schema.MonitoringState
	Error  error
}

// HealthCheckResult holds the monitoring state and raw JSON health metrics
// for a single node as determined by HealthCheck.
type HealthCheckResult struct {
	State         schema.MonitoringState
	HealthMetrics string // JSON: {"missing":[...],"degraded":[...]}
}

// MaxMissingDataPoints is the threshold for stale data detection.
// A buffer is considered healthy if the gap between its last data point
// and the current time is within MaxMissingDataPoints * frequency.
const MaxMissingDataPoints int64 = 5

// bufferExists returns true if the buffer is non-nil and contains data.
func (b *buffer) bufferExists() bool {
	if b == nil || b.data == nil || len(b.data) == 0 {
		return false
	}

	return true
}

// isBufferHealthy returns true if the buffer has recent data within
// MaxMissingDataPoints * frequency of the current time.
func (b *buffer) isBufferHealthy() bool {
	bufferEnd := b.start + b.frequency*int64(len(b.data))
	t := time.Now().Unix()

	return t-bufferEnd <= MaxMissingDataPoints*b.frequency
}

// collectMetricStatus walks the subtree rooted at l and classifies each
// expected metric into the healthy or degraded map.
//
// Classification rules (evaluated per buffer, pessimistic):
//   - A single stale buffer marks the metric as degraded permanently.
//   - A healthy buffer only counts if no stale buffer has been seen.
//   - Metrics absent from the global config or without any buffer remain
//     in neither map and are later reported as missing.
func (l *Level) collectMetricStatus(m *MemoryStore, expectedMetrics []string, healthy, degraded map[string]bool) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	for _, metricName := range expectedMetrics {
		if degraded[metricName] {
			continue // already degraded, cannot improve
		}
		mc := m.Metrics[metricName]
		b := l.metrics[mc.offset]
		if b.bufferExists() {
			if !b.isBufferHealthy() {
				degraded[metricName] = true
				delete(healthy, metricName)
			} else if !degraded[metricName] {
				healthy[metricName] = true
			}
		}
	}

	for _, lvl := range l.children {
		lvl.collectMetricStatus(m, expectedMetrics, healthy, degraded)
	}
}

// getHealthyMetrics walks the complete subtree rooted at l and classifies
// each expected metric by comparing the collected status against the
// expected list.
//
// Returns:
//   - missingList: metrics not found in global config or without any buffer
//   - degradedList: metrics with at least one stale buffer in the subtree
func (l *Level) getHealthyMetrics(m *MemoryStore, expectedMetrics []string) ([]string, []string) {
	healthy := make(map[string]bool, len(expectedMetrics))
	degraded := make(map[string]bool)

	l.collectMetricStatus(m, expectedMetrics, healthy, degraded)

	missingList := make([]string, 0)
	degradedList := make([]string, 0)

	for _, metricName := range expectedMetrics {
		if healthy[metricName] {
			continue
		}

		if degraded[metricName] {
			degradedList = append(degradedList, metricName)
		} else {
			missingList = append(missingList, metricName)
		}
	}

	return degradedList, missingList
}

// GetHealthyMetrics returns missing and degraded metric lists for a node.
//
// It walks the metric tree starting from the node identified by selector
// and classifies each expected metric:
//   - Missing: no buffer anywhere in the subtree, or metric not in global config
//   - Degraded: at least one stale buffer exists in the subtree
//
// Metrics present in expectedMetrics but absent from both returned lists
// are considered fully healthy.
func (m *MemoryStore) GetHealthyMetrics(selector []string, expectedMetrics []string) ([]string, []string, error) {
	lvl := m.root.findLevel(selector)
	if lvl == nil {
		return nil, nil, fmt.Errorf("[METRICSTORE]> GetHealthyMetrics: host not found: %#v", selector)
	}

	degradedList, missingList := lvl.getHealthyMetrics(m, expectedMetrics)
	return degradedList, missingList, nil
}

// HealthCheck evaluates multiple nodes against a set of expected metrics
// and returns a monitoring state per node.
//
// States:
//   - MonitoringStateFull: all expected metrics are healthy
//   - MonitoringStatePartial: some metrics are missing or degraded
//   - MonitoringStateFailed: node not found, or no healthy metrics at all
func (m *MemoryStore) HealthCheck(cluster string,
	nodes []string, expectedMetrics []string,
) (map[string]HealthCheckResult, error) {
	results := make(map[string]HealthCheckResult, len(nodes))

	for _, hostname := range nodes {
		selector := []string{cluster, hostname}

		degradedList, missingList, err := m.GetHealthyMetrics(selector, expectedMetrics)
		if err != nil {
			results[hostname] = HealthCheckResult{State: schema.MonitoringStateFailed}
			continue
		}

		degradedCount := len(degradedList)
		missingCount := len(missingList)

		healthyCount := len(expectedMetrics) - degradedCount - missingCount

		if degradedCount > 0 {
			cclog.ComponentInfo("metricstore", "HealthCheck: node ", hostname, "degraded metrics:", degradedList)
		}
		if missingCount > 0 {
			cclog.ComponentInfo("metricstore", "HealthCheck: node ", hostname, "missing metrics:", missingList)
		}

		var state schema.MonitoringState
		switch {
		case degradedCount == 0 && missingCount == 0:
			state = schema.MonitoringStateFull
		case healthyCount == 0:
			state = schema.MonitoringStateFailed
		default:
			state = schema.MonitoringStatePartial
		}

		hm, _ := json.Marshal(map[string][]string{
			"missing":  missingList,
			"degraded": degradedList,
		})

		results[hostname] = HealthCheckResult{
			State:         state,
			HealthMetrics: string(hm),
		}
	}

	return results, nil
}
