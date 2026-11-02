// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestRestoredCheckpointIncludesAcceptedOverwrite(t *testing.T) {
	ms := newDeviceTestStore()
	cf := &CheckpointFile{Metrics: map[string]*CheckpointMetrics{
		"fs_read_bw": {Frequency: 60, Start: 90, Data: []schema.Float{1, 2}},
	}}
	if err := ms.root.loadFile(cf, ms); err != nil {
		t.Fatal(err)
	}
	unchanged, err := ms.root.toCheckpointFile(120, 240, ms)
	if err != nil || unchanged != nil {
		t.Fatalf("unchanged checkpoint: %v, %v", unchanged, err)
	}
	if err := ms.Write(nil, 180, []Metric{{Name: "fs_read_bw", Value: 42}}); err != nil {
		t.Fatal(err)
	}
	changed, err := ms.root.toCheckpointFile(120, 240, ms)
	if err != nil {
		t.Fatal(err)
	}
	if changed == nil || changed.Metrics["fs_read_bw"] == nil {
		t.Fatal("accepted overwrite disappeared from the next checkpoint")
	}
	data := changed.Metrics["fs_read_bw"].Data
	if len(data) != 2 || data[0] != 1 || data[1] != 42 {
		t.Fatalf("checkpoint data = %v, want [1 42]", data)
	}
}

func TestArchivedBufferDirtyOnlyAfterAcceptedMutation(t *testing.T) {
	b := newBuffer(120, 60)
	if _, err := b.write(120, 1); err != nil {
		t.Fatal(err)
	}
	b.archived = true
	if _, err := b.write(0, 9); err == nil || !b.archived {
		t.Fatal("rejected write must preserve the archived state")
	}
	if _, err := b.write(180, 2); err != nil {
		t.Fatal(err)
	}
	if b.archived {
		t.Fatal("accepted append remained marked archived")
	}
}
