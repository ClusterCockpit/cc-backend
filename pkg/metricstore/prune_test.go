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

// writeNode creates cluster/node (and optional deeper sub-levels) and fills
// every metric buffer with points from startTs to endTs (inclusive) at the
// store's metric frequency. subPath appends levels below the node (e.g.
// {"socket0"}); pass nil to write buffers directly on the node level.
func writeNode(ms *MemoryStore, cluster, node string, subPath []string, freq, startTs, endTs int64) {
	selector := append([]string{cluster, node}, subPath...)
	lvl := ms.root.findLevelOrCreate(selector, len(ms.Metrics))
	for i := range lvl.metrics {
		lvl.metrics[i] = newBuffer(startTs, freq)
		for ts := startTs; ts <= endTs; ts += freq {
			lvl.metrics[i].write(ts, schema.Float(1))
		}
	}
}

// hasNode reports whether [cluster, node] still exists in the tree.
func hasNode(ms *MemoryStore, cluster, node string) bool {
	return slices.Contains(ms.ListChildren([]string{cluster}), node)
}

func TestFreeExcludingUsedPrunesDeadNode(t *testing.T) {
	ms := newTestStore() // one metric "cpu_load", freq 60
	const freq, thr = int64(60), int64(100000)

	// dead: last data ~2000, well below threshold -> emptied -> pruned
	writeNode(ms, "fritz", "dead", nil, freq, 1000, 2000)
	// alive: data around/after threshold -> stays
	writeNode(ms, "fritz", "alive", nil, freq, thr, thr+600)

	freed, err := ms.root.freeExcludingUsed(thr, nil)
	if err != nil {
		t.Fatalf("freeExcludingUsed: %v", err)
	}
	if freed == 0 {
		t.Fatal("expected at least one buffer freed")
	}
	if hasNode(ms, "fritz", "dead") {
		t.Error("dead node must be pruned from the tree")
	}
	if !hasNode(ms, "fritz", "alive") {
		t.Error("alive node must be preserved")
	}
}

func TestFreeExcludingUsedPreservesUsedNode(t *testing.T) {
	ms := newTestStore()
	const freq, thr = int64(60), int64(100000)

	// both would be dead by timestamp, but "used" is excluded
	writeNode(ms, "fritz", "used", nil, freq, 1000, 2000)
	writeNode(ms, "fritz", "gone", nil, freq, 1000, 2000)

	used := map[string][]string{"fritz": {"used"}} // sorted hostnames

	if _, err := ms.root.freeExcludingUsed(thr, used); err != nil {
		t.Fatalf("freeExcludingUsed: %v", err)
	}
	if !hasNode(ms, "fritz", "used") {
		t.Error("used node must be preserved even when stale")
	}
	if hasNode(ms, "fritz", "gone") {
		t.Error("non-used dead node must be pruned")
	}
	// used node still holds its buffer
	lvl := ms.root.findLevel([]string{"fritz", "used"})
	if lvl == nil || lvl.metrics[0] == nil {
		t.Error("used node must keep its buffers")
	}
}

func TestFreeExcludingUsedPrunesSubLevelNode(t *testing.T) {
	ms := newTestStore()
	const freq, thr = int64(60), int64(100000)

	// node holds no direct buffers; only a socket0 child, all stale
	writeNode(ms, "fritz", "deep", []string{"socket0"}, freq, 1000, 2000)

	if _, err := ms.root.freeExcludingUsed(thr, nil); err != nil {
		t.Fatalf("freeExcludingUsed: %v", err)
	}
	if hasNode(ms, "fritz", "deep") {
		t.Error("node must be pruned once all descendant buffers are freed")
	}
}
