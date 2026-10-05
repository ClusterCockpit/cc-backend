// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import ccfleet "github.com/ClusterCockpit/cc-lib/v2/fleet"

// The service type codes themselves are wire definitions shared with every
// fleet member and live in cc-lib (ccfleet.ServiceType, ccfleet.ServiceTypes).
// Which peers a consumer gets to discover is server policy and stays here.

// relevantProviders is the single source of truth for which provider service
// types each consumer type needs to discover. Keep it here and edit in one
// place.
//
// Confirmed universal edge: every service must reach cc-backend (ccb) to
// register and pull its config, so ccb is relevant to all of them. Richer
// peer edges (a collector wanting the metric store, an energy manager wanting
// the node controller, …) are left commented for an operator to enable once the
// concrete topology is settled — they are intentionally not assumed here.
var relevantProviders = map[ccfleet.ServiceType][]ccfleet.ServiceType{
	ccfleet.ServiceMetricStore:     {ccfleet.ServiceBackend},
	ccfleet.ServiceMetricCollector: {ccfleet.ServiceBackend /*, ccfleet.ServiceMetricStore */},
	ccfleet.ServiceEventStore:      {ccfleet.ServiceBackend},
	ccfleet.ServiceSlurmAdapter:    {ccfleet.ServiceBackend},
	ccfleet.ServiceNodeController:  {ccfleet.ServiceBackend},
	ccfleet.ServiceEnergyManager:   {ccfleet.ServiceBackend /*, ccfleet.ServiceMetricStore, ccfleet.ServiceNodeController */},
	ccfleet.ServiceBackend:         {}, // ccb discovers no peers by default
}

// RelevantProviders returns the provider service types that consumer should
// discover. The returned slice must not be mutated by callers.
func RelevantProviders(consumer ccfleet.ServiceType) []ccfleet.ServiceType {
	return relevantProviders[consumer]
}
