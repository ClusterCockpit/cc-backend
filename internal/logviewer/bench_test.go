// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"io"
	"testing"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// discardWriter stands in for stderr without the syscall, so the benchmarks
// measure the sink itself rather than the terminal.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func benchmarkLogging(b *testing.B, withSink bool) {
	defer snapshotLoggers().restore()
	defer resetForTest()
	resetForTest()

	setLoggers(discardWriter{}, nil)
	if withSink {
		InstallSink(DefaultBufferSize)
	}

	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		cclog.Warnf("job %d finished on node %s", i, "node042")
	}
}

func BenchmarkLoggerWithoutSink(b *testing.B) { benchmarkLogging(b, false) }
func BenchmarkLoggerWithSink(b *testing.B)    { benchmarkLogging(b, true) }

func BenchmarkRingAppend(b *testing.B) {
	r := newRing(DefaultBufferSize)
	msg := []byte("<4>[WARNING]  rest.go:57: job 4711 finished on node042")

	b.ReportAllocs()
	for b.Loop() {
		r.append(prioWarn, msg)
	}
}

func BenchmarkRingCollect(b *testing.B) {
	r := newRing(DefaultBufferSize)
	fill(r, DefaultBufferSize, prioInfo)
	q := Query{Lines: 200, Level: -1, Search: "message"}

	b.ReportAllocs()
	for b.Loop() {
		r.collect(q, noCutoff)
	}
}

var _ io.Writer = discardWriter{}
