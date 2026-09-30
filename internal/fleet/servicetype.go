// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

// ServiceType is the kind of cc-* service a fleet member is. It is a
// string-backed enum (mirroring schema.MonitoringState and the local
// ScopeCluster/ScopeInfra constants): the short code is the canonical value
// stored in the DB, used as the config-tree directory name, and used as a NATS
// discovery-subject token. Description() gives the human-readable name.
type ServiceType string

const (
	ServiceTypeMetricStore    ServiceType = "ccms" // cc-metric-store
	ServiceTypeCollector      ServiceType = "ccmc" // cc-metric-collector
	ServiceTypeBackend        ServiceType = "ccb"  // cc-backend
	ServiceTypeEventStore     ServiceType = "cces" // cc-event-store
	ServiceTypeSlurmAdapter   ServiceType = "ccsa" // cc-slurm-adapter
	ServiceTypeNodeController ServiceType = "ccnc" // cc-node-controller
	ServiceTypeEnergyManager  ServiceType = "ccem" // cc-energy-manager
)

// AllServiceTypes lists every valid service type. Order is stable so callers
// that iterate (e.g. the discovery publisher) produce deterministic output.
var AllServiceTypes = []ServiceType{
	ServiceTypeMetricStore,
	ServiceTypeCollector,
	ServiceTypeBackend,
	ServiceTypeEventStore,
	ServiceTypeSlurmAdapter,
	ServiceTypeNodeController,
	ServiceTypeEnergyManager,
}

var serviceTypeDescriptions = map[ServiceType]string{
	ServiceTypeMetricStore:    "cc-metric-store",
	ServiceTypeCollector:      "cc-metric-collector",
	ServiceTypeBackend:        "cc-backend",
	ServiceTypeEventStore:     "cc-event-store",
	ServiceTypeSlurmAdapter:   "cc-slurm-adapter",
	ServiceTypeNodeController: "cc-node-controller",
	ServiceTypeEnergyManager:  "cc-energy-manager",
}

// Valid reports whether t is a known service type.
func (t ServiceType) Valid() bool {
	_, ok := serviceTypeDescriptions[t]
	return ok
}

// Description returns the full human-readable service name (e.g. "cc-metric-store"),
// or the raw code if unknown.
func (t ServiceType) Description() string {
	if d, ok := serviceTypeDescriptions[t]; ok {
		return d
	}
	return string(t)
}

// relevantProviders is the single source of truth for which provider service
// types each consumer type needs to discover. Keep it here and edit in one
// place.
//
// Confirmed universal edge: every service must reach cc-backend (ccb) to
// register and pull its config, so ccb is relevant to all of them. Richer
// peer edges (a collector wanting the metric store, an energy manager wanting
// the node controller, …) are left commented for an operator to enable once the
// concrete topology is settled — they are intentionally not assumed here.
var relevantProviders = map[ServiceType][]ServiceType{
	ServiceTypeMetricStore:    {ServiceTypeBackend},
	ServiceTypeCollector:      {ServiceTypeBackend /*, ServiceTypeMetricStore */},
	ServiceTypeEventStore:     {ServiceTypeBackend},
	ServiceTypeSlurmAdapter:   {ServiceTypeBackend},
	ServiceTypeNodeController: {ServiceTypeBackend},
	ServiceTypeEnergyManager:  {ServiceTypeBackend /*, ServiceTypeMetricStore, ServiceTypeNodeController */},
	ServiceTypeBackend:        {}, // ccb discovers no peers by default
}

// RelevantProviders returns the provider service types that consumer should
// discover. The returned slice must not be mutated by callers.
func RelevantProviders(consumer ServiceType) []ServiceType {
	return relevantProviders[consumer]
}
