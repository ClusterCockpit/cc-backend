// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"math"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestNewBufferStatsInitialized(t *testing.T) {
	b := newBuffer(100, 10)
	if b.statSamples != 0 {
		t.Errorf("statSamples = %d, want 0", b.statSamples)
	}
	if b.statSum != 0 {
		t.Errorf("statSum = %v, want 0", b.statSum)
	}
	if b.statMin != math.MaxFloat32 {
		t.Errorf("statMin = %v, want MaxFloat32 sentinel", b.statMin)
	}
	if b.statMax != -math.MaxFloat32 {
		t.Errorf("statMax = %v, want -MaxFloat32 sentinel", b.statMax)
	}
	if !b.statsValid {
		t.Error("statsValid = false, want true for a fresh empty buffer")
	}
}

func TestWriteUpdatesRunningStats(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(3.0))
	b.write(110, schema.Float(1.0))
	b.write(120, schema.Float(5.0))

	if b.statSamples != 3 {
		t.Errorf("statSamples = %d, want 3", b.statSamples)
	}
	if b.statSum != 9.0 {
		t.Errorf("statSum = %v, want 9.0", b.statSum)
	}
	if b.statMin != 1.0 {
		t.Errorf("statMin = %v, want 1.0", b.statMin)
	}
	if b.statMax != 5.0 {
		t.Errorf("statMax = %v, want 5.0", b.statMax)
	}
	if !b.statsValid {
		t.Error("statsValid = false, want true after appends only")
	}
}

func TestWriteNaNExcludedFromStats(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(2.0))
	b.write(110, schema.NaN)        // explicit NaN
	b.write(130, schema.Float(4.0)) // leaves a NaN-filled gap at t=120

	if b.statSamples != 2 {
		t.Errorf("statSamples = %d, want 2 (NaN and gap excluded)", b.statSamples)
	}
	if b.statSum != 6.0 {
		t.Errorf("statSum = %v, want 6.0", b.statSum)
	}
	if b.statMin != 2.0 || b.statMax != 4.0 {
		t.Errorf("statMin/statMax = %v/%v, want 2.0/4.0", b.statMin, b.statMax)
	}
}

func TestWriteOverwriteInvalidatesStats(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))
	if !b.statsValid {
		t.Fatal("precondition: statsValid should be true after appends")
	}

	b.write(100, schema.Float(99.0)) // overwrite existing index
	if b.statsValid {
		t.Error("statsValid = true, want false after overwrite")
	}
}

func TestRecomputeStatsRebuildsAfterOverwrite(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))
	b.write(100, schema.Float(99.0)) // overwrite -> statsValid false, min stale

	b.recomputeStats()

	if !b.statsValid {
		t.Error("statsValid = false, want true after recompute")
	}
	if b.statSamples != 2 {
		t.Errorf("statSamples = %d, want 2", b.statSamples)
	}
	if b.statSum != 101.0 {
		t.Errorf("statSum = %v, want 101.0", b.statSum)
	}
	if b.statMin != 2.0 {
		t.Errorf("statMin = %v, want 2.0 (1.0 was overwritten)", b.statMin)
	}
	if b.statMax != 99.0 {
		t.Errorf("statMax = %v, want 99.0", b.statMax)
	}
}

func TestRecomputeStatsEmptyBuffer(t *testing.T) {
	b := newBuffer(100, 10)
	b.recomputeStats()
	if b.statSamples != 0 || b.statSum != 0 {
		t.Errorf("empty buffer stats = samples %d sum %v, want 0/0", b.statSamples, b.statSum)
	}
	if b.statMin != math.MaxFloat32 || b.statMax != -math.MaxFloat32 {
		t.Errorf("empty buffer min/max = %v/%v, want sentinels", b.statMin, b.statMax)
	}
}

func TestStatsFullBufferMatchesScan(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(3.0))
	b.write(110, schema.Float(1.0))
	b.write(120, schema.Float(5.0))
	// Range fully covers the buffer (from before firstWrite, to at/after end).
	s, _, _, err := b.stats(100, 130)
	if err != nil {
		t.Fatalf("stats() error = %v", err)
	}
	if s.Samples != 3 {
		t.Errorf("Samples = %d, want 3", s.Samples)
	}
	if s.Min != 1.0 || s.Max != 5.0 {
		t.Errorf("Min/Max = %v/%v, want 1.0/5.0", s.Min, s.Max)
	}
	if s.Avg != 3.0 {
		t.Errorf("Avg = %v, want 3.0", s.Avg)
	}
}

func TestStatsPartialRangeScans(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(3.0)) // t=100
	b.write(110, schema.Float(1.0)) // t=110
	b.write(120, schema.Float(5.0)) // t=120
	// Partial range: only t=110 and t=120 (from excludes t=100).
	s, _, _, err := b.stats(110, 130)
	if err != nil {
		t.Fatalf("stats() error = %v", err)
	}
	if s.Samples != 2 {
		t.Errorf("Samples = %d, want 2", s.Samples)
	}
	if s.Min != 1.0 || s.Max != 5.0 {
		t.Errorf("Min/Max = %v/%v, want 1.0/5.0", s.Min, s.Max)
	}
}

func TestStatsOverwriteThenFullQuery(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))
	b.write(120, schema.Float(3.0))
	b.write(100, schema.Float(99.0)) // overwrite the running min -> statsValid false

	s, _, _, err := b.stats(100, 130)
	if err != nil {
		t.Fatalf("stats() error = %v", err)
	}
	if s.Min != 2.0 {
		t.Errorf("Min = %v, want 2.0 (recomputed after overwrite)", s.Min)
	}
	if s.Max != 99.0 {
		t.Errorf("Max = %v, want 99.0", s.Max)
	}
	if s.Samples != 3 {
		t.Errorf("Samples = %d, want 3", s.Samples)
	}
	if !b.statsValid {
		t.Error("statsValid should be true after the recompute triggered by the query")
	}
}

func TestStatsMultiBufferChain(t *testing.T) {
	// Two full buffers of cap=3 (interior buffers are always full).
	// b1: t=100,110,120 = 1,2,3 ; b2: t=130,140,150 = 4,5,6.
	b1 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 95}
	b1.data = append(b1.data, 1.0, 2.0, 3.0)
	b2 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 125}
	b2.data = append(b2.data, 4.0, 5.0, 6.0)
	b2.prev = b1
	b1.next = b2
	// b1/b2 built bare: statsValid is false (zero value) -> stats() must recompute.

	s, _, _, err := b2.stats(100, 160)
	if err != nil {
		t.Fatalf("stats() error = %v", err)
	}
	if s.Samples != 6 {
		t.Errorf("Samples = %d, want 6", s.Samples)
	}
	if s.Min != 1.0 || s.Max != 6.0 {
		t.Errorf("Min/Max = %v/%v, want 1.0/6.0", s.Min, s.Max)
	}
	if s.Avg != 3.5 {
		t.Errorf("Avg = %v, want 3.5", s.Avg)
	}
}

func TestStatsFastPathThenPartialTail(t *testing.T) {
	// Regression test: three buffers (all frequency 10) where the query fast-paths
	// earlier fully-covered buffers, then only PARTIALLY covers the last buffer.
	// b1: positions 0,1,2 = 1,2,3 at t=95,105,115 (end=125, fully in [100,190))
	// b2: positions 0,1,2 = 4,5,6 at t=125,135,145 (end=155, fully in [100,190))
	// b3: positions 0,1,2 = 7,8,9 at t=155,165,175 (end=185, only 155,165 in [100,170))
	// Query [100, 170) includes: 105, 115, 125, 135, 145, 155, 165 (7 points from b1,b2,b3).
	// Note: 175 is not included since 175 >= 170.
	// Expected: Samples=7, Sum=28, Min=1, Max=7, Avg=4.0.
	// Buffers built bare: statsValid is false (zero value) -> stats() must recompute.

	b1 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 95}
	b1.data = append(b1.data, 1.0, 2.0, 3.0)

	b2 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 125}
	b2.data = append(b2.data, 4.0, 5.0, 6.0)

	b3 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 155}
	b3.data = append(b3.data, 7.0, 8.0, 9.0)

	b1.next = b2
	b2.prev = b1
	b2.next = b3
	b3.prev = b2

	s, _, _, err := b3.stats(100, 170)
	if err != nil {
		t.Fatalf("stats() error = %v", err)
	}
	if s.Samples != 7 {
		t.Errorf("Samples = %d, want 7", s.Samples)
	}
	if s.Min != 1.0 || s.Max != 7.0 {
		t.Errorf("Min/Max = %v/%v, want 1.0/7.0", s.Min, s.Max)
	}
	if s.Avg != 4.0 {
		t.Errorf("Avg = %v, want 4.0", s.Avg)
	}
}
