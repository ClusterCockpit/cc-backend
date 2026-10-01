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
	FilesystemString   = string(schema.MetricScopeFilesystem)
	NetworkString      = string(schema.MetricScopeNetwork)
)

// deviceTypeString returns the pre-converted query type of a device scope, which
// is also the level-key prefix the metric store uses for that device's data.
func deviceTypeString(scope schema.MetricScope) *string {
	switch scope {
	case schema.MetricScopeAccelerator:
		return &AcceleratorString
	case schema.MetricScopeFilesystem:
		return &FilesystemString
	case schema.MetricScopeNetwork:
		return &NetworkString
	default:
		return nil
	}
}

// DeviceIDs returns the instance ids to query for a metric of the given native
// scope on one host of a job: the accelerators allocated to the job for
// accelerator metrics, the ids declared in the topology for filesystem and
// network metrics, and nil for CPU and node scopes.
func DeviceIDs(nativeScope schema.MetricScope, topology *schema.Topology, allocatedAccelerators []string) []string {
	if nativeScope == schema.MetricScopeAccelerator {
		return allocatedAccelerators
	}
	return topology.GetDeviceIDs(nativeScope)
}

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
//
// deviceIDs are the instances of a device-native metric (accelerator,
// filesystem, network) on this host; see DeviceIDs. Device metrics are only
// available at their own scope or aggregated to node scope, and a device scope
// requested for a CPU-native metric yields no data rather than falling back to
// the native scope.
func BuildScopeQueries(
	nativeScope, requestedScope schema.MetricScope,
	metric, hostname string,
	topology *schema.Topology,
	hwthreads []int,
	deviceIDs []string,
) ([]ScopeQueryResult, bool) {
	results := []ScopeQueryResult{}

	// Device -> Device / Node
	if nativeScope.IsDevice() {
		if len(deviceIDs) == 0 {
			// Expected Exception -> Return Empty Slice
			return results, true
		}

		switch requestedScope {
		case nativeScope:
			results = append(results, ScopeQueryResult{
				Metric:    metric,
				Hostname:  hostname,
				Aggregate: false,
				Type:      deviceTypeString(nativeScope),
				TypeIds:   deviceIDs,
				Scope:     nativeScope,
			})
		case schema.MetricScopeNode:
			results = append(results, ScopeQueryResult{
				Metric:    metric,
				Hostname:  hostname,
				Aggregate: true,
				Type:      deviceTypeString(nativeScope),
				TypeIds:   deviceIDs,
				Scope:     schema.MetricScopeNode,
			})
		}
		// Any other requested scope: Expected Exception -> Return Empty Slice
		return results, true
	}

	// CPU/Node -> Device: Expected Exception -> Return Empty Slice
	if requestedScope.IsDevice() {
		return results, true
	}

	scope := nativeScope.Max(requestedScope)
	hwthreadsStr := IntToStringSlice(hwthreads)

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

// ExtractTypeID returns the type ID at the given index from a query's TypeIds slice.
// Returns nil if queryType is nil (no type filtering). Logs a warning and returns nil
// if the index is out of range.
func ExtractTypeID(queryType *string, typeIds []string, ndx int, metric, hostname string) *string {
	if queryType == nil {
		return nil
	}
	if ndx < len(typeIds) {
		id := typeIds[ndx]
		return &id
	}
	cclog.Warnf("TypeIds index out of range: %d with length %d for metric %s on host %s",
		ndx, len(typeIds), metric, hostname)
	return nil
}

// IsMetricRemovedForSubCluster checks whether a metric is marked as removed
// for the given subcluster in its per-subcluster configuration.
func IsMetricRemovedForSubCluster(mc *schema.MetricConfig, subCluster string) bool {
	for _, scConfig := range mc.SubClusters {
		if scConfig.Name == subCluster && scConfig.Remove {
			return true
		}
	}
	return false
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
