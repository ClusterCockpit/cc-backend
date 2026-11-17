// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

func TestGapInfoRecord(t *testing.T) {
	var g gapInfo
	g.record(3, 0)
	g.record(1, 5)
	want := gapInfo{buffers: 2, maxFront: 3, maxBack: 5}
	if g != want {
		t.Errorf("gapInfo = %+v, want %+v", g, want)
	}
}

func TestMissingReportMergesPerHost(t *testing.T) {
	var r missingReport
	r.add("n2", "flops", gapInfo{buffers: 4, maxFront: 2, maxBack: 1})
	r.add("n1", "temp", gapInfo{buffers: 2, maxFront: 3})
	r.add("n1", "flops", gapInfo{buffers: 5, maxFront: 1, maxBack: 6})
	r.add("n1", "flops", gapInfo{buffers: 1, maxFront: 1})
	r.add("n3", "load", gapInfo{}) // no gaps: no entry

	got := r.lines("cl", 100, 200)
	want := []string{
		"[METRICSTORE]> missing data points: node=cl/n1 requested[100,200] metrics=2 max_missing_front=3 pts max_missing_back=6 pts",
		"[METRICSTORE]> missing data points: node=cl/n2 requested[100,200] metrics=1 max_missing_front=2 pts max_missing_back=1 pts",
	}
	if !slices.Equal(got, want) {
		t.Errorf("lines =\n%q\nwant\n%q", got, want)
	}
}

func TestMissingReportEmptyAllocatesNothing(t *testing.T) {
	var r missingReport
	r.add("n1", "load", gapInfo{})
	if r.hosts != nil {
		t.Error("add without gaps allocated the host map")
	}
	if got := r.lines("cl", 0, 10); len(got) != 0 {
		t.Errorf("lines = %q, want none", got)
	}
}

func TestReadRecordsGaps(t *testing.T) {
	ms := newFetchStatsStore(t, 3)
	sel := querySelectors("cl", coreQuery("temp", false))[0]

	var g gapInfo
	if _, _, _, _, err := ms.read(sel, "temp", fsFrom, fsFrom+fsCount*fsFreq, 0, "", &g); err != nil {
		t.Fatal(err)
	}
	want := gapInfo{buffers: 1, maxFront: 3, maxBack: 0}
	if g != want {
		t.Errorf("gapInfo = %+v, want %+v", g, want)
	}
}

// captureWarnings redirects cclog warnings to a buffer for the duration of the test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := cclog.WarnLog.Writer()
	cclog.WarnLog.SetOutput(&buf)
	t.Cleanup(func() { cclog.WarnLog.SetOutput(old) })
	return &buf
}

// captureDebug switches cclog to debug level and redirects debug output to a
// buffer for the duration of the test. cclog.Init replaces every logger, so
// call it before captureWarnings.
func captureDebug(t *testing.T) *bytes.Buffer {
	t.Helper()
	oldLevel := cclog.Loglevel()
	oldDebug, oldInfo, oldWarn, oldErr, oldCrit := cclog.DebugLog, cclog.InfoLog, cclog.WarnLog, cclog.ErrLog, cclog.CritLog
	cclog.Init("debug", false)
	var buf bytes.Buffer
	cclog.DebugLog.SetOutput(&buf)
	t.Cleanup(func() {
		cclog.Init(oldLevel, false)
		cclog.DebugLog, cclog.InfoLog, cclog.WarnLog, cclog.ErrLog, cclog.CritLog = oldDebug, oldInfo, oldWarn, oldErr, oldCrit
	})
	return &buf
}

func gappedRequest(queries ...APIQuery) APIQueryRequest {
	return APIQueryRequest{
		Cluster: "cl", From: fsFrom, To: fsFrom + fsCount*fsFreq,
		Queries: queries,
	}
}

func TestFetchDataLogsOneSummaryPerHost(t *testing.T) {
	newFetchStatsStore(t, 3)
	buf := captureWarnings(t)

	if _, err := FetchData(gappedRequest(coreQuery("temp", false), coreQuery("flops", false))); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n := strings.Count(out, "missing data points"); n != 1 {
		t.Fatalf("got %d summary lines, want 1:\n%s", n, out)
	}
	if want := "node=cl/n1 requested[1000,1200] metrics=2 max_missing_front=3 pts max_missing_back=0 pts"; !strings.Contains(out, want) {
		t.Errorf("summary missing %q:\n%s", want, out)
	}
	if strings.Contains(out, "does not align") {
		t.Errorf("per-buffer line logged as warning:\n%s", out)
	}
}

func TestFetchDataLogsPerBufferAtDebug(t *testing.T) {
	newFetchStatsStore(t, 3)
	debug := captureDebug(t)
	warn := captureWarnings(t)

	if _, err := FetchData(gappedRequest(coreQuery("temp", false), coreQuery("flops", false))); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(debug.String(), "does not align"); n != 2 {
		t.Errorf("got %d per-buffer debug lines, want 2 (core1 of temp and flops):\n%s", n, debug.String())
	}
	if !strings.Contains(debug.String(), "node=cl/n1/core1") {
		t.Errorf("debug lines do not name the buffer:\n%s", debug.String())
	}
	out := warn.String()
	if n := strings.Count(out, "missing data points"); n != 1 {
		t.Errorf("got %d summary warnings, want 1:\n%s", n, out)
	}
	if strings.Contains(out, "does not align") {
		t.Errorf("per-buffer line logged as warning:\n%s", out)
	}
}

func TestFetchStatsFallbackLogsSummary(t *testing.T) {
	newFetchStatsStore(t, 3)
	buf := captureWarnings(t)

	if _, err := FetchStats(gappedRequest(coreQuery("temp", true))); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n := strings.Count(out, "missing data points"); n != 1 {
		t.Fatalf("got %d summary lines, want 1:\n%s", n, out)
	}
	if want := "node=cl/n1 requested[1000,1200] metrics=1 max_missing_front=3 pts max_missing_back=0 pts"; !strings.Contains(out, want) {
		t.Errorf("summary missing %q:\n%s", want, out)
	}
}
