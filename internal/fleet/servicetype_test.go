// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"testing"

	ccfleet "github.com/ClusterCockpit/cc-lib/v2/fleet"
)

func TestRelevantProvidersCoversEveryServiceType(t *testing.T) {
	// A service type added to cc-lib must get an explicit routing decision.
	for _, st := range ccfleet.ServiceTypes {
		if _, ok := relevantProviders[st]; !ok {
			t.Errorf("%q has no entry in relevantProviders", st)
		}
	}
	for st := range relevantProviders {
		if !st.Valid() {
			t.Errorf("relevantProviders lists unknown service type %q", st)
		}
	}
}

func TestRelevantProviders(t *testing.T) {
	// Confirmed universal edge: every non-ccb service must discover ccb.
	for _, st := range ccfleet.ServiceTypes {
		if st == ccfleet.ServiceBackend {
			continue
		}
		rel := RelevantProviders(st)
		found := false
		for _, p := range rel {
			if p == ccfleet.ServiceBackend {
				found = true
			}
		}
		if !found {
			t.Errorf("%q must have ccb as a relevant provider, got %v", st, rel)
		}
	}
	// ccb discovers no peers by default.
	if rel := RelevantProviders(ccfleet.ServiceBackend); len(rel) != 0 {
		t.Errorf("ccb should have no relevant providers, got %v", rel)
	}
}
