// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"reflect"
	"testing"

	pq "github.com/parquet-go/parquet-go"
)

// Exact original pre-BUG-0020 schema, independently frozen for reader compatibility.
type legacyParquetJobRow struct {
	JobID          int64   `parquet:"job_id"`
	Cluster        string  `parquet:"cluster"`
	SubCluster     string  `parquet:"sub_cluster"`
	Partition      string  `parquet:"partition,optional"`
	Project        string  `parquet:"project"`
	User           string  `parquet:"user"`
	State          string  `parquet:"job_state"`
	StartTime      int64   `parquet:"start_time"`
	Duration       int32   `parquet:"duration"`
	Walltime       int64   `parquet:"walltime"`
	NumNodes       int32   `parquet:"num_nodes"`
	NumHWThreads   int32   `parquet:"num_hwthreads"`
	NumAcc         int32   `parquet:"num_acc"`
	Exclusive      int32   `parquet:"exclusive"`
	Energy         float64 `parquet:"energy"`
	SMT            int32   `parquet:"smt"`
	ResourcesJSON  []byte  `parquet:"resources_json"`
	StatisticsJSON []byte  `parquet:"statistics_json,optional"`
	TagsJSON       []byte  `parquet:"tags_json,optional"`
	MetaDataJSON   []byte  `parquet:"meta_data_json,optional"`
	FootprintJSON  []byte  `parquet:"footprint_json,optional"`
	EnergyFootJSON []byte  `parquet:"energy_footprint_json,optional"`
	MetricDataGz   []byte  `parquet:"metric_data_gz"`
}

func TestLegacyAndExtendedParquetReadersRemainCompatible(t *testing.T) {
	meta, data := makeTestJob(77)
	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatal(err)
	}
	// Populate the frozen original schema by matching original field names.
	var old legacyParquetJobRow
	oldValue, newValue := reflect.ValueOf(&old).Elem(), reflect.ValueOf(row).Elem()
	for i := 0; i < oldValue.NumField(); i++ {
		oldValue.Field(i).Set(newValue.FieldByName(oldValue.Type().Field(i).Name))
	}
	var buffer bytes.Buffer
	oldWriter := pq.NewGenericWriter[legacyParquetJobRow](&buffer)
	if _, err := oldWriter.Write([]legacyParquetJobRow{old}); err != nil {
		t.Fatal(err)
	}
	if err := oldWriter.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadParquetFile(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].JobID != 77 || rows[0].Shared != "" || rows[0].ArrayJobID != 0 || rows[0].RequestedMemory != 0 || rows[0].MonitoringStatus != 0 || rows[0].SubmitTime != 0 {
		t.Fatal("new reader changed original values/defaults")
	}
	// Old reader must ignore additive optional columns and retain every old value.
	row.Shared, row.ArrayJobID, row.RequestedMemory, row.MonitoringStatus, row.SubmitTime = "single_user", 900, 128000, 3, 1600000000
	extended, err := writeParquetBytes([]ParquetJobRow{*row})
	if err != nil {
		t.Fatal(err)
	}
	reader := pq.NewGenericReader[legacyParquetJobRow](bytes.NewReader(extended))
	defer reader.Close()
	got := make([]legacyParquetJobRow, 1)
	if n, err := reader.Read(got); n != 1 {
		t.Fatalf("old reader got %d rows: %v", n, err)
	}
	if !reflect.DeepEqual(got[0], old) {
		t.Fatal("old reader changed existing schema values")
	}
}

func TestExtendedParquetReaderRejectsTruncatedFiles(t *testing.T) {
	meta, data := makeTestJob(77)
	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := writeParquetBytes([]ParquetJobRow{*row})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadParquetFile(complete[:len(complete)-8]); err == nil {
		t.Fatal("truncated file accepted")
	}
}
