// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstoreclient - Query Building
//
// This file contains the query construction and scope transformation logic for cc-metric-store queries.
// It handles the complex mapping between requested metric scopes and native hardware topology,
// automatically aggregating or filtering metrics as needed.
//
// # Scope Transformations
//
// The buildScopeQueries function implements the core scope transformation algorithm.
// It handles 25+ different transformation cases, mapping between:
//   - Accelerator (GPU) scope
//   - HWThread (hardware thread/SMT) scope
//   - Core (CPU core) scope
//   - Socket (CPU package) scope
//   - MemoryDomain (NUMA domain) scope
//   - Node (full system) scope
//
// Transformations follow these rules:
//   - Same scope: Return data as-is (e.g., Core → Core)
//   - Coarser scope: Aggregate data (e.g., Core → Socket with Aggregate=true)
//   - Finer scope: Error - cannot increase granularity
//
// # Query Building
//
// buildQueries and buildNodeQueries are the main entry points, handling job-specific
// and node-specific query construction respectively. They:
//   - Validate metric configurations
//   - Handle subcluster-specific metric filtering
//   - Detect and skip duplicate scope requests
//   - Call buildScopeQueries for each metric/scope/host combination
package metricstoreclient

import (
	"fmt"
	"strconv"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Scope string constants used in API queries.
// Pre-converted to avoid repeated allocations during query building.
var (
	hwthreadString     = string(schema.MetricScopeHWThread)
	coreString         = string(schema.MetricScopeCore)
	memoryDomainString = string(schema.MetricScopeMemoryDomain)
	socketString       = string(schema.MetricScopeSocket)
	acceleratorString  = string(schema.MetricScopeAccelerator)
)

// buildQueries constructs API queries for job-specific metric data.
// It iterates through metrics, scopes, and job resources to build the complete query set.
//
// The function handles:
//   - Metric configuration validation and subcluster filtering
//   - Scope deduplication to avoid redundant queries
//   - Hardware thread list resolution (job-allocated vs full node)
//   - Delegation to buildScopeQueries for scope transformations
//
// Returns queries and their corresponding assigned scopes (which may differ from requested scopes).
func (ccms *CCMetricStore) buildQueries(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
) ([]APIQuery, []schema.MetricScope, error) {
	queries := make([]APIQuery, 0, len(metrics)*len(scopes)*len(job.Resources))
	assignedScope := []schema.MetricScope{}

	subcluster, scerr := archive.GetSubCluster(job.Cluster, job.SubCluster)
	if scerr != nil {
		return nil, nil, scerr
	}
	topology := subcluster.Topology

	for _, metric := range metrics {
		remoteName := metric
		mc := archive.GetMetricConfig(job.Cluster, metric)
		if mc == nil {
			cclog.Warnf("metric '%s' is not specified for cluster '%s' - skipping", metric, job.Cluster)
			continue
		}

		// Skip if metric is removed for subcluster
		if len(mc.SubClusters) != 0 {
			isRemoved := false
			for _, scConfig := range mc.SubClusters {
				if scConfig.Name == job.SubCluster && scConfig.Remove {
					isRemoved = true
					break
				}
			}
			if isRemoved {
				continue
			}
		}

		// Avoid duplicates...
		handledScopes := make([]schema.MetricScope, 0, 3)

	scopesLoop:
		for _, requestedScope := range scopes {
			nativeScope := mc.Scope
			if nativeScope == schema.MetricScopeAccelerator && job.NumAcc == 0 {
				continue
			}

			scope := nativeScope.Max(requestedScope)
			for _, s := range handledScopes {
				if scope == s {
					continue scopesLoop
				}
			}
			handledScopes = append(handledScopes, scope)

			for _, host := range job.Resources {
				hwthreads := host.HWThreads
				if hwthreads == nil {
					hwthreads = topology.Node
				}

				hostQueries, hostScopes := buildScopeQueries(
					nativeScope, requestedScope,
					remoteName, host.Hostname,
					&topology, hwthreads, host.Accelerators,
					resolution,
				)

				if len(hostQueries) == 0 && len(hostScopes) == 0 {
					return nil, nil, fmt.Errorf("METRICDATA/CCMS > TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
				}

				queries = append(queries, hostQueries...)
				assignedScope = append(assignedScope, hostScopes...)
			}
		}
	}

	return queries, assignedScope, nil
}

// buildNodeQueries constructs API queries for node-specific metric data (Systems View).
// Similar to buildQueries but uses full node topology instead of job-allocated resources.
//
// The function handles:
//   - Subcluster topology resolution (either pre-loaded or per-node lookup)
//   - Full node hardware thread lists (not job-specific subsets)
//   - All accelerators on each node
//   - Metric configuration validation with subcluster filtering
//
// Returns queries and their corresponding assigned scopes.
func (ccms *CCMetricStore) buildNodeQueries(
	cluster string,
	subCluster string,
	nodes []string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
) ([]APIQuery, []schema.MetricScope, error) {
	queries := make([]APIQuery, 0, len(metrics)*len(scopes)*len(nodes))
	assignedScope := []schema.MetricScope{}

	// Get Topol before loop if subCluster given
	var subClusterTopol *schema.SubCluster
	var scterr error
	if subCluster != "" {
		subClusterTopol, scterr = archive.GetSubCluster(cluster, subCluster)
		if scterr != nil {
			cclog.Errorf("could not load cluster %s subCluster %s topology: %s", cluster, subCluster, scterr.Error())
			return nil, nil, scterr
		}
	}

	for _, metric := range metrics {
		remoteName := metric
		mc := archive.GetMetricConfig(cluster, metric)
		if mc == nil {
			cclog.Warnf("metric '%s' is not specified for cluster '%s'", metric, cluster)
			continue
		}

		// Skip if metric is removed for subcluster
		if mc.SubClusters != nil {
			isRemoved := false
			for _, scConfig := range mc.SubClusters {
				if scConfig.Name == subCluster && scConfig.Remove {
					isRemoved = true
					break
				}
			}
			if isRemoved {
				continue
			}
		}

		// Avoid duplicates...
		handledScopes := make([]schema.MetricScope, 0, 3)

	scopesLoop:
		for _, requestedScope := range scopes {
			nativeScope := mc.Scope

			scope := nativeScope.Max(requestedScope)
			for _, s := range handledScopes {
				if scope == s {
					continue scopesLoop
				}
			}
			handledScopes = append(handledScopes, scope)

			for _, hostname := range nodes {

				// If no subCluster given, get it by node
				if subCluster == "" {
					subClusterName, scnerr := archive.GetSubClusterByNode(cluster, hostname)
					if scnerr != nil {
						return nil, nil, scnerr
					}
					subClusterTopol, scterr = archive.GetSubCluster(cluster, subClusterName)
					if scterr != nil {
						return nil, nil, scterr
					}
				}

				// Always full node hwthread id list, no partial queries expected -> Use "topology.Node" directly where applicable
				// Always full accelerator id list, no partial queries expected -> Use "acceleratorIds" directly where applicable
				topology := subClusterTopol.Topology
				acceleratorIds := topology.GetAcceleratorIDs()

				// Moved check here if metric matches hardware specs
				if nativeScope == schema.MetricScopeAccelerator && len(acceleratorIds) == 0 {
					continue scopesLoop
				}

				nodeQueries, nodeScopes := buildScopeQueries(
					nativeScope, requestedScope,
					remoteName, hostname,
					&topology, topology.Node, acceleratorIds,
					resolution,
				)

				if len(nodeQueries) == 0 && len(nodeScopes) == 0 {
					return nil, nil, fmt.Errorf("METRICDATA/CCMS > TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
				}

				queries = append(queries, nodeQueries...)
				assignedScope = append(assignedScope, nodeScopes...)
			}
		}
	}

	return queries, assignedScope, nil
}

// buildScopeQueries generates API queries for a given scope transformation.
// It returns a slice of queries and corresponding assigned scopes.
// Some transformations (e.g., HWThread -> Core/Socket) may generate multiple queries.
func buildScopeQueries(
	nativeScope, requestedScope schema.MetricScope,
	metric, hostname string,
	topology *schema.Topology,
	hwthreads []int,
	accelerators []string,
	resolution int,
) ([]APIQuery, []schema.MetricScope) {
	scope := nativeScope.Max(requestedScope)
	queries := []APIQuery{}
	scopes := []schema.MetricScope{}

	hwthreadsStr := intToStringSlice(hwthreads)

	// Accelerator -> Accelerator (Use "accelerator" scope if requested scope is lower than node)
	if nativeScope == schema.MetricScopeAccelerator && scope.LT(schema.MetricScopeNode) {
		if scope != schema.MetricScopeAccelerator {
			// Skip all other caught cases
			return queries, scopes
		}

		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  false,
			Type:       &acceleratorString,
			TypeIds:    accelerators,
			Resolution: resolution,
		})
		scopes = append(scopes, schema.MetricScopeAccelerator)
		return queries, scopes
	}

	// Accelerator -> Node
	if nativeScope == schema.MetricScopeAccelerator && scope == schema.MetricScopeNode {
		if len(accelerators) == 0 {
			return queries, scopes
		}

		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  true,
			Type:       &acceleratorString,
			TypeIds:    accelerators,
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// HWThread -> HWThread
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeHWThread {
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  false,
			Type:       &hwthreadString,
			TypeIds:    hwthreadsStr,
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// HWThread -> Core
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeCore {
		cores, _ := topology.GetCoresFromHWThreads(hwthreads)
		for _, core := range cores {
			queries = append(queries, APIQuery{
				Metric:     metric,
				Hostname:   hostname,
				Aggregate:  true,
				Type:       &hwthreadString,
				TypeIds:    intToStringSlice(topology.Core[core]),
				Resolution: resolution,
			})
			scopes = append(scopes, scope)
		}
		return queries, scopes
	}

	// HWThread -> Socket
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeSocket {
		sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
		for _, socket := range sockets {
			queries = append(queries, APIQuery{
				Metric:     metric,
				Hostname:   hostname,
				Aggregate:  true,
				Type:       &hwthreadString,
				TypeIds:    intToStringSlice(topology.Socket[socket]),
				Resolution: resolution,
			})
			scopes = append(scopes, scope)
		}
		return queries, scopes
	}

	// HWThread -> Node
	if nativeScope == schema.MetricScopeHWThread && scope == schema.MetricScopeNode {
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  true,
			Type:       &hwthreadString,
			TypeIds:    hwthreadsStr,
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// Core -> Core
	if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeCore {
		cores, _ := topology.GetCoresFromHWThreads(hwthreads)
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  false,
			Type:       &coreString,
			TypeIds:    intToStringSlice(cores),
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// Core -> Socket
	if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeSocket {
		sockets, _ := topology.GetSocketsFromCores(hwthreads)
		for _, socket := range sockets {
			queries = append(queries, APIQuery{
				Metric:     metric,
				Hostname:   hostname,
				Aggregate:  true,
				Type:       &coreString,
				TypeIds:    intToStringSlice(topology.Socket[socket]),
				Resolution: resolution,
			})
			scopes = append(scopes, scope)
		}
		return queries, scopes
	}

	// Core -> Node
	if nativeScope == schema.MetricScopeCore && scope == schema.MetricScopeNode {
		cores, _ := topology.GetCoresFromHWThreads(hwthreads)
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  true,
			Type:       &coreString,
			TypeIds:    intToStringSlice(cores),
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// MemoryDomain -> MemoryDomain
	if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeMemoryDomain {
		memDomains, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  false,
			Type:       &memoryDomainString,
			TypeIds:    intToStringSlice(memDomains),
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// MemoryDomain -> Node
	if nativeScope == schema.MetricScopeMemoryDomain && scope == schema.MetricScopeNode {
		memDomains, _ := topology.GetMemoryDomainsFromHWThreads(hwthreads)
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  true,
			Type:       &memoryDomainString,
			TypeIds:    intToStringSlice(memDomains),
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// Socket -> Socket
	if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeSocket {
		sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  false,
			Type:       &socketString,
			TypeIds:    intToStringSlice(sockets),
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// Socket -> Node
	if nativeScope == schema.MetricScopeSocket && scope == schema.MetricScopeNode {
		sockets, _ := topology.GetSocketsFromHWThreads(hwthreads)
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Aggregate:  true,
			Type:       &socketString,
			TypeIds:    intToStringSlice(sockets),
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// Node -> Node
	if nativeScope == schema.MetricScopeNode && scope == schema.MetricScopeNode {
		queries = append(queries, APIQuery{
			Metric:     metric,
			Hostname:   hostname,
			Resolution: resolution,
		})
		scopes = append(scopes, scope)
		return queries, scopes
	}

	// Unhandled case - return empty slices
	return queries, scopes
}

// intToStringSlice converts a slice of integers to a slice of strings.
// Used to convert hardware IDs (core IDs, socket IDs, etc.) to the string format required by the API.
func intToStringSlice(is []int) []string {
	ss := make([]string, len(is))
	for i, x := range is {
		ss[i] = strconv.Itoa(x)
	}
	return ss
}
