// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestParquetRowToJob(t *testing.T) {
	meta := &schema.Job{
		JobID:        42,
		Cluster:      "testcluster",
		SubCluster:   "sc0",
		Partition:    "main",
		Project:      "testproject",
		User:         "testuser",
		State:        schema.JobStateCompleted,
		StartTime:    1700000000,
		Duration:     3600,
		Walltime:     7200,
		NumNodes:     2,
		NumHWThreads: 16,
		NumAcc:       4,
		Energy:       123.45,
		SMT:          2,
		Resources: []*schema.Resource{
			{Hostname: "node001", HWThreads: []int{0, 1, 2, 3}},
			{Hostname: "node002", HWThreads: []int{4, 5, 6, 7}},
		},
		Statistics: map[string]schema.JobStatistics{
			"cpu_load": {Avg: 50.0, Min: 10.0, Max: 90.0},
		},
		Tags: []*schema.Tag{
			{Type: "test", Name: "tag1"},
		},
		MetaData: map[string]string{
			"key1": "value1",
		},
		Footprint: map[string]float64{
			"cpu_load": 50.0,
		},
		EnergyFootprint: map[string]float64{
			"total": 123.45,
		},
	}

	data := &schema.JobData{
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

	// Convert to parquet row
	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatalf("JobToParquetRow: %v", err)
	}

	// Convert back
	gotMeta, gotData, err := ParquetRowToJob(row)
	if err != nil {
		t.Fatalf("ParquetRowToJob: %v", err)
	}

	// Verify scalar fields
	if gotMeta.JobID != meta.JobID {
		t.Errorf("JobID = %d, want %d", gotMeta.JobID, meta.JobID)
	}
	if gotMeta.Cluster != meta.Cluster {
		t.Errorf("Cluster = %q, want %q", gotMeta.Cluster, meta.Cluster)
	}
	if gotMeta.SubCluster != meta.SubCluster {
		t.Errorf("SubCluster = %q, want %q", gotMeta.SubCluster, meta.SubCluster)
	}
	if gotMeta.Partition != meta.Partition {
		t.Errorf("Partition = %q, want %q", gotMeta.Partition, meta.Partition)
	}
	if gotMeta.Project != meta.Project {
		t.Errorf("Project = %q, want %q", gotMeta.Project, meta.Project)
	}
	if gotMeta.User != meta.User {
		t.Errorf("User = %q, want %q", gotMeta.User, meta.User)
	}
	if gotMeta.State != meta.State {
		t.Errorf("State = %q, want %q", gotMeta.State, meta.State)
	}
	if gotMeta.StartTime != meta.StartTime {
		t.Errorf("StartTime = %d, want %d", gotMeta.StartTime, meta.StartTime)
	}
	if gotMeta.Duration != meta.Duration {
		t.Errorf("Duration = %d, want %d", gotMeta.Duration, meta.Duration)
	}
	if gotMeta.Walltime != meta.Walltime {
		t.Errorf("Walltime = %d, want %d", gotMeta.Walltime, meta.Walltime)
	}
	if gotMeta.NumNodes != meta.NumNodes {
		t.Errorf("NumNodes = %d, want %d", gotMeta.NumNodes, meta.NumNodes)
	}
	if gotMeta.NumHWThreads != meta.NumHWThreads {
		t.Errorf("NumHWThreads = %d, want %d", gotMeta.NumHWThreads, meta.NumHWThreads)
	}
	if gotMeta.NumAcc != meta.NumAcc {
		t.Errorf("NumAcc = %d, want %d", gotMeta.NumAcc, meta.NumAcc)
	}
	if gotMeta.Energy != meta.Energy {
		t.Errorf("Energy = %f, want %f", gotMeta.Energy, meta.Energy)
	}
	if gotMeta.SMT != meta.SMT {
		t.Errorf("SMT = %d, want %d", gotMeta.SMT, meta.SMT)
	}

	// Verify complex fields
	if len(gotMeta.Resources) != 2 {
		t.Fatalf("Resources len = %d, want 2", len(gotMeta.Resources))
	}
	if gotMeta.Resources[0].Hostname != "node001" {
		t.Errorf("Resources[0].Hostname = %q, want %q", gotMeta.Resources[0].Hostname, "node001")
	}
	if len(gotMeta.Resources[0].HWThreads) != 4 {
		t.Errorf("Resources[0].HWThreads len = %d, want 4", len(gotMeta.Resources[0].HWThreads))
	}

	if len(gotMeta.Statistics) != 1 {
		t.Fatalf("Statistics len = %d, want 1", len(gotMeta.Statistics))
	}
	if stat, ok := gotMeta.Statistics["cpu_load"]; !ok {
		t.Error("Statistics missing cpu_load")
	} else if stat.Avg != 50.0 {
		t.Errorf("Statistics[cpu_load].Avg = %f, want 50.0", stat.Avg)
	}

	if len(gotMeta.Tags) != 1 || gotMeta.Tags[0].Name != "tag1" {
		t.Errorf("Tags = %v, want [{test tag1}]", gotMeta.Tags)
	}

	if gotMeta.MetaData["key1"] != "value1" {
		t.Errorf("MetaData[key1] = %q, want %q", gotMeta.MetaData["key1"], "value1")
	}

	if gotMeta.Footprint["cpu_load"] != 50.0 {
		t.Errorf("Footprint[cpu_load] = %f, want 50.0", gotMeta.Footprint["cpu_load"])
	}

	if gotMeta.EnergyFootprint["total"] != 123.45 {
		t.Errorf("EnergyFootprint[total] = %f, want 123.45", gotMeta.EnergyFootprint["total"])
	}

	// Verify metric data
	if gotData == nil {
		t.Fatal("JobData is nil")
	}
	cpuLoad, ok := (*gotData)["cpu_load"]
	if !ok {
		t.Fatal("JobData missing cpu_load")
	}
	nodeMetric, ok := cpuLoad[schema.MetricScopeNode]
	if !ok {
		t.Fatal("cpu_load missing node scope")
	}
	if nodeMetric.Timestep != 60 {
		t.Errorf("Timestep = %d, want 60", nodeMetric.Timestep)
	}
	if len(nodeMetric.Series) != 1 {
		t.Fatalf("Series len = %d, want 1", len(nodeMetric.Series))
	}
	if nodeMetric.Series[0].Hostname != "node001" {
		t.Errorf("Series[0].Hostname = %q, want %q", nodeMetric.Series[0].Hostname, "node001")
	}
	if len(nodeMetric.Series[0].Data) != 3 {
		t.Errorf("Series[0].Data len = %d, want 3", len(nodeMetric.Series[0].Data))
	}
}

func TestParquetRowToJobNilOptionalFields(t *testing.T) {
	meta := &schema.Job{
		JobID:      1,
		Cluster:    "test",
		SubCluster: "sc0",
		Project:    "proj",
		User:       "user",
		State:      schema.JobStateCompleted,
		StartTime:  1700000000,
		Duration:   60,
		NumNodes:   1,
		Resources: []*schema.Resource{
			{Hostname: "node001"},
		},
	}

	data := &schema.JobData{
		"cpu_load": {
			schema.MetricScopeNode: &schema.JobMetric{
				Timestep: 60,
				Series: []schema.Series{
					{Hostname: "node001", Data: []schema.Float{1.0}},
				},
			},
		},
	}

	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatalf("JobToParquetRow: %v", err)
	}

	gotMeta, gotData, err := ParquetRowToJob(row)
	if err != nil {
		t.Fatalf("ParquetRowToJob: %v", err)
	}

	if gotMeta.JobID != 1 {
		t.Errorf("JobID = %d, want 1", gotMeta.JobID)
	}
	if gotMeta.Tags != nil {
		t.Errorf("Tags should be nil, got %v", gotMeta.Tags)
	}
	if gotMeta.Statistics != nil {
		t.Errorf("Statistics should be nil, got %v", gotMeta.Statistics)
	}
	if gotMeta.MetaData != nil {
		t.Errorf("MetaData should be nil, got %v", gotMeta.MetaData)
	}
	if gotMeta.Footprint != nil {
		t.Errorf("Footprint should be nil, got %v", gotMeta.Footprint)
	}
	if gotMeta.EnergyFootprint != nil {
		t.Errorf("EnergyFootprint should be nil, got %v", gotMeta.EnergyFootprint)
	}
	if gotData == nil {
		t.Fatal("JobData is nil")
	}
}

func TestRoundTripThroughParquetFile(t *testing.T) {
	meta, data := makeTestJob(999)
	meta.Tags = []*schema.Tag{{Type: "test", Name: "roundtrip"}}

	// Convert to row and write to parquet
	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatalf("JobToParquetRow: %v", err)
	}

	// Write to parquet bytes
	parquetBytes, err := writeParquetBytes([]ParquetJobRow{*row})
	if err != nil {
		t.Fatalf("writeParquetBytes: %v", err)
	}

	// Read back from parquet bytes
	rows, err := ReadParquetFile(parquetBytes)
	if err != nil {
		t.Fatalf("ReadParquetFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	// Convert back to job
	gotMeta, gotData, err := ParquetRowToJob(&rows[0])
	if err != nil {
		t.Fatalf("ParquetRowToJob: %v", err)
	}

	// Verify key fields survived the round trip
	if gotMeta.JobID != 999 {
		t.Errorf("JobID = %d, want 999", gotMeta.JobID)
	}
	if gotMeta.Cluster != "testcluster" {
		t.Errorf("Cluster = %q, want %q", gotMeta.Cluster, "testcluster")
	}
	if gotMeta.User != "testuser" {
		t.Errorf("User = %q, want %q", gotMeta.User, "testuser")
	}
	if gotMeta.State != schema.JobStateCompleted {
		t.Errorf("State = %q, want %q", gotMeta.State, schema.JobStateCompleted)
	}
	if len(gotMeta.Tags) != 1 || gotMeta.Tags[0].Name != "roundtrip" {
		t.Errorf("Tags = %v, want [{test roundtrip}]", gotMeta.Tags)
	}
	if len(gotMeta.Resources) != 2 {
		t.Errorf("Resources len = %d, want 2", len(gotMeta.Resources))
	}

	if gotData == nil {
		t.Fatal("JobData is nil")
	}
	if _, ok := (*gotData)["cpu_load"]; !ok {
		t.Error("JobData missing cpu_load")
	}
}
