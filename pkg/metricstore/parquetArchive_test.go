// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	pq "github.com/parquet-go/parquet-go"
)

func TestParseScopeFromName(t *testing.T) {
	tests := []struct {
		name      string
		wantScope string
		wantID    string
	}{
		{"socket0", "socket", "0"},
		{"socket12", "socket", "12"},
		{"core0", "core", "0"},
		{"core127", "core", "127"},
		{"cpu0", "hwthread", "0"},
		{"hwthread5", "hwthread", "5"},
		{"memoryDomain0", "memoryDomain", "0"},
		{"accelerator0", "accelerator", "0"},
		{"unknown", "unknown", ""},
		{"socketX", "socketX", ""}, // not numeric suffix
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, id := parseScopeFromName(tt.name)
			if scope != tt.wantScope || id != tt.wantID {
				t.Errorf("parseScopeFromName(%q) = (%q, %q), want (%q, %q)",
					tt.name, scope, id, tt.wantScope, tt.wantID)
			}
		})
	}
}

func TestFlattenCheckpointFile(t *testing.T) {
	cf := &CheckpointFile{
		From: 1000,
		To:   1060,
		Metrics: map[string]*CheckpointMetrics{
			"cpu_load": {
				Frequency: 60,
				Start:     1000,
				Data:      []schema.Float{0.5, 0.7, schema.NaN},
			},
		},
		Children: map[string]*CheckpointFile{
			"socket0": {
				Metrics: map[string]*CheckpointMetrics{
					"mem_bw": {
						Frequency: 60,
						Start:     1000,
						Data:      []schema.Float{100.0, schema.NaN, 200.0},
					},
				},
				Children: make(map[string]*CheckpointFile),
			},
		},
	}

	rows := flattenCheckpointFile(cf, "fritz", "node001", "node", "", nil)

	// cpu_load: 2 non-NaN values at node scope
	// mem_bw: 2 non-NaN values at socket0 scope
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Verify a node-scope row
	found := false
	for _, r := range rows {
		if r.Metric == "cpu_load" && r.Timestamp == 1000 {
			found = true
			if r.Cluster != "fritz" || r.Hostname != "node001" || r.Scope != "node" || r.Value != 0.5 {
				t.Errorf("unexpected row: %+v", r)
			}
		}
	}
	if !found {
		t.Error("expected cpu_load row at timestamp 1000")
	}

	// Verify a socket-scope row
	found = false
	for _, r := range rows {
		if r.Metric == "mem_bw" && r.Scope == "socket" && r.ScopeID == "0" {
			found = true
		}
	}
	if !found {
		t.Error("expected mem_bw row with scope=socket, scope_id=0")
	}
}

func TestParquetArchiveRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create checkpoint files on disk (JSON format)
	cpDir := filepath.Join(tmpDir, "checkpoints", "testcluster", "node001")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cf := &CheckpointFile{
		From: 1000,
		To:   1180,
		Metrics: map[string]*CheckpointMetrics{
			"cpu_load": {
				Frequency: 60,
				Start:     1000,
				Data:      []schema.Float{0.5, 0.7, 0.9},
			},
			"mem_used": {
				Frequency: 60,
				Start:     1000,
				Data:      []schema.Float{45.0, 46.0, 47.0},
			},
		},
		Children: map[string]*CheckpointFile{
			"socket0": {
				Metrics: map[string]*CheckpointMetrics{
					"mem_bw": {
						Frequency: 60,
						Start:     1000,
						Data:      []schema.Float{100.0, 110.0, 120.0},
					},
				},
				Children: make(map[string]*CheckpointFile),
			},
		},
	}

	// Write JSON checkpoint
	cpFile := filepath.Join(cpDir, "1000.json")
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cpFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Archive to Parquet
	archiveDir := filepath.Join(tmpDir, "archive")
	rows, files, err := archiveCheckpointsToParquet(cpDir, "testcluster", "node001", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "1000.json" {
		t.Fatalf("expected 1 file, got %v", files)
	}

	parquetFile := filepath.Join(archiveDir, "testcluster", "1000.parquet")
	if err := writeParquetArchive(parquetFile, rows); err != nil {
		t.Fatal(err)
	}

	// Read back and verify
	f, err := os.Open(parquetFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	stat, _ := f.Stat()
	pf, err := pq.OpenFile(f, stat.Size())
	if err != nil {
		t.Fatal(err)
	}

	reader := pq.NewGenericReader[ParquetMetricRow](pf)
	readRows := make([]ParquetMetricRow, 100)
	n, err := reader.Read(readRows)
	if err != nil && n == 0 {
		t.Fatal(err)
	}
	readRows = readRows[:n]
	reader.Close()

	// We expect: cpu_load(3) + mem_used(3) + mem_bw(3) = 9 rows
	if n != 9 {
		t.Fatalf("expected 9 rows in parquet file, got %d", n)
	}

	// Verify cluster and hostname are set correctly
	for _, r := range readRows {
		if r.Cluster != "testcluster" {
			t.Errorf("expected cluster=testcluster, got %s", r.Cluster)
		}
		if r.Hostname != "node001" {
			t.Errorf("expected hostname=node001, got %s", r.Hostname)
		}
	}

	// Verify parquet file is smaller than JSON (compression working)
	if stat.Size() == 0 {
		t.Error("parquet file is empty")
	}

	t.Logf("Parquet file size: %d bytes for %d rows", stat.Size(), n)
}

func TestLoadCheckpointFileFromDisk_JSON(t *testing.T) {
	tmpDir := t.TempDir()

	cf := &CheckpointFile{
		From: 1000,
		To:   1060,
		Metrics: map[string]*CheckpointMetrics{
			"test_metric": {
				Frequency: 60,
				Start:     1000,
				Data:      []schema.Float{1.0, 2.0, 3.0},
			},
		},
		Children: make(map[string]*CheckpointFile),
	}

	filename := filepath.Join(tmpDir, "1000.json")
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadCheckpointFileFromDisk(filename)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.From != 1000 || loaded.To != 1060 {
		t.Errorf("expected From=1000, To=1060, got From=%d, To=%d", loaded.From, loaded.To)
	}

	m, ok := loaded.Metrics["test_metric"]
	if !ok {
		t.Fatal("expected test_metric in loaded checkpoint")
	}
	if m.Frequency != 60 || m.Start != 1000 || len(m.Data) != 3 {
		t.Errorf("unexpected metric data: %+v", m)
	}
}
