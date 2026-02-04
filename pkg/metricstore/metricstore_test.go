// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestAssignAggregationStrategy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AggregationStrategy
		wantErr  bool
	}{
		{"empty string", "", NoAggregation, false},
		{"sum", "sum", SumAggregation, false},
		{"avg", "avg", AvgAggregation, false},
		{"invalid", "invalid", NoAggregation, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AssignAggregationStrategy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("AssignAggregationStrategy(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("AssignAggregationStrategy(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBufferWrite(t *testing.T) {
	b := newBuffer(100, 10)

	// Test writing value
	nb, err := b.write(100, schema.Float(42.0))
	if err != nil {
		t.Errorf("buffer.write() error = %v", err)
	}
	if nb != b {
		t.Error("buffer.write() created new buffer unexpectedly")
	}
	if len(b.data) != 1 {
		t.Errorf("buffer.write() len(data) = %d, want 1", len(b.data))
	}
	if b.data[0] != schema.Float(42.0) {
		t.Errorf("buffer.write() data[0] = %v, want 42.0", b.data[0])
	}

	// Test writing value from past (should error)
	_, err = b.write(50, schema.Float(10.0))
	if err == nil {
		t.Error("buffer.write() expected error for past timestamp")
	}
}

func TestBufferRead(t *testing.T) {
	b := newBuffer(100, 10)

	// Write some test data
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))
	b.write(120, schema.Float(3.0))

	// Read data
	data := make([]schema.Float, 3)
	result, from, to, err := b.read(100, 130, data)
	if err != nil {
		t.Errorf("buffer.read() error = %v", err)
	}
	// Buffer read should return from as firstWrite (start + freq/2)
	if from != 100 {
		t.Errorf("buffer.read() from = %d, want 100", from)
	}
	if to != 130 {
		t.Errorf("buffer.read() to = %d, want 130", to)
	}
	if len(result) != 3 {
		t.Errorf("buffer.read() len(result) = %d, want 3", len(result))
	}
}

func TestHealthCheck(t *testing.T) {
	// Create a test MemoryStore with some metrics
	metrics := map[string]MetricConfig{
		"load":       {Frequency: 10, Aggregation: AvgAggregation, offset: 0},
		"mem_used":   {Frequency: 10, Aggregation: AvgAggregation, offset: 1},
		"cpu_user":   {Frequency: 10, Aggregation: AvgAggregation, offset: 2},
		"cpu_system": {Frequency: 10, Aggregation: AvgAggregation, offset: 3},
	}

	ms := &MemoryStore{
		Metrics: metrics,
		root: Level{
			metrics:  make([]*buffer, len(metrics)),
			children: make(map[string]*Level),
		},
	}

	// Use recent timestamps (current time minus a small offset)
	now := time.Now().Unix()
	startTime := now - 100 // Start 100 seconds ago to have enough data points

	// Setup test data for node001 - all metrics healthy (recent data)
	node001 := ms.root.findLevelOrCreate([]string{"testcluster", "node001"}, len(metrics))
	for i := 0; i < len(metrics); i++ {
		node001.metrics[i] = newBuffer(startTime, 10)
		// Write recent data up to now
		for ts := startTime; ts <= now; ts += 10 {
			node001.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}

	// Setup test data for node002 - some metrics stale (old data beyond MaxMissingDataPoints threshold)
	node002 := ms.root.findLevelOrCreate([]string{"testcluster", "node002"}, len(metrics))
	// MaxMissingDataPoints = 5, frequency = 10, so threshold is 50 seconds
	staleTime := now - 100 // Data ends 100 seconds ago (well beyond 50 second threshold)
	for i := 0; i < len(metrics); i++ {
		node002.metrics[i] = newBuffer(staleTime-50, 10)
		if i < 2 {
			// First two metrics: healthy (recent data)
			for ts := startTime; ts <= now; ts += 10 {
				node002.metrics[i].write(ts, schema.Float(float64(i+1)))
			}
		} else {
			// Last two metrics: stale (data ends 100 seconds ago)
			for ts := staleTime - 50; ts <= staleTime; ts += 10 {
				node002.metrics[i].write(ts, schema.Float(float64(i+1)))
			}
		}
	}

	// Setup test data for node003 - some metrics missing (no buffer)
	node003 := ms.root.findLevelOrCreate([]string{"testcluster", "node003"}, len(metrics))
	// Only create buffers for first two metrics
	for i := 0; i < 2; i++ {
		node003.metrics[i] = newBuffer(startTime, 10)
		for ts := startTime; ts <= now; ts += 10 {
			node003.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}
	// Leave metrics[2] and metrics[3] as nil (missing)

	// Setup test data for node005 - all metrics stale
	node005 := ms.root.findLevelOrCreate([]string{"testcluster", "node005"}, len(metrics))
	for i := 0; i < len(metrics); i++ {
		node005.metrics[i] = newBuffer(staleTime-50, 10)
		// All metrics have stale data (ends 100 seconds ago)
		for ts := staleTime - 50; ts <= staleTime; ts += 10 {
			node005.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}

	// node004 doesn't exist at all

	tests := []struct {
		name            string
		cluster         string
		nodes           []string
		expectedMetrics []string
		wantStates      map[string]schema.MonitoringState
	}{
		{
			name:            "all metrics healthy",
			cluster:         "testcluster",
			nodes:           []string{"node001"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node001": schema.MonitoringStateFull,
			},
		},
		{
			name:            "some metrics stale",
			cluster:         "testcluster",
			nodes:           []string{"node002"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node002": schema.MonitoringStatePartial,
			},
		},
		{
			name:            "some metrics missing",
			cluster:         "testcluster",
			nodes:           []string{"node003"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node003": schema.MonitoringStatePartial,
			},
		},
		{
			name:            "node not found",
			cluster:         "testcluster",
			nodes:           []string{"node004"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node004": schema.MonitoringStateFailed,
			},
		},
		{
			name:            "all metrics stale",
			cluster:         "testcluster",
			nodes:           []string{"node005"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node005": schema.MonitoringStateFailed,
			},
		},
		{
			name:            "multiple nodes mixed states",
			cluster:         "testcluster",
			nodes:           []string{"node001", "node002", "node003", "node004", "node005"},
			expectedMetrics: []string{"load", "mem_used"},
			wantStates: map[string]schema.MonitoringState{
				"node001": schema.MonitoringStateFull,
				"node002": schema.MonitoringStateFull,   // Only checking first 2 metrics which are healthy
				"node003": schema.MonitoringStateFull,   // Only checking first 2 metrics which exist
				"node004": schema.MonitoringStateFailed, // Node doesn't exist
				"node005": schema.MonitoringStateFailed, // Both metrics are stale
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := ms.HealthCheck(tt.cluster, tt.nodes, tt.expectedMetrics)
			if err != nil {
				t.Errorf("HealthCheck() error = %v", err)
				return
			}

			// Check that we got results for all nodes
			if len(results) != len(tt.nodes) {
				t.Errorf("HealthCheck() returned %d results, want %d", len(results), len(tt.nodes))
			}

			// Check each node's state
			for _, node := range tt.nodes {
				state, ok := results[node]
				if !ok {
					t.Errorf("HealthCheck() missing result for node %s", node)
					continue
				}

				// Check status
				if wantStatus, ok := tt.wantStates[node]; ok {
					if state != wantStatus {
						t.Errorf("HealthCheck() node %s status = %v, want %v", node, state, wantStatus)
					}
				}
			}
		})
	}
}

// TestGetHealthyMetrics tests the GetHealthyMetrics function which returns lists of missing and degraded metrics
func TestGetHealthyMetrics(t *testing.T) {
	metrics := map[string]MetricConfig{
		"load":     {Frequency: 10, Aggregation: AvgAggregation, offset: 0},
		"mem_used": {Frequency: 10, Aggregation: AvgAggregation, offset: 1},
		"cpu_user": {Frequency: 10, Aggregation: AvgAggregation, offset: 2},
	}

	ms := &MemoryStore{
		Metrics: metrics,
		root: Level{
			metrics:  make([]*buffer, len(metrics)),
			children: make(map[string]*Level),
		},
	}

	now := time.Now().Unix()
	startTime := now - 100
	staleTime := now - 100

	// Setup node with mixed health states
	node := ms.root.findLevelOrCreate([]string{"testcluster", "testnode"}, len(metrics))

	// Metric 0 (load): healthy - recent data
	node.metrics[0] = newBuffer(startTime, 10)
	for ts := startTime; ts <= now; ts += 10 {
		node.metrics[0].write(ts, schema.Float(1.0))
	}

	// Metric 1 (mem_used): degraded - stale data
	node.metrics[1] = newBuffer(staleTime-50, 10)
	for ts := staleTime - 50; ts <= staleTime; ts += 10 {
		node.metrics[1].write(ts, schema.Float(2.0))
	}

	// Metric 2 (cpu_user): missing - no buffer (nil)

	tests := []struct {
		name            string
		selector        []string
		expectedMetrics []string
		wantMissing     []string
		wantDegraded    []string
		wantErr         bool
	}{
		{
			name:            "mixed health states",
			selector:        []string{"testcluster", "testnode"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user"},
			wantMissing:     []string{"cpu_user"},
			wantDegraded:    []string{"mem_used"},
			wantErr:         false,
		},
		{
			name:            "node not found",
			selector:        []string{"testcluster", "nonexistent"},
			expectedMetrics: []string{"load"},
			wantMissing:     nil,
			wantDegraded:    nil,
			wantErr:         true,
		},
		{
			name:            "check only healthy metric",
			selector:        []string{"testcluster", "testnode"},
			expectedMetrics: []string{"load"},
			wantMissing:     []string{},
			wantDegraded:    []string{},
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, degraded, err := ms.GetHealthyMetrics(tt.selector, tt.expectedMetrics)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetHealthyMetrics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Check missing list
			if len(missing) != len(tt.wantMissing) {
				t.Errorf("GetHealthyMetrics() missing = %v, want %v", missing, tt.wantMissing)
			} else {
				for i, m := range tt.wantMissing {
					if missing[i] != m {
						t.Errorf("GetHealthyMetrics() missing[%d] = %v, want %v", i, missing[i], m)
					}
				}
			}

			// Check degraded list
			if len(degraded) != len(tt.wantDegraded) {
				t.Errorf("GetHealthyMetrics() degraded = %v, want %v", degraded, tt.wantDegraded)
			} else {
				for i, d := range tt.wantDegraded {
					if degraded[i] != d {
						t.Errorf("GetHealthyMetrics() degraded[%d] = %v, want %v", i, degraded[i], d)
					}
				}
			}
		})
	}
}

// TestBufferHealthChecks tests the buffer-level health check functions
func TestBufferHealthChecks(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name        string
		setupBuffer func() *buffer
		wantExists  bool
		wantHealthy bool
		description string
	}{
		{
			name: "nil buffer",
			setupBuffer: func() *buffer {
				return nil
			},
			wantExists:  false,
			wantHealthy: false,
			description: "nil buffer should not exist and not be healthy",
		},
		{
			name: "empty buffer",
			setupBuffer: func() *buffer {
				b := newBuffer(now, 10)
				b.data = nil
				return b
			},
			wantExists:  false,
			wantHealthy: false,
			description: "empty buffer should not exist and not be healthy",
		},
		{
			name: "healthy buffer with recent data",
			setupBuffer: func() *buffer {
				b := newBuffer(now-30, 10)
				// Write data up to now (within MaxMissingDataPoints * frequency = 50 seconds)
				for ts := now - 30; ts <= now; ts += 10 {
					b.write(ts, schema.Float(1.0))
				}
				return b
			},
			wantExists:  true,
			wantHealthy: true,
			description: "buffer with recent data should be healthy",
		},
		{
			name: "stale buffer beyond threshold",
			setupBuffer: func() *buffer {
				b := newBuffer(now-200, 10)
				// Write data that ends 100 seconds ago (beyond MaxMissingDataPoints * frequency = 50 seconds)
				for ts := now - 200; ts <= now-100; ts += 10 {
					b.write(ts, schema.Float(1.0))
				}
				return b
			},
			wantExists:  true,
			wantHealthy: false,
			description: "buffer with stale data should exist but not be healthy",
		},
		{
			name: "buffer at threshold boundary",
			setupBuffer: func() *buffer {
				b := newBuffer(now-50, 10)
				// Write data that ends exactly at threshold (MaxMissingDataPoints * frequency = 50 seconds)
				for ts := now - 50; ts <= now-50; ts += 10 {
					b.write(ts, schema.Float(1.0))
				}
				return b
			},
			wantExists:  true,
			wantHealthy: true,
			description: "buffer at threshold boundary should still be healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.setupBuffer()

			exists := b.bufferExists()
			if exists != tt.wantExists {
				t.Errorf("bufferExists() = %v, want %v: %s", exists, tt.wantExists, tt.description)
			}

			if b != nil && b.data != nil && len(b.data) > 0 {
				healthy := b.isBufferHealthy()
				if healthy != tt.wantHealthy {
					t.Errorf("isBufferHealthy() = %v, want %v: %s", healthy, tt.wantHealthy, tt.description)
				}
			}
		})
	}
}
