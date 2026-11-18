// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"

	"github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

func TestDecodeLineRoutesEachResolvedClusterAndHost(t *testing.T) {
	previous := Keys.Checkpoints
	Keys.Checkpoints.FileFormat = "json"
	t.Cleanup(func() { Keys.Checkpoints = previous })
	tests := []struct {
		name, input, cluster, host string
	}{
		{
			name:    "cluster falls back after explicit tag",
			input:   "audit,cluster=tagged,hostname=node value=11 1700000000\naudit,hostname=node value=22 1700000010\n",
			cluster: "default", host: "node",
		},
		{
			name:    "host is absent after explicit tag",
			input:   "audit,hostname=node value=11 1700000000\naudit value=22 1700000010\n",
			cluster: "default", host: "",
		},
		{
			name:    "both tags fall back",
			input:   "audit,cluster=tagged,hostname=node value=11 1700000000\naudit value=22 1700000010\n",
			cluster: "default", host: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &MemoryStore{Metrics: map[string]MetricConfig{"audit": {Frequency: 10}}}
			if err := DecodeLine(lineprotocol.NewDecoderWithBytes([]byte(test.input)), store, "default"); err != nil {
				t.Fatal(err)
			}
			cluster := store.root.children[test.cluster]
			if cluster == nil || cluster.children[test.host] == nil {
				t.Fatalf("second sample was not routed to %q/%q", test.cluster, test.host)
			}
			buffer := cluster.children[test.host].metrics[0]
			if buffer == nil || buffer.firstWrite() != 1700000010 || len(buffer.data) != 1 || buffer.data[0] != 22 {
				t.Fatalf("unexpected second sample at %q/%q: %#v", test.cluster, test.host, buffer)
			}
		})
	}
}
