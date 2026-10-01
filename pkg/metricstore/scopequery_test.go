// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricstore

import (
	"slices"
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
		Filesystems: []*schema.Filesystem{
			{ID: "/scratch", Type: "lustre"},
			{ID: "/home", Type: "nfs"},
		},
		Networks: []*schema.Network{
			{ID: "ib0", Type: "infiniband"},
		},
	}
	return topo
}

var (
	cpuScopes = []schema.MetricScope{
		schema.MetricScopeHWThread, schema.MetricScopeCore,
		schema.MetricScopeMemoryDomain, schema.MetricScopeSocket,
	}
	deviceScopes = []schema.MetricScope{
		schema.MetricScopeAccelerator, schema.MetricScopeFilesystem, schema.MetricScopeNetwork,
	}
)

func TestDeviceIDs(t *testing.T) {
	topo := makeTopology()
	allocated := []string{"gpu1"}

	tests := []struct {
		native schema.MetricScope
		want   []string
	}{
		{schema.MetricScopeAccelerator, []string{"gpu1"}},
		{schema.MetricScopeFilesystem, []string{"/scratch", "/home"}},
		{schema.MetricScopeNetwork, []string{"ib0"}},
		{schema.MetricScopeHWThread, nil},
		{schema.MetricScopeNode, nil},
	}
	for _, tt := range tests {
		if got := DeviceIDs(tt.native, &topo, allocated); !slices.Equal(got, tt.want) {
			t.Errorf("DeviceIDs(%s) = %v, want %v", tt.native, got, tt.want)
		}
	}

	if got := DeviceIDs(schema.MetricScopeAccelerator, &topo, nil); len(got) != 0 {
		t.Errorf("DeviceIDs(accelerator) without allocation = %v, want none", got)
	}
}

func TestBuildScopeQueries(t *testing.T) {
	topo := makeTopology()
	topo.InitTopologyMaps()
	accIds := topo.GetAcceleratorIDs()

	type testCase struct {
		name           string
		nativeScope    schema.MetricScope
		requestedScope schema.MetricScope
		noIDs          bool  // pass no device ids
		hwthreads      []int // hardware threads of the job; nil means the whole node
		expectOk       bool
		expectLen      int // expected number of results
		expectAgg      bool
		expectScope    schema.MetricScope
		expectType     string // expected Type of device queries; TypeIds must be the device ids
		// expectTargets maps the id of every aggregated result to its TypeIds,
		// compared as a set because some branches iterate maps. Nil means every
		// result must have a nil id.
		expectTargets map[string][]string
	}

	tests := []testCase{
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
		// Aggregation cases
		{
			name: "HWThread->Core", nativeScope: schema.MetricScopeHWThread,
			requestedScope: schema.MetricScopeCore, expectOk: true, expectLen: 4, // 4 cores
			expectAgg: true, expectScope: schema.MetricScopeCore,
			expectTargets: map[string][]string{
				"0": {"0", "1"}, "1": {"2", "3"}, "2": {"4", "5"}, "3": {"6", "7"},
			},
		},
		{
			name: "HWThread->Socket", nativeScope: schema.MetricScopeHWThread,
			requestedScope: schema.MetricScopeSocket, expectOk: true, expectLen: 2, // 2 sockets
			expectAgg: true, expectScope: schema.MetricScopeSocket,
			expectTargets: map[string][]string{"0": {"0", "1", "2", "3"}, "1": {"4", "5", "6", "7"}},
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
			expectTargets: map[string][]string{"0": {"0", "1"}, "1": {"2", "3"}},
		},
		{
			name: "Core->Socket (job on socket 0)", nativeScope: schema.MetricScopeCore,
			requestedScope: schema.MetricScopeSocket, hwthreads: []int{0, 1, 2, 3},
			expectOk: true, expectLen: 1,
			expectAgg: true, expectScope: schema.MetricScopeSocket,
			expectTargets: map[string][]string{"0": {"0", "1"}},
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
			expectTargets: map[string][]string{"0": {"0"}, "1": {"1"}},
		},
	}

	// Device scopes: native -> native, native -> node, and nothing for every
	// other requested scope, or when the host has no instances.
	for _, native := range deviceScopes {
		n := string(native)
		tests = append(tests,
			testCase{
				name: n + "->" + n, nativeScope: native, requestedScope: native,
				expectOk: true, expectLen: 1, expectAgg: false, expectScope: native,
				expectType: n,
			},
			testCase{
				name: n + "->Node", nativeScope: native, requestedScope: schema.MetricScopeNode,
				expectOk: true, expectLen: 1, expectAgg: true, expectScope: schema.MetricScopeNode,
				expectType: n,
			},
			testCase{
				name: n + "->" + n + " (no ids)", nativeScope: native, requestedScope: native,
				noIDs: true, expectOk: true, expectLen: 0,
			},
			testCase{
				name: n + "->Node (no ids)", nativeScope: native, requestedScope: schema.MetricScopeNode,
				noIDs: true, expectOk: true, expectLen: 0,
			},
		)
		for _, requested := range cpuScopes {
			tests = append(tests, testCase{
				name: n + "->" + string(requested) + " (exception)", nativeScope: native,
				requestedScope: requested, expectOk: true, expectLen: 0,
			})
		}
		for _, requested := range deviceScopes {
			if requested != native {
				tests = append(tests, testCase{
					name: n + "->" + string(requested) + " (exception)", nativeScope: native,
					requestedScope: requested, expectOk: true, expectLen: 0,
				})
			}
		}
	}

	// No fallback to the native scope when a device scope is requested for a
	// CPU or node metric.
	for _, native := range []schema.MetricScope{
		schema.MetricScopeHWThread, schema.MetricScopeCore, schema.MetricScopeNode,
	} {
		for _, requested := range deviceScopes {
			tests = append(tests, testCase{
				name: string(native) + "->" + string(requested) + " (exception)", nativeScope: native,
				requestedScope: requested, expectOk: true, expectLen: 0,
			})
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deviceIDs []string
			if !tt.noIDs {
				deviceIDs = DeviceIDs(tt.nativeScope, &topo, accIds)
			}
			hwthreads := tt.hwthreads
			if hwthreads == nil {
				hwthreads = topo.Node
			}
			results, ok := BuildScopeQueries(
				tt.nativeScope, tt.requestedScope,
				"test_metric", "node001",
				&topo, hwthreads, deviceIDs,
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
					if tt.expectType != "" {
						if r.Type == nil || *r.Type != tt.expectType {
							t.Errorf("expected type %q, got %v", tt.expectType, r.Type)
						}
						if !slices.Equal(r.TypeIds, deviceIDs) {
							t.Errorf("expected type ids %v, got %v", deviceIDs, r.TypeIds)
						}
					}
				}
			}

			if tt.expectTargets == nil {
				for _, r := range results {
					if r.ID != nil {
						t.Errorf("expected nil id, got %q (type ids %v)", *r.ID, r.TypeIds)
					}
				}
				return
			}
			got := make(map[string][]string, len(results))
			for _, r := range results {
				if r.ID == nil {
					t.Fatalf("expected an id for type ids %v, got nil", r.TypeIds)
				}
				if _, dup := got[*r.ID]; dup {
					t.Fatalf("duplicate id %q", *r.ID)
				}
				got[*r.ID] = r.TypeIds
			}
			if len(got) != len(tt.expectTargets) {
				t.Fatalf("expected targets %v, got %v", tt.expectTargets, got)
			}
			for id, want := range tt.expectTargets {
				if !slices.Equal(got[id], want) {
					t.Errorf("target %q: expected type ids %v, got %v", id, want, got[id])
				}
			}
		})
	}
}

func TestBuildScopeQueries_UnhandledCase(t *testing.T) {
	topo := makeTopology()
	topo.InitTopologyMaps()

	// Every combination of a CPU/node scope and a device scope, in both
	// directions, must be handled: either with queries or as an expected
	// exception (empty, ok=true). memoryDomain is left out because hwthread and
	// core metrics have no conversion to it.
	scopes := []schema.MetricScope{
		schema.MetricScopeHWThread, schema.MetricScopeCore,
		schema.MetricScopeSocket, schema.MetricScopeNode,
		schema.MetricScopeAccelerator, schema.MetricScopeFilesystem, schema.MetricScopeNetwork,
	}

	for _, native := range scopes {
		for _, requested := range scopes {
			results, ok := BuildScopeQueries(
				native, requested,
				"m", "h", &topo, topo.Node, DeviceIDs(native, &topo, topo.GetAcceleratorIDs()),
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
