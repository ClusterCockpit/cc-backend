// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"math"
	"testing"
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
