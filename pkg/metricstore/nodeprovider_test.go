// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"
)

// fakeNodeProvider implements NodeProvider for tests.
type fakeNodeProvider struct {
	nodes map[string][]string
	err   error
}

func (f *fakeNodeProvider) GetUsedNodes(ts int64) (map[string][]string, error) {
	return f.nodes, f.err
}

func TestIsNodeUsed(t *testing.T) {
	used := map[string][]string{"fritz": {"node001", "node003"}}

	cases := []struct {
		cluster, host string
		want          bool
	}{
		{"fritz", "node001", true},
		{"fritz", "node003", true},
		{"fritz", "node002", false},
		{"alex", "node001", false},
	}
	for _, c := range cases {
		if got := isNodeUsed(used, c.cluster, c.host); got != c.want {
			t.Errorf("isNodeUsed(%q, %q) = %v, want %v", c.cluster, c.host, got, c.want)
		}
	}

	if isNodeUsed(nil, "fritz", "node001") {
		t.Error("nil map must report false")
	}
	if isNodeUsed(map[string][]string{}, "fritz", "node001") {
		t.Error("empty map must report false")
	}
}
