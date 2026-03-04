// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file contains shared scope transformation logic used by both the internal
// metric store (pkg/metricstore) and the external cc-metric-store client
// (internal/metricstoreclient). It extracts the common algorithm for mapping
// between native metric scopes and requested scopes based on cluster topology.
package metricstore

import (
	"strconv"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Pre-converted scope strings avoid repeated string(MetricScope) allocations
// during query construction. Used in ScopeQueryResult.Type field.
var (
	HWThreadString     = string(schema.MetricScopeHWThread)
	CoreString         = string(schema.MetricScopeCore)
	MemoryDomainString = string(schema.MetricScopeMemoryDomain)
	SocketString       = string(schema.MetricScopeSocket)
	AcceleratorString  = string(schema.MetricScopeAccelerator)
)

// ScopeQueryResult is a package-independent intermediate type returned by
// BuildScopeQueries. Each consumer converts it to their own APIQuery type
// (adding Resolution and any other package-specific fields).
type ScopeQueryResult struct {
	Type      *string
	Metric    string
	Hostname  string
	TypeIds   []string
	Scope     schema.MetricScope
	Aggregate bool
}

// BuildScopeQueries generates scope query results for a given scope transformation.
// It returns a slice of results and a boolean indicating success.
// An empty slice means an expected exception (skip this combination).
// ok=false means an unhandled case (caller should return an error).
func BuildScopeQueries(
	nativeScope, requestedScope schema.MetricScope,
	metric, hostname string,
	topology *schema.Topology,
	hwthreads []int,
	accelerators []string,
) ([]ScopeQueryResult, bool) {
	scope := nativeScope.Max(requestedScope)
	results := []ScopeQueryResult{}

	hwthreadsStr := IntToStringSlice(hwthreads)

	// Accelerator -> Accelerator (Use "accelerator" scope if requested scope is lower than node)
	if nativeScope == schema.MetricScopeAccelerator && scope.LT(schema.MetricScopeNode) {
		if scope != schema.MetricScopeAccelerator {
			// Expected Exception -> Return Empty Slice
			return results, true
		}

		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: false,
			Type:      &AcceleratorString,
			TypeIds:   accelerators,
			Scope:     schema.MetricScopeAccelerator,
		})
		return results, true
	}

	// Accelerator -> Node
	if nativeScope == schema.MetricScopeAccelerator && scope == schema.MetricScopeNode {
		if len(accelerators) == 0 {
			// Expected Exception -> Return Empty Slice
			return results, true
		}

		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: true,
			Type:      &AcceleratorString,
			TypeIds:   accelerators,
			Scope:     scope,
		})
		return results, true
	}

	// HWThread -> HWThread
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: false,
			Type:      &HWThreadString,
			TypeIds:   hwthreadsStr,
			Scope:     scope,
		})
		return results, true
	}

	// HWThread -> Core
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeCore {
		cores, _ := topology.GetCoresFromHWThreads(hwthreads)
		for _, core := range cores {
			results = append(results, ScopeQueryResult{
				Metric:    metric,
				Hostname:  hostname,
				Aggregate: true,
				Type:      &HWThreadString,
				TypeIds:   IntToStringSlice(topology.Core[core]),
				Scope:     scope,
			})
		}
		return results, true
	}

	// HWThread -> Socket
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeSocket {
		sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
		for _, socket := range sockets {
			results = append(results, ScopeQueryResult{
				Metric:    metric,
				Hostname:  hostname,
				Aggregate: true,
				Type:      &HWThreadString,
				TypeIds:   IntToStringSlice(topology.Socket[socket]),
				Scope:     scope,
			})
		}
		return results, true
	}

	// HWThread -> Node
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeNode {
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: true,
			Type:      &HWThreadString,
			TypeIds:   hwthreadsStr,
			Scope:     scope,
		})
		return results, true
	}

	// Core -> Core
	if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeCore {
		cores, _ := topology.GetCoresFromHWThreads(hwthreads)
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: false,
			Type:      &CoreString,
			TypeIds:   IntToStringSlice(cores),
			Scope:     scope,
		})
		return results, true
	}

	// Core -> Socket
	if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeSocket {
		sockets, _ := topology.GetSocketsFromCores(hwthreads)
		for _, socket := range sockets {
			results = append(results, ScopeQueryResult{
				Metric:    metric,
				Hostname:  hostname,
				Aggregate: true,
				Type:      &CoreString,
				TypeIds:   IntToStringSlice(topology.Socket[socket]),
				Scope:     scope,
			})
		}
		return results, true
	}

	// Core -> Node
	if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeNode {
		cores, _ := topology.GetCoresFromHWThreads(hwthreads)
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: true,
			Type:      &CoreString,
			TypeIds:   IntToStringSlice(cores),
			Scope:     scope,
		})
		return results, true
	}

	// MemoryDomain -> MemoryDomain
	if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeMemoryDomain {
		memDomains, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: false,
			Type:      &MemoryDomainString,
			TypeIds:   IntToStringSlice(memDomains),
			Scope:     scope,
		})
		return results, true
	}

	// MemoryDomain -> Socket
	if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeSocket {
		memDomains, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
		socketToDomains, err := topology.GetMemoryDomainsBySocket(memDomains)
		if err != nil {
			cclog.Errorf("Error mapping memory domains to sockets, return unchanged: %v", err)
			// Rare Error Case -> Still Continue -> Return Empty Slice
			return results, true
		}

		// Create a query for each socket
		for _, domains := range socketToDomains {
			results = append(results, ScopeQueryResult{
				Metric:    metric,
				Hostname:  hostname,
				Aggregate: true,
				Type:      &MemoryDomainString,
				TypeIds:   IntToStringSlice(domains),
				Scope:     scope,
			})
		}
		return results, true
	}

	// MemoryDomain -> Node
	if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeNode {
		memDomains, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: true,
			Type:      &MemoryDomainString,
			TypeIds:   IntToStringSlice(memDomains),
			Scope:     scope,
		})
		return results, true
	}

	// Socket -> Socket
	if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeSocket {
		sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: false,
			Type:      &SocketString,
			TypeIds:   IntToStringSlice(sockets),
			Scope:     scope,
		})
		return results, true
	}

	// Socket -> Node
	if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeNode {
		sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
		results = append(results, ScopeQueryResult{
			Metric:    metric,
			Hostname:  hostname,
			Aggregate: true,
			Type:      &SocketString,
			TypeIds:   IntToStringSlice(sockets),
			Scope:     scope,
		})
		return results, true
	}

	// Node -> Node
	if nativeScope == schema.MetricScopeNode && scope == schema.MetricScopeNode {
		results = append(results, ScopeQueryResult{
			Metric:   metric,
			Hostname: hostname,
			Scope:    scope,
		})
		return results, true
	}

	// Unhandled Case
	return nil, false
}

// IntToStringSlice converts a slice of integers to a slice of strings.
// Used to convert hardware thread/core/socket IDs from topology (int) to query TypeIds (string).
// Optimized to reuse a byte buffer for string conversion, reducing allocations.
func IntToStringSlice(is []int) []string {
	if len(is) == 0 {
		return nil
	}

	ss := make([]string, len(is))
	buf := make([]byte, 0, 16) // Reusable buffer for integer conversion
	for i, x := range is {
		buf = strconv.AppendInt(buf[:0], int64(x), 10)
		ss[i] = string(buf)
	}
	return ss
}

// SanitizeStats replaces NaN values in statistics with 0 to enable JSON marshaling.
// If ANY of avg/min/max is NaN, ALL three are zeroed for consistency.
func SanitizeStats(avg, min, max *schema.Float) {
	if avg.IsNaN() || min.IsNaN() || max.IsNaN() {
		*avg = schema.Float(0)
		*min = schema.Float(0)
		*max = schema.Float(0)
	}
}
