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

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
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
	// Initialize both slices together
	queries := make([]APIQuery, 0, len(metrics)*len(scopes)*len(job.Resources))
	assignedScope := make([]schema.MetricScope, 0, len(metrics)*len(scopes)*len(job.Resources))

	topology, err := ccms.getTopology(job.Cluster, job.SubCluster)
	if err != nil {
		cclog.Errorf("could not load cluster %s subCluster %s topology: %s", job.Cluster, job.SubCluster, err.Error())
		return nil, nil, err
	}

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

				scopeResults, ok := metricstore.BuildScopeQueries(
					nativeScope, requestedScope,
					remoteName, host.Hostname,
					topology, hwthreads, host.Accelerators,
				)

				if !ok {
					return nil, nil, fmt.Errorf("METRICDATA/EXTERNAL-CCMS > TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
				}

				for _, sr := range scopeResults {
					queries = append(queries, APIQuery{
						Metric:     sr.Metric,
						Hostname:   sr.Hostname,
						Aggregate:  sr.Aggregate,
						Type:       sr.Type,
						TypeIds:    sr.TypeIds,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, sr.Scope)
				}
			}
		}
	}

	return queries, assignedScope, nil
}

// buildNodeQueries constructs API queries for node-specific metric data (Systems View).
// Similar to buildQueries but uses full node topology instead of job-allocated resources.
//
// The function handles:
//   - SubCluster topology resolution (either pre-loaded or per-node lookup)
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
	// Initialize both slices together
	queries := make([]APIQuery, 0, len(metrics)*len(scopes)*len(nodes))
	assignedScope := make([]schema.MetricScope, 0, len(metrics)*len(scopes)*len(nodes))

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
				var topology *schema.Topology
				var err error

				// If no subCluster given, get it by node
				if subCluster == "" {
					topology, err = ccms.getTopologyByNode(cluster, hostname)
				} else {
					topology, err = ccms.getTopology(cluster, subCluster)
				}

				if err != nil {
					return nil, nil, err
				}

				// Always full node hwthread id list, no partial queries expected -> Use "topology.Node" directly where applicable
				// Always full accelerator id list, no partial queries expected -> Use "acceleratorIds" directly where applicable
				acceleratorIds := topology.GetAcceleratorIDs()

				// Moved check here if metric matches hardware specs
				if nativeScope == schema.MetricScopeAccelerator && len(acceleratorIds) == 0 {
					continue scopesLoop
				}

				scopeResults, ok := metricstore.BuildScopeQueries(
					nativeScope, requestedScope,
					remoteName, hostname,
					topology, topology.Node, acceleratorIds,
				)

				if !ok {
					return nil, nil, fmt.Errorf("METRICDATA/EXTERNAL-CCMS > TODO: unhandled case: native-scope=%s, requested-scope=%s", nativeScope, requestedScope)
				}

				for _, sr := range scopeResults {
					queries = append(queries, APIQuery{
						Metric:     sr.Metric,
						Hostname:   sr.Hostname,
						Aggregate:  sr.Aggregate,
						Type:       sr.Type,
						TypeIds:    sr.TypeIds,
						Resolution: resolution,
					})
					assignedScope = append(assignedScope, sr.Scope)
				}
			}
		}
	}

	return queries, assignedScope, nil
}

