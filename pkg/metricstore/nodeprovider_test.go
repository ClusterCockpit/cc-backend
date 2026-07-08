// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// fakeNodeProvider implements NodeProvider for tests.
type fakeNodeProvider struct {
	nodes map[string][]string
	err   error
}

func (f *fakeNodeProvider) GetUsedNodes(ts int64) (map[string][]string, error) {
	return f.nodes, f.err
}

var errTestProvider = errors.New("provider failure")

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

// newTestStore builds a minimal MemoryStore without touching the singleton.
func newTestStore() *MemoryStore {
	return &MemoryStore{
		Metrics: map[string]MetricConfig{
			"cpu_load": {Frequency: 60, offset: 0},
		},
		root: Level{
			metrics:  make([]*buffer, 1),
			children: make(map[string]*Level),
		},
	}
}

// writeTestCheckpoint writes a valid JSON checkpoint file <ts>.json into dir.
func writeTestCheckpoint(t *testing.T, dir string, ts int64) {
	t.Helper()
	cf := &CheckpointFile{
		From: ts,
		To:   ts + 120,
		Metrics: map[string]*CheckpointMetrics{
			"cpu_load": {Frequency: 60, Start: ts, Data: []schema.Float{1.0, 2.0, 3.0}},
		},
		Children: make(map[string]*CheckpointFile),
	}
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", ts)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeThreeCheckpointsPerHost creates ts 1000/2000/5000 for node001+node002.
func writeThreeCheckpointsPerHost(t *testing.T, dir string) {
	t.Helper()
	for _, host := range []string{"node001", "node002"} {
		hostDir := filepath.Join(dir, "fritz", host)
		writeTestCheckpoint(t, hostDir, 1000)
		writeTestCheckpoint(t, hostDir, 2000)
		writeTestCheckpoint(t, hostDir, 5000)
	}
}

func TestFromCheckpointLoadsAllFilesForUsedNodes(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	dir := t.TempDir()
	writeThreeCheckpointsPerHost(t, dir)

	ms := newTestStore()
	ms.SetNodeProvider(&fakeNodeProvider{nodes: map[string][]string{"fritz": {"node001"}}})

	n, err := ms.FromCheckpoint(dir, 3000)
	if err != nil {
		t.Fatal(err)
	}
	// node001 (used): all 3 files. node002: bridge 2000.json + 5000.json = 2.
	if n != 5 {
		t.Fatalf("expected 5 files loaded, got %d", n)
	}
}

func TestFromCheckpointWithoutProviderKeepsCutoff(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	dir := t.TempDir()
	writeThreeCheckpointsPerHost(t, dir)

	ms := newTestStore()

	n, err := ms.FromCheckpoint(dir, 3000)
	if err != nil {
		t.Fatal(err)
	}
	// 2 files per host, no provider set.
	if n != 4 {
		t.Fatalf("expected 4 files loaded, got %d", n)
	}
}

func TestFromCheckpointProviderErrorFallsBack(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	dir := t.TempDir()
	writeThreeCheckpointsPerHost(t, dir)

	ms := newTestStore()
	ms.SetNodeProvider(&fakeNodeProvider{err: errTestProvider})

	n, err := ms.FromCheckpoint(dir, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("expected fallback to cutoff load (4 files), got %d", n)
	}
}

func TestDeleteCheckpointsSkipsUsedNodes(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	dir := t.TempDir()
	writeTestCheckpoint(t, filepath.Join(dir, "fritz", "node001"), 1000)
	writeTestCheckpoint(t, filepath.Join(dir, "fritz", "node002"), 1000)

	used := map[string][]string{"fritz": {"node001"}}
	n, err := deleteCheckpoints(dir, 3000, used)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 file deleted, got %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "fritz", "node001", "1000.json")); err != nil {
		t.Errorf("used node's checkpoint must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fritz", "node002", "1000.json")); !os.IsNotExist(err) {
		t.Error("unused node's checkpoint must be deleted")
	}
}

func TestArchiveCheckpointsSkipsUsedNodes(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	cpDir := t.TempDir()
	archiveDir := t.TempDir()
	writeTestCheckpoint(t, filepath.Join(cpDir, "fritz", "node001"), 1000)
	writeTestCheckpoint(t, filepath.Join(cpDir, "fritz", "node002"), 1000)

	used := map[string][]string{"fritz": {"node001"}}
	n, err := archiveCheckpoints(cpDir, archiveDir, 3000, used)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 file archived, got %d", n)
	}
	if _, err := os.Stat(filepath.Join(cpDir, "fritz", "node001", "1000.json")); err != nil {
		t.Errorf("used node's checkpoint must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cpDir, "fritz", "node002", "1000.json")); !os.IsNotExist(err) {
		t.Error("unused node's checkpoint must be removed after archiving")
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "fritz", "3000.parquet")); err != nil {
		t.Errorf("parquet archive must exist: %v", err)
	}
}

func TestCleanupCheckpointsAbortsOnProviderError(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	oldMS := msInstance
	msInstance = newTestStore()
	msInstance.SetNodeProvider(&fakeNodeProvider{err: errTestProvider})
	t.Cleanup(func() { msInstance = oldMS })

	dir := t.TempDir()
	writeTestCheckpoint(t, filepath.Join(dir, "fritz", "node001"), 1000)

	if _, err := CleanupCheckpoints(dir, "", 3000, true); err == nil {
		t.Fatal("expected error when GetUsedNodes fails")
	}
	if _, err := os.Stat(filepath.Join(dir, "fritz", "node001", "1000.json")); err != nil {
		t.Errorf("no files may be deleted when provider errors: %v", err)
	}
}

func TestCleanupCheckpointsUsedNodesSurvive(t *testing.T) {
	oldWorkers := Keys.NumWorkers
	Keys.NumWorkers = 2
	t.Cleanup(func() { Keys.NumWorkers = oldWorkers })

	oldMS := msInstance
	msInstance = newTestStore()
	msInstance.SetNodeProvider(&fakeNodeProvider{nodes: map[string][]string{"fritz": {"node001"}}})
	t.Cleanup(func() { msInstance = oldMS })

	dir := t.TempDir()
	writeTestCheckpoint(t, filepath.Join(dir, "fritz", "node001"), 1000)
	writeTestCheckpoint(t, filepath.Join(dir, "fritz", "node002"), 1000)

	n, err := CleanupCheckpoints(dir, "", 3000, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 file deleted, got %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "fritz", "node001", "1000.json")); err != nil {
		t.Errorf("used node's checkpoint must survive: %v", err)
	}
}
