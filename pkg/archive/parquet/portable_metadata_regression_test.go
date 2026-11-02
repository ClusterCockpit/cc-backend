// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import "testing"

func TestPortableMetadataSurvivesParquetRoundTrip(t *testing.T) {
	meta, data := makeTestJob(1001)
	meta.ArrayJobID, meta.SubmitTime = 900, 1600000000
	meta.Shared, meta.MonitoringStatus, meta.RequestedMemory = "multi_user", 3, 128000
	row, err := JobToParquetRow(meta, data)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := writeParquetBytes([]ParquetJobRow{*row})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadParquetFile(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := ParquetRowToJob(&rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.ArrayJobID != meta.ArrayJobID || got.SubmitTime != meta.SubmitTime || got.Shared != meta.Shared ||
		got.MonitoringStatus != meta.MonitoringStatus || got.RequestedMemory != meta.RequestedMemory {
		t.Fatal("portable allocation/lifecycle metadata was lost")
	}
}
