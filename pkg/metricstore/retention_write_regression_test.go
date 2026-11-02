// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"bytes"
	"reflect"
	"runtime"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

func TestLevelReviewRetentionWriteReachability(t *testing.T) {
	lost := 0
	for i := range 300 {
		store := &MemoryStore{Metrics: map[string]MetricConfig{"metric": {Frequency: 10}}}
		start, done := make(chan struct{}), make(chan struct{})
		go func() {
			<-start
			for range 128 {
				store.root.free(0)
				runtime.Gosched()
			}
			close(done)
		}()
		close(start)
		err := store.Write([]string{"cluster", "node", "hwthread2"}, 100, []Metric{{Name: "metric", Value: 7}})
		<-done
		if err != nil {
			t.Fatal(err)
		}
		data, _, _, _, err := store.Read(util.Selector{{String: "cluster"}, {String: "node"}, {String: "hwthread2"}}, "metric", 100, 110, 0, "")
		if err != nil || !reflect.DeepEqual(data, []schema.Float{7}) {
			lost++
			if lost <= 3 {
				t.Logf("successful write%d later missing: data%v err%v", i, data, err)
			}
		}
	}
	if lost != 0 {
		t.Errorf("%d/300 successful writes became unreachable during ordinary retention cutoff0", lost)
	}
}

func TestLevelReviewDecodeLineReachability(t *testing.T) {
	oldFormat := Keys.Checkpoints.FileFormat
	Keys.Checkpoints.FileFormat = "json"
	t.Cleanup(func() { Keys.Checkpoints.FileFormat = oldFormat })
	lost := 0
	for i := range 300 {
		store := &MemoryStore{Metrics: map[string]MetricConfig{"metric": {Frequency: 10}}}
		start, done := make(chan struct{}), make(chan struct{})
		go func() {
			<-start
			for range 128 {
				store.root.free(0)
				runtime.Gosched()
			}
			close(done)
		}()
		close(start)
		decoder := lineprotocol.NewDecoder(bytes.NewBufferString("metric,cluster=cluster,hostname=node,type=hwthread,type-id=2 value=7 100\nmetric,cluster=cluster,hostname=node,type=hwthread,type-id=2 value=8 110\n"))
		err := DecodeLine(decoder, store, "")
		<-done
		if err != nil {
			t.Fatal(err)
		}
		data, _, _, _, err := store.Read(util.Selector{{String: "cluster"}, {String: "node"}, {String: "hwthread2"}}, "metric", 100, 120, 0, "")
		if err != nil || !reflect.DeepEqual(data, []schema.Float{7, 8}) {
			lost++
			if lost <= 3 {
				t.Logf("decoded valid batch%d missing: %v/%v", i, data, err)
			}
		}
	}
	if lost != 0 {
		t.Errorf("%d/300 valid decoded batches lost a successful metric write", lost)
	}
}
