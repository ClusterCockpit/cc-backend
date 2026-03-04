// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricstore

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// makeTopology creates a simple 2-socket, 4-core, 8-hwthread topology for testing.
// Socket 0: cores 0,1 with hwthreads 0,1,2,3
// Socket 1: cores 2,3 with hwthreads 4,5,6,7
// MemoryDomain 0: hwthreads 0,1,2,3 (socket 0)
// MemoryDomain 1: hwthreads 4,5,6,7 (socket 1)
func makeTopology() schema.Topology {
	topo := schema.Topology{
		Node:         []int{0, 1, 2, 3, 4, 5, 6, 7},
		Socket:       [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}},
		MemoryDomain: [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}},
		Core:         [][]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}},
		Accelerators: []*schema.Accelerator{
			{ID: "gpu0"},
			{ID: "gpu1"},
		},
	}
	return topo
}

func TestBuildScopeQueries(t *testing.T) {
	topo := makeTopology()
	topo.InitTopologyMaps()
	accIds := topo.GetAcceleratorIDs()

	tests := []struct {
		name           string
		nativeScope    schema.MetricScope
		requestedScope schema.MetricScope
		expectOk       bool
		expectLen      int // expected number of results
		expectAgg      bool
		expectScope    schema.MetricScope
	}{
		// Same-scope cases
		{
			name: "HWThread->HWThread", nativeScope: schema.MetricScopeHWThread,
			requestedScope: schema.MetricScopeHWThread, expectOk: true, expectLen: 1,
			expectAgg: false, expectScope: schema.MetricScopeHWThread,
		},
		{
			name: "Core->Core", nativeScope: schema.MetricScopeCore,
			requestedScope: schema.MetricScopeCore, expectOk: true, expectLen: 1,
			expectAgg: false, expectScope: schema.MetricScopeCore,
		},
		{
			name: "Socket->Socket", nativeScope: schema.MetricScopeSocket,
			requestedScope: schema.MetricScopeSocket, expectOk: true, expectLen: 1,
			expectAgg: false, expectScope: schema.MetricScopeSocket,
		},
		{
			name: "MemoryDomain->MemoryDomain", nativeScope: schema.MetricScopeMemoryDomain,
			requestedScope: schema.MetricScopeMemoryDomain, expectOk: true, expectLen: 1,
			expectAgg: false, expectScope: schema.MetricScopeMemoryDomain,
		},
		{
			name: "Node->Node", nativeScope: schema.MetricScopeNode,
			requestedScope: schema.MetricScopeNode, expectOk: true, expectLen: 1,
			expectAgg: false, expectScope: schema.MetricScopeNode,
		},
		{
			name: "Accelerator->Accelerator", nativeScope: schema.MetricScopeAccelerator,
			requestedScope: schema.MetricScopeAccelerator, expectOk: true, expectLen: 1,
			expectAgg: false, expectScope: schema.MetricScopeAccelerator,
		},
		// Aggregation cases
		{
			name: "HWThread->Core", nativeScope: schema.MetricScopeHWThread,
			requestedScope: schema.MetricScopeCore, expectOk: true, expectLen: 4, // 4 cores
			expectAgg: true, expectScope: schema.MetricScopeCore,
		},
		{
			name: "HWThread->Socket", nativeScope: schema.MetricScopeHWThread,
			requestedScope: schema.MetricScopeSocket, expectOk: true, expectLen: 2, // 2 sockets
			expectAgg: true, expectScope: schema.MetricScopeSocket,
		},
		{
			name: "HWThread->Node", nativeScope: schema.MetricScopeHWThread,
			requestedScope: schema.MetricScopeNode, expectOk: true, expectLen: 1,
			expectAgg: true, expectScope: schema.MetricScopeNode,
		},
		{
			name: "Core->Socket", nativeScope: schema.MetricScopeCore,
			requestedScope: schema.MetricScopeSocket, expectOk: true, expectLen: 2, // 2 sockets
			expectAgg: true, expectScope: schema.MetricScopeSocket,
		},
		{
			name: "Core->Node", nativeScope: schema.MetricScopeCore,
			requestedScope: schema.MetricScopeNode, expectOk: true, expectLen: 1,
			expectAgg: true, expectScope: schema.MetricScopeNode,
		},
		{
			name: "Socket->Node", nativeScope: schema.MetricScopeSocket,
			requestedScope: schema.MetricScopeNode, expectOk: true, expectLen: 1,
			expectAgg: true, expectScope: schema.MetricScopeNode,
		},
		{
			name: "MemoryDomain->Node", nativeScope: schema.MetricScopeMemoryDomain,
			requestedScope: schema.MetricScopeNode, expectOk: true, expectLen: 1,
			expectAgg: true, expectScope: schema.MetricScopeNode,
		},
		{
			name: "MemoryDomain->Socket", nativeScope: schema.MetricScopeMemoryDomain,
			requestedScope: schema.MetricScopeSocket, expectOk: true, expectLen: 2, // 2 sockets
			expectAgg: true, expectScope: schema.MetricScopeSocket,
		},
		{
			name: "Accelerator->Node", nativeScope: schema.MetricScopeAccelerator,
			requestedScope: schema.MetricScopeNode, expectOk: true, expectLen: 1,
			expectAgg: true, expectScope: schema.MetricScopeNode,
		},
		// Expected exception: Accelerator scope requested but non-accelerator scope in between
		{
			name: "Accelerator->Core (exception)", nativeScope: schema.MetricScopeAccelerator,
			requestedScope: schema.MetricScopeCore, expectOk: true, expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, ok := BuildScopeQueries(
				tt.nativeScope, tt.requestedScope,
				"test_metric", "node001",
				&topo, topo.Node, accIds,
			)

			if ok != tt.expectOk {
				t.Fatalf("expected ok=%v, got ok=%v", tt.expectOk, ok)
			}

			if len(results) != tt.expectLen {
				t.Fatalf("expected %d results, got %d", tt.expectLen, len(results))
			}

			if tt.expectLen > 0 {
				for _, r := range results {
					if r.Scope != tt.expectScope {
						t.Errorf("expected scope %s, got %s", tt.expectScope, r.Scope)
					}
					if r.Aggregate != tt.expectAgg {
						t.Errorf("expected aggregate=%v, got %v", tt.expectAgg, r.Aggregate)
					}
					if r.Metric != "test_metric" {
						t.Errorf("expected metric 'test_metric', got '%s'", r.Metric)
					}
					if r.Hostname != "node001" {
						t.Errorf("expected hostname 'node001', got '%s'", r.Hostname)
					}
				}
			}
		})
	}
}

func TestBuildScopeQueries_UnhandledCase(t *testing.T) {
	topo := makeTopology()
	topo.InitTopologyMaps()

	// Node native with HWThread requested => scope.Max = Node, but let's try an invalid combination
	// Actually all valid combinations are handled. An unhandled case would be something like
	// a scope that doesn't exist in the if-chain. Since all real scopes are covered,
	// we test with a synthetic unhandled combination by checking the bool return.
	// The function should return ok=false for truly unhandled cases.

	// For now, verify all known combinations return ok=true
	scopes := []schema.MetricScope{
		schema.MetricScopeHWThread, schema.MetricScopeCore,
		schema.MetricScopeSocket, schema.MetricScopeNode,
	}

	for _, native := range scopes {
		for _, requested := range scopes {
			results, ok := BuildScopeQueries(
				native, requested,
				"m", "h", &topo, topo.Node, nil,
			)
			if !ok {
				t.Errorf("unexpected unhandled case: native=%s, requested=%s", native, requested)
			}
			if results == nil {
				t.Errorf("results should not be nil for native=%s, requested=%s", native, requested)
			}
		}
	}
}

func TestIntToStringSlice(t *testing.T) {
	tests := []struct {
		input    []int
		expected []string
	}{
		{nil, nil},
		{[]int{}, nil},
		{[]int{0}, []string{"0"}},
		{[]int{1, 2, 3}, []string{"1", "2", "3"}},
		{[]int{10, 100, 1000}, []string{"10", "100", "1000"}},
	}

	for _, tt := range tests {
		result := IntToStringSlice(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("IntToStringSlice(%v): expected len %d, got %d", tt.input, len(tt.expected), len(result))
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("IntToStringSlice(%v)[%d]: expected %s, got %s", tt.input, i, tt.expected[i], result[i])
			}
		}
	}
}

func TestSanitizeStats(t *testing.T) {
	// Test: all valid - should remain unchanged
	avg, min, max := schema.Float(1.0), schema.Float(0.5), schema.Float(2.0)
	SanitizeStats(&avg, &min, &max)
	if avg != 1.0 || min != 0.5 || max != 2.0 {
		t.Errorf("SanitizeStats should not change valid values")
	}

	// Test: one NaN - all should be zeroed
	avg, min, max = schema.Float(1.0), schema.Float(0.5), schema.NaN
	SanitizeStats(&avg, &min, &max)
	if avg != 0 || min != 0 || max != 0 {
		t.Errorf("SanitizeStats should zero all when any is NaN, got avg=%v min=%v max=%v", avg, min, max)
	}

	// Test: all NaN
	avg, min, max = schema.NaN, schema.NaN, schema.NaN
	SanitizeStats(&avg, &min, &max)
	if avg != 0 || min != 0 || max != 0 {
		t.Errorf("SanitizeStats should zero all NaN values")
	}
}

func TestNodeToNodeQuery(t *testing.T) {
	topo := makeTopology()
	topo.InitTopologyMaps()

	results, ok := BuildScopeQueries(
		schema.MetricScopeNode, schema.MetricScopeNode,
		"cpu_load", "node001",
		&topo, topo.Node, nil,
	)

	if !ok {
		t.Fatal("expected ok=true for Node->Node")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Type != nil {
		t.Error("Node->Node should have nil Type")
	}
	if r.TypeIds != nil {
		t.Error("Node->Node should have nil TypeIds")
	}
	if r.Aggregate {
		t.Error("Node->Node should not aggregate")
	}
}
