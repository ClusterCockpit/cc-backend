// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

func newDeviceTestStore() *MemoryStore {
	return &MemoryStore{
		Metrics: map[string]MetricConfig{"fs_read_bw": {Frequency: 60, Aggregation: SumAggregation, offset: 0}},
		root:    Level{metrics: make([]*buffer, 1), children: make(map[string]*Level)},
	}
}

// ingestDeviceLines writes fs_read_bw = 1 for /home and 2 for /scratch on host
// h1, tagged with tags (which must leave the device id last), and returns the
// time of the last sample.
func ingestDeviceLines(t *testing.T, ms *MemoryStore, tags string) int64 {
	t.Helper()
	now := time.Now().Unix() / 60 * 60
	var b strings.Builder
	for ts := now - 300; ts <= now; ts += 60 {
		fmt.Fprintf(&b, "fs_read_bw,cluster=c,hostname=h1,%s/home value=1 %d\n", tags, ts)
		fmt.Fprintf(&b, "fs_read_bw,cluster=c,hostname=h1,%s/scratch value=2 %d\n", tags, ts)
	}
	if err := DecodeLine(lineprotocol.NewDecoderWithBytes([]byte(b.String())), ms, "c"); err != nil {
		t.Fatalf("DecodeLine: %v", err)
	}
	return now
}

// TestDeviceTagsReachDeviceQueries pins the collector convention for device
// metrics: type=<device>,type-id=<id>, as for accelerators. The queries built
// by BuildScopeQueries must read each instance and sum them at node scope.
func TestDeviceTagsReachDeviceQueries(t *testing.T) {
	ms := newDeviceTestStore()
	now := ingestDeviceLines(t, ms, "type=filesystem,type-id=")

	topo := makeTopology()
	ids := []string{"/home", "/scratch"}
	read := func(requested schema.MetricScope) [][]schema.Float {
		results, ok := BuildScopeQueries(schema.MetricScopeFilesystem, requested,
			"fs_read_bw", "h1", &topo, topo.Node, ids)
		if !ok || len(results) != 1 {
			t.Fatalf("BuildScopeQueries(%s) = %+v, %v", requested, results, ok)
		}
		r := results[0]
		q := APIQuery{Metric: r.Metric, Hostname: r.Hostname, Type: r.Type, TypeIds: r.TypeIds, Aggregate: r.Aggregate}
		var series [][]schema.Float
		for _, sel := range querySelectors("c", q) {
			data, _, _, _, err := ms.Read(sel, "fs_read_bw", now-240, now, 60, "")
			if err != nil {
				t.Fatalf("Read(%v): %v", sel, err)
			}
			series = append(series, data)
		}
		return series
	}

	perMount := read(schema.MetricScopeFilesystem)
	if len(perMount) != 2 {
		t.Fatalf("got %d filesystem series, want 2", len(perMount))
	}
	for i, want := range []schema.Float{1, 2} {
		if len(perMount[i]) == 0 || perMount[i][0] != want {
			t.Errorf("%s series = %v, want %v", ids[i], perMount[i], want)
		}
	}

	node := read(schema.MetricScopeNode)
	if len(node) != 1 || len(node[0]) == 0 || node[0][0] != 3 {
		t.Errorf("node series = %v, want the sum 3", node)
	}
}

// TestDeviceSubtypeTagsUnderNodeAreNotDeviceData documents why the stype form
// is not the convention: below type=node the decoder ignores stype/stype-id
// and writes into the host buffer, where the instances overwrite each other.
func TestDeviceSubtypeTagsUnderNodeAreNotDeviceData(t *testing.T) {
	ms := newDeviceTestStore()
	ingestDeviceLines(t, ms, "type=node,stype=filesystem,stype-id=")

	host := ms.root.findLevel([]string{"c", "h1"})
	if host == nil {
		t.Fatal("host level not created")
	}
	if len(host.children) != 0 {
		t.Errorf("stype under type=node created device levels %v; update the documented collector convention", host.children)
	}
	if host.metrics[0] == nil {
		t.Error("expected the samples in the host buffer")
	}
}
