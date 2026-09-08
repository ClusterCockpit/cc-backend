// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// resetForTest drops the package state built up by InstallSink and Init.
func resetForTest() {
	detachSink()
	source = nil
	active = ModeDisabled
}

// loggerState snapshots the cclog globals so a test can install sinks without
// leaking them into the next test.
type loggerState struct {
	writers []io.Writer
	loggers []*log.Logger
}

func snapshotLoggers() loggerState {
	return loggerState{
		writers: []io.Writer{cclog.DebugWriter, cclog.InfoWriter, cclog.WarnWriter, cclog.ErrWriter, cclog.CritWriter},
		loggers: []*log.Logger{cclog.DebugLog, cclog.InfoLog, cclog.WarnLog, cclog.ErrLog, cclog.CritLog},
	}
}

func (s loggerState) restore() {
	cclog.DebugWriter, cclog.InfoWriter, cclog.WarnWriter, cclog.ErrWriter, cclog.CritWriter =
		s.writers[0], s.writers[1], s.writers[2], s.writers[3], s.writers[4]
	cclog.DebugLog, cclog.InfoLog, cclog.WarnLog, cclog.ErrLog, cclog.CritLog =
		s.loggers[0], s.loggers[1], s.loggers[2], s.loggers[3], s.loggers[4]
}

// setLoggers points every cclog level at w, except those listed in discard.
func setLoggers(w io.Writer, discard map[string]bool) {
	pick := func(name string) io.Writer {
		if discard[name] {
			return io.Discard
		}
		return w
	}

	cclog.DebugWriter, cclog.InfoWriter = pick("debug"), pick("info")
	cclog.WarnWriter, cclog.ErrWriter, cclog.CritWriter = pick("warn"), pick("err"), pick("crit")
	cclog.DebugLog = log.New(cclog.DebugWriter, cclog.DebugPrefix, 0)
	cclog.InfoLog = log.New(cclog.InfoWriter, cclog.InfoPrefix, 0)
	cclog.WarnLog = log.New(cclog.WarnWriter, cclog.WarnPrefix, 0)
	cclog.ErrLog = log.New(cclog.ErrWriter, cclog.ErrPrefix, 0)
	cclog.CritLog = log.New(cclog.CritWriter, cclog.CritPrefix, 0)
}

// fill appends n records with an increasing message index.
func fill(r *ring, n int, prio uint8) {
	for i := range n {
		r.append(prio, fmt.Appendf(nil, "message %d", i))
	}
}

func messages(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Message
	}
	return out
}

func allQuery(lines int) Query {
	return Query{Lines: lines, Level: -1}
}

func TestRingKeepsNewestAfterWrap(t *testing.T) {
	r := newRing(minBufferSize)
	fill(r, 3*minBufferSize, prioInfo)

	entries := r.collect(allQuery(maxLogLinesForTest), noCutoff)
	if len(entries) != minBufferSize {
		t.Fatalf("got %d entries, want %d", len(entries), minBufferSize)
	}

	// Oldest first, and only the last minBufferSize messages survived.
	if want := fmt.Sprintf("message %d", 2*minBufferSize); entries[0].Message != want {
		t.Errorf("first entry = %q, want %q", entries[0].Message, want)
	}
	if want := fmt.Sprintf("message %d", 3*minBufferSize-1); entries[len(entries)-1].Message != want {
		t.Errorf("last entry = %q, want %q", entries[len(entries)-1].Message, want)
	}
}

func TestRingResizePreservesNewest(t *testing.T) {
	r := newRing(200)
	fill(r, 200, prioInfo)

	r.resize(minBufferSize)

	entries := r.collect(allQuery(maxLogLinesForTest), noCutoff)
	if len(entries) != minBufferSize {
		t.Fatalf("got %d entries after shrink, want %d", len(entries), minBufferSize)
	}
	if want := fmt.Sprintf("message %d", 200-minBufferSize); entries[0].Message != want {
		t.Errorf("first entry = %q, want %q", entries[0].Message, want)
	}

	// Growing keeps what is there and accepts more.
	r.resize(300)
	fill(r, 10, prioInfo)
	if entries = r.collect(allQuery(maxLogLinesForTest), noCutoff); len(entries) != minBufferSize+10 {
		t.Errorf("got %d entries after grow, want %d", len(entries), minBufferSize+10)
	}
}

func TestRingResizeBelowMinimum(t *testing.T) {
	r := newRing(1)
	if len(r.buf) != minBufferSize {
		t.Errorf("newRing(1) capacity = %d, want %d", len(r.buf), minBufferSize)
	}
}

func TestRingLevelFilter(t *testing.T) {
	r := newRing(minBufferSize)
	r.append(prioDebug, []byte("debug line"))
	r.append(prioInfo, []byte("info line"))
	r.append(prioWarn, []byte("warn line"))
	r.append(prioErr, []byte("error line"))

	// journalctl --priority semantics: the level and everything more severe.
	q := allQuery(10)
	q.Level = int(prioWarn)
	got := messages(r.collect(q, noCutoff))
	want := []string{"warn line", "error line"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("level filter got %v, want %v", got, want)
	}

	q.Level = -1
	if got := r.collect(q, noCutoff); len(got) != 4 {
		t.Errorf("unfiltered got %d entries, want 4", len(got))
	}
}

func TestRingSearchIsCaseInsensitive(t *testing.T) {
	r := newRing(minBufferSize)
	r.append(prioInfo, []byte("Starting Archiver"))
	r.append(prioInfo, []byte("stopping archiver"))
	r.append(prioInfo, []byte("unrelated"))

	q := allQuery(10)
	q.Search = "ARCHIVER"
	if got := r.collect(q, noCutoff); len(got) != 2 {
		t.Errorf("got %d matches, want 2: %v", len(got), messages(got))
	}

	q.Search = "nothing here"
	if got := r.collect(q, noCutoff); len(got) != 0 {
		t.Errorf("got %d matches, want 0", len(got))
	}
}

func TestRingLinesCap(t *testing.T) {
	r := newRing(minBufferSize)
	fill(r, 50, prioInfo)

	entries := r.collect(allQuery(10), noCutoff)
	if len(entries) != 10 {
		t.Fatalf("got %d entries, want 10", len(entries))
	}
	// The newest 10, oldest first.
	if entries[0].Message != "message 40" || entries[9].Message != "message 49" {
		t.Errorf("unexpected window: %v", messages(entries))
	}
}

func TestRingSinceCutoff(t *testing.T) {
	r := newRing(minBufferSize)
	fill(r, 10, prioInfo)

	// Backdate the first five records by an hour.
	cutoff := time.Now().Add(-30 * time.Minute).UnixMicro()
	for i := range 5 {
		r.buf[i].tsMicros = time.Now().Add(-time.Hour).UnixMicro()
	}

	entries := r.collect(allQuery(100), cutoff)
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5: %v", len(entries), messages(entries))
	}
	if entries[0].Message != "message 5" {
		t.Errorf("first entry = %q, want 'message 5'", entries[0].Message)
	}
}

func TestRingTruncatesLongRecords(t *testing.T) {
	r := newRing(minBufferSize)
	r.append(prioInfo, bytes.Repeat([]byte("x"), 2*maxRecordBytes))

	entries := r.collect(allQuery(1), noCutoff)
	if len(entries[0].Message) != maxRecordBytes {
		t.Errorf("message length = %d, want %d", len(entries[0].Message), maxRecordBytes)
	}
}

func TestRingAppendDoesNotAllocate(t *testing.T) {
	r := newRing(minBufferSize)
	msg := []byte("a log line well below the preallocated slot size")

	// Warm up so every slot owns its arena segment.
	fill(r, minBufferSize, prioInfo)

	if allocs := testing.AllocsPerRun(1000, func() { r.append(prioInfo, msg) }); allocs != 0 {
		t.Errorf("append allocated %v times per run, want 0", allocs)
	}
}

func TestSinkOneWritePerRecord(t *testing.T) {
	r := newRing(minBufferSize)
	sink := &levelSink{r: r, prio: prioErr, prefix: []byte(cclog.ErrPrefix)}

	// A multi-line message arrives as a single Write and must stay one entry.
	line := cclog.ErrPrefix + "main.go:12: first line\nsecond line\n"
	n, err := sink.Write([]byte(line))
	if err != nil || n != len(line) {
		t.Fatalf("Write() = (%d, %v), want (%d, nil)", n, err, len(line))
	}

	entries := r.collect(allQuery(10), noCutoff)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Message != "main.go:12: first line\nsecond line" {
		t.Errorf("message = %q", entries[0].Message)
	}
	if entries[0].Priority != int(prioErr) {
		t.Errorf("priority = %d, want %d", entries[0].Priority, prioErr)
	}
	if entries[0].Timestamp == "" {
		t.Error("timestamp is empty")
	}
}

func TestInstallSinkCaptures(t *testing.T) {
	defer snapshotLoggers().restore()
	defer resetForTest()
	resetForTest()

	var stderr bytes.Buffer
	setLoggers(&stderr, nil)
	InstallSink(minBufferSize)
	Init(Options{Mode: ModeMemory, BufferSize: minBufferSize})

	cclog.Warn("a warning")

	// The original writer still sees the line ...
	if !strings.Contains(stderr.String(), "a warning") {
		t.Errorf("original writer did not receive the line: %q", stderr.String())
	}

	// ... and so does the log view. Filtering to warnings keeps Init's own
	// info line about the resolved source out of the way.
	entries, err := Get().Query(t.Context(), Query{Lines: 10, Level: int(prioWarn)})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Message != "a warning" {
		t.Fatalf("got %v, want one 'a warning' entry", messages(entries))
	}
	if entries[0].Priority != int(prioWarn) {
		t.Errorf("priority = %d, want %d", entries[0].Priority, prioWarn)
	}
}

func TestInstallSinkRespectsLogLevel(t *testing.T) {
	defer snapshotLoggers().restore()
	defer resetForTest()
	resetForTest()

	var stderr bytes.Buffer
	// Mirrors cclog.Init("warn", ...): debug and info are discarded.
	setLoggers(&stderr, map[string]bool{"debug": true, "info": true})
	InstallSink(minBufferSize)
	Init(Options{Mode: ModeMemory, BufferSize: minBufferSize})

	cclog.Debug("a debug line")
	cclog.Info("an info line")
	cclog.Warn("a warning")

	entries, err := Get().Query(t.Context(), Query{Lines: 10, Level: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Message != "a warning" {
		t.Errorf("got %v, want only the warning", messages(entries))
	}
}

func TestInitDetachesSinkWhenNotMemory(t *testing.T) {
	defer snapshotLoggers().restore()
	defer resetForTest()
	resetForTest()

	var stderr bytes.Buffer
	setLoggers(&stderr, nil)
	InstallSink(minBufferSize)
	captured := buffer

	Init(Options{Mode: ModeDisabled})

	if buffer != nil {
		t.Error("buffer was not released")
	}
	if Get() != nil {
		t.Error("source should be nil when disabled")
	}

	cclog.Warn("after detach")
	if !strings.Contains(stderr.String(), "after detach") {
		t.Error("original writer was not restored")
	}
	if got := captured.collect(allQuery(10), noCutoff); len(got) != 0 {
		t.Errorf("detached buffer still captured %v", messages(got))
	}
}

func TestInstallSinkIsIdempotent(t *testing.T) {
	defer snapshotLoggers().restore()
	defer resetForTest()
	resetForTest()

	var stderr bytes.Buffer
	setLoggers(&stderr, nil)
	InstallSink(minBufferSize)
	first := buffer
	InstallSink(minBufferSize)

	if buffer != first {
		t.Error("second InstallSink replaced the buffer")
	}

	cclog.Warn("once")
	if got := buffer.collect(allQuery(10), noCutoff); len(got) != 1 {
		t.Errorf("line captured %d times, want once", len(got))
	}
}

func TestRingConcurrentAccess(t *testing.T) {
	r := newRing(minBufferSize)

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				r.append(prioInfo, fmt.Appendf(nil, "writer %d line %d", w, i))
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			q := allQuery(50)
			q.Search = "writer"
			r.collect(q, noCutoff)
		}
	}()

	wg.Wait()

	if got := r.collect(allQuery(maxLogLinesForTest), noCutoff); len(got) != minBufferSize {
		t.Errorf("got %d entries, want %d", len(got), minBufferSize)
	}
}

func TestContainsFold(t *testing.T) {
	tests := []struct {
		hay    string
		needle string
		want   bool
	}{
		{"Starting Archiver", "archiver", true},
		{"Starting Archiver", "starting", true},
		{"Starting Archiver", "ARCHIVER", false}, // needle must be lower case
		{"abc", "", true},
		{"abc", "abcd", false},
		{"aab", "ab", true},
		{"", "a", false},
	}

	for _, tt := range tests {
		if got := containsFold([]byte(tt.hay), tt.needle); got != tt.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", tt.hay, tt.needle, got, tt.want)
		}
	}
}

// noCutoff disables the 'since' filter, and maxLogLinesForTest is a limit
// larger than any buffer used here.
const (
	noCutoff           = int64(-1) << 62
	maxLogLinesForTest = 100000
)
