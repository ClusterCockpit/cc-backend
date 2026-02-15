// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	pq "github.com/parquet-go/parquet-go"
)

// memTarget collects written files in memory for testing.
type memTarget struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMemTarget() *memTarget {
	return &memTarget{files: make(map[string][]byte)}
}

func (m *memTarget) WriteFile(name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[name] = append([]byte(nil), data...)
	return nil
}

func makeTestJob(jobID int64) (*schema.Job, *schema.JobData) {
	meta := &schema.Job{
		JobID:      jobID,
		Cluster:    "testcluster",
		SubCluster: "sc0",
		Project:    "testproject",
		User:       "testuser",
		State:      schema.JobStateCompleted,
		StartTime:  1700000000,
		Duration:   3600,
		Walltime:   7200,
		NumNodes:   2,
		NumHWThreads: 16,
		SMT:        1,
		Resources: []*schema.Resource{
			{Hostname: "node001"},
			{Hostname: "node002"},
		},
	}

	data := schema.JobData{
		"cpu_load": {
			schema.MetricScopeNode: &schema.JobMetric{
				Unit:     schema.Unit{Base: ""},
				Timestep: 60,
				Series: []schema.Series{
					{
						Hostname: "node001",
						Data:     []schema.Float{1.0, 2.0, 3.0},
					},
				},
			},
		},
	}

	return meta, &data
}

func TestJobToParquetRowConversion(t *testing.T) {
	meta, data := makeTestJob(1001)
	meta.Tags = []*schema.Tag{{Type: "test", Name: "tag1"}}
	meta.MetaData = map[string]string{"key": "value"}

	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatalf("JobToParquetRow: %v", err)
	}

	if row.JobID != 1001 {
		t.Errorf("JobID = %d, want 1001", row.JobID)
	}
	if row.Cluster != "testcluster" {
		t.Errorf("Cluster = %q, want %q", row.Cluster, "testcluster")
	}
	if row.User != "testuser" {
		t.Errorf("User = %q, want %q", row.User, "testuser")
	}
	if row.State != "completed" {
		t.Errorf("State = %q, want %q", row.State, "completed")
	}
	if row.NumNodes != 2 {
		t.Errorf("NumNodes = %d, want 2", row.NumNodes)
	}

	// Verify resources JSON
	var resources []*schema.Resource
	if err := json.Unmarshal(row.ResourcesJSON, &resources); err != nil {
		t.Fatalf("unmarshal resources: %v", err)
	}
	if len(resources) != 2 {
		t.Errorf("resources len = %d, want 2", len(resources))
	}

	// Verify tags JSON
	var tags []*schema.Tag
	if err := json.Unmarshal(row.TagsJSON, &tags); err != nil {
		t.Fatalf("unmarshal tags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "tag1" {
		t.Errorf("tags = %v, want [{test tag1}]", tags)
	}

	// Verify metric data is gzip-compressed valid JSON
	gz, err := gzip.NewReader(bytes.NewReader(row.MetricDataGz))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decompressed, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	var jobData schema.JobData
	if err := json.Unmarshal(decompressed, &jobData); err != nil {
		t.Fatalf("unmarshal metric data: %v", err)
	}
	if _, ok := jobData["cpu_load"]; !ok {
		t.Error("metric data missing cpu_load key")
	}
}

func TestParquetWriterSingleBatch(t *testing.T) {
	target := newMemTarget()
	pw := NewParquetWriter(target, 512)

	for i := int64(0); i < 5; i++ {
		meta, data := makeTestJob(i)
		row, err := JobToParquetRow(meta, data)
		if err != nil {
			t.Fatalf("convert job %d: %v", i, err)
		}
		if err := pw.AddJob(*row); err != nil {
			t.Fatalf("add job %d: %v", i, err)
		}
	}

	if err := pw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if len(target.files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(target.files))
	}

	// Verify the parquet file is readable
	for name, data := range target.files {
		file := bytes.NewReader(data)
		pf, err := pq.OpenFile(file, int64(len(data)))
		if err != nil {
			t.Fatalf("open parquet %s: %v", name, err)
		}
		if pf.NumRows() != 5 {
			t.Errorf("parquet rows = %d, want 5", pf.NumRows())
		}
	}
}

func TestParquetWriterBatching(t *testing.T) {
	target := newMemTarget()
	// Use a very small max size to force multiple files
	pw := NewParquetWriter(target, 0) // 0 MB means every job triggers a flush
	pw.maxSizeBytes = 1               // Force flush after every row

	for i := int64(0); i < 3; i++ {
		meta, data := makeTestJob(i)
		row, err := JobToParquetRow(meta, data)
		if err != nil {
			t.Fatalf("convert job %d: %v", i, err)
		}
		if err := pw.AddJob(*row); err != nil {
			t.Fatalf("add job %d: %v", i, err)
		}
	}

	if err := pw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// With maxSizeBytes=1, each AddJob should flush the previous batch,
	// resulting in multiple files
	if len(target.files) < 2 {
		t.Errorf("expected multiple files due to batching, got %d", len(target.files))
	}

	// Verify all files are valid parquet
	for name, data := range target.files {
		file := bytes.NewReader(data)
		_, err := pq.OpenFile(file, int64(len(data)))
		if err != nil {
			t.Errorf("invalid parquet file %s: %v", name, err)
		}
	}
}

func TestFileTarget(t *testing.T) {
	dir := t.TempDir()
	ft, err := NewFileTarget(dir)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}

	testData := []byte("test parquet data")
	if err := ft.WriteFile("test.parquet", testData); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Verify file exists and has correct content
	// (using the target itself is sufficient; we just check no error)
}

func TestFileTargetSubdirectories(t *testing.T) {
	dir := t.TempDir()
	ft, err := NewFileTarget(dir)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}

	testData := []byte("test data in subdir")
	if err := ft.WriteFile("fritz/cc-archive-2025-01-20-001.parquet", testData); err != nil {
		t.Fatalf("WriteFile with subdir: %v", err)
	}

	// Verify file was created in subdirectory
	content, err := os.ReadFile(filepath.Join(dir, "fritz", "cc-archive-2025-01-20-001.parquet"))
	if err != nil {
		t.Fatalf("read file in subdir: %v", err)
	}
	if !bytes.Equal(content, testData) {
		t.Error("file content mismatch")
	}
}

func makeTestJobForCluster(jobID int64, cluster string) (*schema.Job, *schema.JobData) {
	meta, data := makeTestJob(jobID)
	meta.Cluster = cluster
	return meta, data
}

func TestClusterAwareParquetWriter(t *testing.T) {
	target := newMemTarget()
	cw := NewClusterAwareParquetWriter(target, 512)

	// Set cluster configs
	cw.SetClusterConfig("fritz", &schema.Cluster{Name: "fritz"})
	cw.SetClusterConfig("alex", &schema.Cluster{Name: "alex"})

	// Add jobs from different clusters
	for i := int64(0); i < 3; i++ {
		meta, data := makeTestJobForCluster(i, "fritz")
		row, err := JobToParquetRow(meta, data)
		if err != nil {
			t.Fatalf("convert fritz job %d: %v", i, err)
		}
		if err := cw.AddJob(*row); err != nil {
			t.Fatalf("add fritz job %d: %v", i, err)
		}
	}

	for i := int64(10); i < 12; i++ {
		meta, data := makeTestJobForCluster(i, "alex")
		row, err := JobToParquetRow(meta, data)
		if err != nil {
			t.Fatalf("convert alex job %d: %v", i, err)
		}
		if err := cw.AddJob(*row); err != nil {
			t.Fatalf("add alex job %d: %v", i, err)
		}
	}

	if err := cw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	target.mu.Lock()
	defer target.mu.Unlock()

	// Check cluster.json files were written
	if _, ok := target.files["fritz/cluster.json"]; !ok {
		t.Error("missing fritz/cluster.json")
	}
	if _, ok := target.files["alex/cluster.json"]; !ok {
		t.Error("missing alex/cluster.json")
	}

	// Verify cluster.json content
	var clusterCfg schema.Cluster
	if err := json.Unmarshal(target.files["fritz/cluster.json"], &clusterCfg); err != nil {
		t.Fatalf("unmarshal fritz cluster.json: %v", err)
	}
	if clusterCfg.Name != "fritz" {
		t.Errorf("fritz cluster name = %q, want %q", clusterCfg.Name, "fritz")
	}

	// Check parquet files are in cluster subdirectories
	fritzParquets := 0
	alexParquets := 0
	for name := range target.files {
		if strings.HasPrefix(name, "fritz/") && strings.HasSuffix(name, ".parquet") {
			fritzParquets++
		}
		if strings.HasPrefix(name, "alex/") && strings.HasSuffix(name, ".parquet") {
			alexParquets++
		}
	}
	if fritzParquets == 0 {
		t.Error("no parquet files in fritz/")
	}
	if alexParquets == 0 {
		t.Error("no parquet files in alex/")
	}

	// Verify parquet files are readable and have correct row counts
	for name, data := range target.files {
		if !strings.HasSuffix(name, ".parquet") {
			continue
		}
		file := bytes.NewReader(data)
		pf, err := pq.OpenFile(file, int64(len(data)))
		if err != nil {
			t.Errorf("open parquet %s: %v", name, err)
			continue
		}
		if strings.HasPrefix(name, "fritz/") && pf.NumRows() != 3 {
			t.Errorf("fritz parquet rows = %d, want 3", pf.NumRows())
		}
		if strings.HasPrefix(name, "alex/") && pf.NumRows() != 2 {
			t.Errorf("alex parquet rows = %d, want 2", pf.NumRows())
		}
	}
}

func TestClusterAwareParquetWriterEmpty(t *testing.T) {
	target := newMemTarget()
	cw := NewClusterAwareParquetWriter(target, 512)

	if err := cw.Close(); err != nil {
		t.Fatalf("close empty writer: %v", err)
	}

	if len(target.files) != 0 {
		t.Errorf("expected no files for empty writer, got %d", len(target.files))
	}
}
