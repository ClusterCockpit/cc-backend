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

func TestHealthCheckAlt(t *testing.T) {
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

	// Setup test data for node001 - all metrics healthy
	node001 := ms.root.findLevelOrCreate([]string{"testcluster", "node001"}, len(metrics))
	for i := 0; i < len(metrics); i++ {
		node001.metrics[i] = newBuffer(startTime, 10)
		// Write recent data with no NaN values
		for ts := startTime; ts <= now; ts += 10 {
			node001.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}

	// Setup test data for node002 - some metrics degraded (many NaN values)
	node002 := ms.root.findLevelOrCreate([]string{"testcluster", "node002"}, len(metrics))
	for i := 0; i < len(metrics); i++ {
		node002.metrics[i] = newBuffer(startTime, 10)
		if i < 2 {
			// First two metrics: healthy (no NaN)
			for ts := startTime; ts <= now; ts += 10 {
				node002.metrics[i].write(ts, schema.Float(float64(i+1)))
			}
		} else {
			// Last two metrics: degraded (many NaN values in recent data)
			// Write real values first, then NaN values at the end
			count := 0
			for ts := startTime; ts <= now; ts += 10 {
				if count < 5 {
					// Write first 5 real values
					node002.metrics[i].write(ts, schema.Float(float64(i+1)))
				} else {
					// Write NaN for the rest (last ~6 values will be NaN)
					node002.metrics[i].write(ts, schema.NaN)
				}
				count++
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
			name:            "some metrics degraded",
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
			name:            "multiple nodes mixed states",
			cluster:         "testcluster",
			nodes:           []string{"node001", "node002", "node003", "node004"},
			expectedMetrics: []string{"load", "mem_used"},
			wantStates: map[string]schema.MonitoringState{
				"node001": schema.MonitoringStateFull,
				"node002": schema.MonitoringStateFull,
				"node003": schema.MonitoringStateFull,
				"node004": schema.MonitoringStateFailed,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := ms.HealthCheckAlt(tt.cluster, tt.nodes, tt.expectedMetrics)
			if err != nil {
				t.Errorf("HealthCheckAlt() error = %v", err)
				return
			}

			// Check that we got results for all nodes
			if len(results) != len(tt.nodes) {
				t.Errorf("HealthCheckAlt() returned %d results, want %d", len(results), len(tt.nodes))
			}

			// Check each node's state
			for _, node := range tt.nodes {
				state, ok := results[node]
				if !ok {
					t.Errorf("HealthCheckAlt() missing result for node %s", node)
					continue
				}

				// Check status
				if wantStatus, ok := tt.wantStates[node]; ok {
					if state != wantStatus {
						t.Errorf("HealthCheckAlt() node %s status = %v, want %v", node, state, wantStatus)
					}
				}
			}
		})
	}
}
