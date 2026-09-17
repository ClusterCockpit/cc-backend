// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import "testing"

func TestServiceTypeValid(t *testing.T) {
	for _, st := range AllServiceTypes {
		if !st.Valid() {
			t.Errorf("%q should be valid", st)
		}
	}
	for _, bad := range []ServiceType{"", "bogus", "metric-store", "CCMS"} {
		if bad.Valid() {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestServiceTypeDescription(t *testing.T) {
	if got := ServiceTypeMetricStore.Description(); got != "cc-metric-store" {
		t.Errorf("ccms description = %q, want cc-metric-store", got)
	}
	// Unknown code falls back to the raw string.
	if got := ServiceType("bogus").Description(); got != "bogus" {
		t.Errorf("unknown description = %q, want bogus", got)
	}
}

func TestRelevantProviders(t *testing.T) {
	// Confirmed universal edge: every non-ccb service must discover ccb.
	for _, st := range AllServiceTypes {
		if st == ServiceTypeBackend {
			continue
		}
		rel := RelevantProviders(st)
		found := false
		for _, p := range rel {
			if p == ServiceTypeBackend {
				found = true
			}
		}
		if !found {
			t.Errorf("%q must have ccb as a relevant provider, got %v", st, rel)
		}
	}
	// ccb discovers no peers by default.
	if rel := RelevantProviders(ServiceTypeBackend); len(rel) != 0 {
		t.Errorf("ccb should have no relevant providers, got %v", rel)
	}
}
