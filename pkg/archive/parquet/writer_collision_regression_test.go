// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import "testing"

func TestConsecutiveJobWritersKeepBothExports(t *testing.T) {
	target := newMemTarget()
	for i := int64(0); i < 2; i++ {
		writer := NewParquetWriter(target, 16)
		meta, data := makeTestJob(1001 + i)
		row, err := JobToParquetRow(meta, data)
		if err != nil {
			t.Fatal(err)
		}
		if err := writer.AddJob(*row); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if len(target.files) != 2 {
		t.Fatal("later job retention pass replaced earlier output")
	}
}

func TestConsecutiveNodeStateWritersKeepBothExports(t *testing.T) {
	target := newMemTarget()
	for i := int64(0); i < 2; i++ {
		writer := NewNodeStateParquetWriter(target, 16)
		if err := writer.AddRow(ParquetNodeStateRow{TimeStamp: 1001 + i, Hostname: "fixture", Cluster: "testcluster"}); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if len(target.files) != 2 {
		t.Fatal("later node-state retention pass replaced earlier output")
	}
}
