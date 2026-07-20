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
