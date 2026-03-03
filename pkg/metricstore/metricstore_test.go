// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// ─── Buffer pool ─────────────────────────────────────────────────────────────

// TestBufferPoolGetReuse verifies that Get() returns pooled buffers before
// allocating new ones, and that an empty pool allocates a fresh BufferCap buffer.
func TestBufferPoolGetReuse(t *testing.T) {
	pool := NewPersistentBufferPool()

	original := &buffer{data: make([]schema.Float, 0, BufferCap), lastUsed: time.Now().Unix()}
	pool.Put(original)

	reused := pool.Get()
	if reused != original {
		t.Error("Get() should return the previously pooled buffer")
	}
	if pool.GetSize() != 0 {
		t.Errorf("pool size after Get() = %d, want 0", pool.GetSize())
	}

	// Empty pool must allocate a fresh buffer with the standard capacity.
	fresh := pool.Get()
	if fresh == nil {
		t.Fatal("Get() from empty pool returned nil")
	}
	if cap(fresh.data) != BufferCap {
		t.Errorf("fresh buffer cap = %d, want %d", cap(fresh.data), BufferCap)
	}
}

// TestBufferPoolClear verifies that Clear() drains all entries.
func TestBufferPoolClear(t *testing.T) {
	pool := NewPersistentBufferPool()
	for i := 0; i < 10; i++ {
		pool.Put(&buffer{data: make([]schema.Float, 0), lastUsed: time.Now().Unix()})
	}
	pool.Clear()
	if pool.GetSize() != 0 {
		t.Errorf("pool size after Clear() = %d, want 0", pool.GetSize())
	}
}

// TestBufferPoolMaxSize verifies that Put() silently drops buffers once the
// pool reaches maxPoolSize, preventing unbounded memory growth.
func TestBufferPoolMaxSize(t *testing.T) {
	pool := NewPersistentBufferPool()
	for i := 0; i < maxPoolSize; i++ {
		pool.Put(&buffer{data: make([]schema.Float, 0, BufferCap), lastUsed: time.Now().Unix()})
	}
	if pool.GetSize() != maxPoolSize {
		t.Fatalf("pool size = %d, want %d", pool.GetSize(), maxPoolSize)
	}

	pool.Put(&buffer{data: make([]schema.Float, 0, BufferCap), lastUsed: time.Now().Unix()})
	if pool.GetSize() != maxPoolSize {
		t.Errorf("pool size after overflow Put = %d, want %d (should not grow)", pool.GetSize(), maxPoolSize)
	}
}

// ─── Buffer helpers ───────────────────────────────────────────────────────────

// TestBufferEndFirstWrite verifies the end() and firstWrite() calculations.
func TestBufferEndFirstWrite(t *testing.T) {
	// start=90, freq=10 → firstWrite = 90+5 = 95
	b := &buffer{data: make([]schema.Float, 4, BufferCap), frequency: 10, start: 90}
	if fw := b.firstWrite(); fw != 95 {
		t.Errorf("firstWrite() = %d, want 95", fw)
	}
	// end = firstWrite + len(data)*freq = 95 + 4*10 = 135
	if e := b.end(); e != 135 {
		t.Errorf("end() = %d, want 135", e)
	}
}

// ─── Buffer write ─────────────────────────────────────────────────────────────

// TestBufferWriteNaNFill verifies that skipped timestamps are filled with NaN.
func TestBufferWriteNaNFill(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))
	// skip 110 and 120
	b.write(130, schema.Float(4.0))

	if len(b.data) != 4 {
		t.Fatalf("len(data) = %d, want 4 (1 value + 2 NaN + 1 value)", len(b.data))
	}
	if b.data[0] != schema.Float(1.0) {
		t.Errorf("data[0] = %v, want 1.0", b.data[0])
	}
	if !b.data[1].IsNaN() {
		t.Errorf("data[1] should be NaN (gap), got %v", b.data[1])
	}
	if !b.data[2].IsNaN() {
		t.Errorf("data[2] should be NaN (gap), got %v", b.data[2])
	}
	if b.data[3] != schema.Float(4.0) {
		t.Errorf("data[3] = %v, want 4.0", b.data[3])
	}
}

// TestBufferWriteCapacityOverflow verifies that exceeding capacity creates and
// links a new buffer rather than panicking or silently dropping data.
func TestBufferWriteCapacityOverflow(t *testing.T) {
	// Cap=2 so the third write must overflow into a new buffer.
	b := &buffer{data: make([]schema.Float, 0, 2), frequency: 10, start: 95}

	nb, _ := b.write(100, schema.Float(1.0))
	nb, _ = nb.write(110, schema.Float(2.0))
	nb, err := nb.write(120, schema.Float(3.0))
	if err != nil {
		t.Fatalf("write() error = %v", err)
	}
	if nb == b {
		t.Fatal("write() should have returned a new buffer after overflow")
	}
	if nb.prev != b {
		t.Error("new buffer should link back to old via prev")
	}
	if b.next != nb {
		t.Error("old buffer should link forward to new via next")
	}
	if len(b.data) != 2 {
		t.Errorf("old buffer len = %d, want 2 (full)", len(b.data))
	}
	if nb.data[0] != schema.Float(3.0) {
		t.Errorf("new buffer data[0] = %v, want 3.0", nb.data[0])
	}
}

// TestBufferWriteOverwrite verifies that writing to an already-occupied index
// replaces the value rather than appending.
func TestBufferWriteOverwrite(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))

	// Overwrite the first slot.
	b.write(100, schema.Float(99.0))
	if len(b.data) != 2 {
		t.Errorf("len(data) after overwrite = %d, want 2 (no append)", len(b.data))
	}
	if b.data[0] != schema.Float(99.0) {
		t.Errorf("data[0] after overwrite = %v, want 99.0", b.data[0])
	}
}

// ─── Buffer read ──────────────────────────────────────────────────────────────

// TestBufferReadBeforeFirstWrite verifies that 'from' is clamped to firstWrite
// when the requested range starts before any data in the chain.
func TestBufferReadBeforeFirstWrite(t *testing.T) {
	b := newBuffer(100, 10) // firstWrite = 100
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))

	data := make([]schema.Float, 10)
	result, adjustedFrom, _, err := b.read(50, 120, data)
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if adjustedFrom != 100 {
		t.Errorf("adjustedFrom = %d, want 100 (clamped to firstWrite)", adjustedFrom)
	}
	if len(result) != 2 {
		t.Errorf("len(result) = %d, want 2", len(result))
	}
}

// TestBufferReadChain verifies that read() traverses a multi-buffer chain and
// returns contiguous values from both buffers.
//
// The switch to b.next in read() triggers on idx >= cap(b.data), so b1 must
// be full (len == cap) for the loop to advance to b2 without producing NaN.
func TestBufferReadChain(t *testing.T) {
	// b1: cap=3, covers t=100..120.  b2: covers t=130..150.  b2 is head.
	b1 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 95}
	b1.data = append(b1.data, 1.0, 2.0, 3.0) // fills b1: len=cap=3

	b2 := &buffer{data: make([]schema.Float, 0, 3), frequency: 10, start: 125}
	b2.data = append(b2.data, 4.0, 5.0, 6.0) // t=130,140,150
	b2.prev = b1
	b1.next = b2

	data := make([]schema.Float, 6)
	result, from, to, err := b2.read(100, 160, data)
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if from != 100 || to != 160 {
		t.Errorf("read() from/to = %d/%d, want 100/160", from, to)
	}
	if len(result) != 6 {
		t.Fatalf("len(result) = %d, want 6", len(result))
	}
	for i, want := range []schema.Float{1, 2, 3, 4, 5, 6} {
		if result[i] != want {
			t.Errorf("result[%d] = %v, want %v", i, result[i], want)
		}
	}
}

// TestBufferReadIdxAfterSwitch is a regression test for the index recalculation
// bug after switching to b.next during a read.
//
// When both buffers share the same start time (can happen with checkpoint-loaded
// chains), the old code hardcoded idx=0 after the switch, causing reads at time t
// to return the wrong element from the next buffer.
func TestBufferReadIdxAfterSwitch(t *testing.T) {
	// b1: cap=2, both buffers start at 0 (firstWrite=5).
	// b1 carries t=5 and t=15; b2 carries t=5,15,25,35 with the same start.
	// When reading reaches t=25 the loop overflows b1 (idx=2 >= cap=2) and
	// switches to b2. The correct index in b2 is (25-0)/10=2 → b2.data[2]=30.0.
	// The old code set idx=0 → b2.data[0]=10.0 (wrong).
	b1 := &buffer{data: make([]schema.Float, 0, 2), frequency: 10, start: 0}
	b1.data = append(b1.data, schema.Float(1.0), schema.Float(2.0)) // t=5, t=15

	b2 := &buffer{data: make([]schema.Float, 0, 10), frequency: 10, start: 0}
	b2.data = append(b2.data,
		schema.Float(10.0), schema.Float(20.0),
		schema.Float(30.0), schema.Float(40.0)) // t=5,15,25,35
	b2.prev = b1
	b1.next = b2

	// from=0 triggers the walkback to b1 (from < b2.firstWrite=5).
	// After clamping, the loop runs t=5,15,25,35.
	data := make([]schema.Float, 4)
	result, _, _, err := b2.read(0, 36, data)
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if len(result) < 3 {
		t.Fatalf("len(result) = %d, want >= 3", len(result))
	}
	if result[0] != schema.Float(1.0) {
		t.Errorf("result[0] (t=5) = %v, want 1.0 (from b1)", result[0])
	}
	if result[1] != schema.Float(2.0) {
		t.Errorf("result[1] (t=15) = %v, want 2.0 (from b1)", result[1])
	}
	// This is the critical assertion: old code returned 10.0 (b2.data[0]).
	if result[2] != schema.Float(30.0) {
		t.Errorf("result[2] (t=25) = %v, want 30.0 (idx recalculation fix)", result[2])
	}
}

// TestBufferReadNaNValues verifies that NaN slots written to the buffer are
// returned as NaN during read.
func TestBufferReadNaNValues(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))
	b.write(110, schema.NaN)
	b.write(120, schema.Float(3.0))

	data := make([]schema.Float, 3)
	result, _, _, err := b.read(100, 130, data)
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}
	if result[0] != schema.Float(1.0) {
		t.Errorf("result[0] = %v, want 1.0", result[0])
	}
	if !result[1].IsNaN() {
		t.Errorf("result[1] should be NaN, got %v", result[1])
	}
	if result[2] != schema.Float(3.0) {
		t.Errorf("result[2] = %v, want 3.0", result[2])
	}
}

// TestBufferReadAccumulation verifies the += accumulation pattern used for
// aggregation: values are added to whatever was already in the data slice.
func TestBufferReadAccumulation(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(3.0))
	b.write(110, schema.Float(5.0))

	// Pre-populate data slice (simulates a second metric being summed in).
	data := []schema.Float{2.0, 1.0, 0.0}
	result, _, _, err := b.read(100, 120, data)
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	// 2.0+3.0=5.0, 1.0+5.0=6.0
	if result[0] != schema.Float(5.0) {
		t.Errorf("result[0] = %v, want 5.0 (2+3)", result[0])
	}
	if result[1] != schema.Float(6.0) {
		t.Errorf("result[1] = %v, want 6.0 (1+5)", result[1])
	}
}

// ─── Buffer free ─────────────────────────────────────────────────────────────

// newTestPool swaps out the package-level bufferPool for a fresh isolated one
// and returns a cleanup function that restores the original.
func newTestPool(t *testing.T) *PersistentBufferPool {
	t.Helper()
	pool := NewPersistentBufferPool()
	saved := bufferPool
	bufferPool = pool
	t.Cleanup(func() { bufferPool = saved })
	return pool
}

// TestBufferFreeRetention verifies that free() removes buffers whose entire
// time range falls before the retention threshold and returns them to the pool.
func TestBufferFreeRetention(t *testing.T) {
	pool := newTestPool(t)

	// b1: firstWrite=5, end=25  b2: firstWrite=25, end=45  b3: firstWrite=45, end=65
	b1 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 0}
	b1.data = append(b1.data, 1.0, 2.0)

	b2 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 20}
	b2.data = append(b2.data, 3.0, 4.0)
	b2.prev = b1
	b1.next = b2

	b3 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 40}
	b3.data = append(b3.data, 5.0, 6.0)
	b3.prev = b2
	b2.next = b3

	// Threshold=30: b1.end()=25 < 30 → freed; b2.end()=45 >= 30 → kept.
	delme, n := b3.free(30)
	if delme {
		t.Error("head buffer b3 should not be marked for deletion")
	}
	if n != 1 {
		t.Errorf("freed count = %d, want 1", n)
	}
	if b2.prev != nil {
		t.Error("b1 should have been unlinked from b2.prev")
	}
	if b3.prev != b2 {
		t.Error("b3 should still reference b2")
	}
	if pool.GetSize() != 1 {
		t.Errorf("pool size = %d, want 1 (b1 returned)", pool.GetSize())
	}
}

// TestBufferFreeAll verifies that free() removes all buffers and signals the
// caller to delete the head when the entire chain is older than the threshold.
func TestBufferFreeAll(t *testing.T) {
	pool := newTestPool(t)

	b1 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 0}
	b1.data = append(b1.data, 1.0, 2.0) // end=25

	b2 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 20}
	b2.data = append(b2.data, 3.0, 4.0) // end=45
	b2.prev = b1
	b1.next = b2

	// Threshold=100 > both ends → both should be freed.
	delme, n := b2.free(100)
	if !delme {
		t.Error("head buffer b2 should be marked for deletion when all data is stale")
	}
	if n != 2 {
		t.Errorf("freed count = %d, want 2", n)
	}
	// b1 was freed inside free(); b2 is returned with delme=true for the caller.
	if pool.GetSize() != 1 {
		t.Errorf("pool size = %d, want 1 (b1 returned; b2 returned by caller)", pool.GetSize())
	}
}

// ─── forceFreeOldest ─────────────────────────────────────────────────────────

// TestForceFreeOldestPoolReturn verifies that forceFreeOldest() returns the
// freed buffer to the pool (regression: previously it was just dropped).
func TestForceFreeOldestPoolReturn(t *testing.T) {
	pool := newTestPool(t)

	b1 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 0}
	b2 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 20}
	b3 := &buffer{data: make([]schema.Float, 0, BufferCap), frequency: 10, start: 40}
	b1.data = append(b1.data, 1.0)
	b2.data = append(b2.data, 2.0)
	b3.data = append(b3.data, 3.0)
	b2.prev = b1
	b1.next = b2
	b3.prev = b2
	b2.next = b3

	delme, n := b3.forceFreeOldest()
	if delme {
		t.Error("head b3 should not be marked for deletion (chain has 3 buffers)")
	}
	if n != 1 {
		t.Errorf("freed count = %d, want 1", n)
	}
	if b2.prev != nil {
		t.Error("b1 should have been unlinked from b2.prev after forceFreeOldest")
	}
	if b3.prev != b2 {
		t.Error("b3 should still link to b2")
	}
	if pool.GetSize() != 1 {
		t.Errorf("pool size = %d, want 1 (b1 returned to pool)", pool.GetSize())
	}
}

// TestForceFreeOldestSingleBuffer verifies that forceFreeOldest() returns
// delme=true when the buffer is the only one in the chain.
func TestForceFreeOldestSingleBuffer(t *testing.T) {
	b := newBuffer(100, 10)
	b.write(100, schema.Float(1.0))

	delme, n := b.forceFreeOldest()
	if !delme {
		t.Error("single-buffer chain: expected delme=true (the buffer IS the oldest)")
	}
	if n != 1 {
		t.Errorf("freed count = %d, want 1", n)
	}
}

// ─── iterFromTo ───────────────────────────────────────────────────────────────

// TestBufferIterFromToOrder verifies that iterFromTo invokes the callback in
// chronological order (oldest → newest).
func TestBufferIterFromToOrder(t *testing.T) {
	// Each buffer has 2 data points so end() = firstWrite + 2*freq.
	b1 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 0}  // end=25
	b2 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 20} // end=45
	b3 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 40} // end=65
	b2.prev = b1
	b1.next = b2
	b3.prev = b2
	b2.next = b3

	var order []*buffer
	err := b3.iterFromTo(0, 100, func(b *buffer) error {
		order = append(order, b)
		return nil
	})
	if err != nil {
		t.Fatalf("iterFromTo() error = %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("callback count = %d, want 3", len(order))
	}
	if order[0] != b1 || order[1] != b2 || order[2] != b3 {
		t.Error("iterFromTo() did not call callbacks in chronological (oldest→newest) order")
	}
}

// TestBufferIterFromToFiltered verifies that iterFromTo only calls the callback
// for buffers whose time range overlaps [from, to].
func TestBufferIterFromToFiltered(t *testing.T) {
	// b1: end=25  b2: start=20, end=45  b3: start=40, end=65
	b1 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 0}
	b2 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 20}
	b3 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 40}
	b2.prev = b1
	b1.next = b2
	b3.prev = b2
	b2.next = b3

	// [30,50]: b1.end=25 < 30 → excluded; b2 and b3 overlap → included.
	var visited []*buffer
	b3.iterFromTo(30, 50, func(b *buffer) error {
		visited = append(visited, b)
		return nil
	})
	if len(visited) != 2 {
		t.Fatalf("visited count = %d, want 2 (b2 and b3)", len(visited))
	}
	if visited[0] != b2 || visited[1] != b3 {
		t.Errorf("visited = %v, want [b2, b3]", visited)
	}
}

// TestBufferIterFromToNilBuffer verifies that iterFromTo on a nil buffer is a
// safe no-op.
func TestBufferIterFromToNilBuffer(t *testing.T) {
	var b *buffer
	called := false
	err := b.iterFromTo(0, 100, func(_ *buffer) error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("iterFromTo(nil) error = %v, want nil", err)
	}
	if called {
		t.Error("callback should not be called for a nil buffer")
	}
}

// ─── count ────────────────────────────────────────────────────────────────────

// TestBufferCount verifies that count() sums data-point lengths across the
// entire chain, including all prev links.
func TestBufferCount(t *testing.T) {
	b1 := &buffer{data: make([]schema.Float, 3, BufferCap), frequency: 10, start: 0}
	b2 := &buffer{data: make([]schema.Float, 2, BufferCap), frequency: 10, start: 35}
	b3 := &buffer{data: make([]schema.Float, 5, BufferCap), frequency: 10, start: 60}
	b2.prev = b1
	b1.next = b2
	b3.prev = b2
	b2.next = b3

	if got := b3.count(); got != 10 {
		t.Errorf("count() = %d, want 10 (3+2+5)", got)
	}

	// Single buffer.
	lone := &buffer{data: make([]schema.Float, 7, BufferCap)}
	if got := lone.count(); got != 7 {
		t.Errorf("count() single buffer = %d, want 7", got)
	}
}

// ─── Existing tests below ────────────────────────────────────────────────────

func TestAssignAggregationStrategy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AggregationStrategy
		wantErr  bool
	}{
		{"empty string", "", NoAggregation, false},
		{"sum", "sum", SumAggregation, false},
		{"avg", "avg", AvgAggregation, false},
		{"invalid", "invalid", NoAggregation, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AssignAggregationStrategy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("AssignAggregationStrategy(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("AssignAggregationStrategy(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBufferWrite(t *testing.T) {
	b := newBuffer(100, 10)

	// Test writing value
	nb, err := b.write(100, schema.Float(42.0))
	if err != nil {
		t.Errorf("buffer.write() error = %v", err)
	}
	if nb != b {
		t.Error("buffer.write() created new buffer unexpectedly")
	}
	if len(b.data) != 1 {
		t.Errorf("buffer.write() len(data) = %d, want 1", len(b.data))
	}
	if b.data[0] != schema.Float(42.0) {
		t.Errorf("buffer.write() data[0] = %v, want 42.0", b.data[0])
	}

	// Test writing value from past (should error)
	_, err = b.write(50, schema.Float(10.0))
	if err == nil {
		t.Error("buffer.write() expected error for past timestamp")
	}
}

func TestBufferRead(t *testing.T) {
	b := newBuffer(100, 10)

	// Write some test data
	b.write(100, schema.Float(1.0))
	b.write(110, schema.Float(2.0))
	b.write(120, schema.Float(3.0))

	// Read data
	data := make([]schema.Float, 3)
	result, from, to, err := b.read(100, 130, data)
	if err != nil {
		t.Errorf("buffer.read() error = %v", err)
	}
	// Buffer read should return from as firstWrite (start + freq/2)
	if from != 100 {
		t.Errorf("buffer.read() from = %d, want 100", from)
	}
	if to != 130 {
		t.Errorf("buffer.read() to = %d, want 130", to)
	}
	if len(result) != 3 {
		t.Errorf("buffer.read() len(result) = %d, want 3", len(result))
	}
}

func TestHealthCheck(t *testing.T) {
	// Create a test MemoryStore with some metrics
	metrics := map[string]MetricConfig{
		"load":       {Frequency: 10, Aggregation: AvgAggregation, offset: 0},
		"mem_used":   {Frequency: 10, Aggregation: AvgAggregation, offset: 1},
		"cpu_user":   {Frequency: 10, Aggregation: AvgAggregation, offset: 2},
		"cpu_system": {Frequency: 10, Aggregation: AvgAggregation, offset: 3},
	}

	ms := &MemoryStore{
		Metrics: metrics,
		root: Level{
			metrics:  make([]*buffer, len(metrics)),
			children: make(map[string]*Level),
		},
	}

	// Use recent timestamps (current time minus a small offset)
	now := time.Now().Unix()
	startTime := now - 100 // Start 100 seconds ago to have enough data points

	// Setup test data for node001 - all metrics healthy (recent data)
	node001 := ms.root.findLevelOrCreate([]string{"testcluster", "node001"}, len(metrics))
	for i := 0; i < len(metrics); i++ {
		node001.metrics[i] = newBuffer(startTime, 10)
		// Write recent data up to now
		for ts := startTime; ts <= now; ts += 10 {
			node001.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}

	// Setup test data for node002 - some metrics stale (old data beyond MaxMissingDataPoints threshold)
	node002 := ms.root.findLevelOrCreate([]string{"testcluster", "node002"}, len(metrics))
	// MaxMissingDataPoints = 5, frequency = 10, so threshold is 50 seconds
	staleTime := now - 100 // Data ends 100 seconds ago (well beyond 50 second threshold)
	for i := 0; i < len(metrics); i++ {
		node002.metrics[i] = newBuffer(staleTime-50, 10)
		if i < 2 {
			// First two metrics: healthy (recent data)
			for ts := startTime; ts <= now; ts += 10 {
				node002.metrics[i].write(ts, schema.Float(float64(i+1)))
			}
		} else {
			// Last two metrics: stale (data ends 100 seconds ago)
			for ts := staleTime - 50; ts <= staleTime; ts += 10 {
				node002.metrics[i].write(ts, schema.Float(float64(i+1)))
			}
		}
	}

	// Setup test data for node003 - some metrics missing (no buffer)
	node003 := ms.root.findLevelOrCreate([]string{"testcluster", "node003"}, len(metrics))
	// Only create buffers for first two metrics
	for i := range 2 {
		node003.metrics[i] = newBuffer(startTime, 10)
		for ts := startTime; ts <= now; ts += 10 {
			node003.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}
	// Leave metrics[2] and metrics[3] as nil (missing)

	// Setup test data for node005 - all metrics stale
	node005 := ms.root.findLevelOrCreate([]string{"testcluster", "node005"}, len(metrics))
	for i := 0; i < len(metrics); i++ {
		node005.metrics[i] = newBuffer(staleTime-50, 10)
		// All metrics have stale data (ends 100 seconds ago)
		for ts := staleTime - 50; ts <= staleTime; ts += 10 {
			node005.metrics[i].write(ts, schema.Float(float64(i+1)))
		}
	}

	// node004 doesn't exist at all

	tests := []struct {
		name            string
		cluster         string
		nodes           []string
		expectedMetrics []string
		wantStates      map[string]schema.MonitoringState
	}{
		{
			name:            "all metrics healthy",
			cluster:         "testcluster",
			nodes:           []string{"node001"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node001": schema.MonitoringStateFull,
			},
		},
		{
			name:            "some metrics stale",
			cluster:         "testcluster",
			nodes:           []string{"node002"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node002": schema.MonitoringStatePartial,
			},
		},
		{
			name:            "some metrics missing",
			cluster:         "testcluster",
			nodes:           []string{"node003"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node003": schema.MonitoringStatePartial,
			},
		},
		{
			name:            "node not found",
			cluster:         "testcluster",
			nodes:           []string{"node004"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node004": schema.MonitoringStateFailed,
			},
		},
		{
			name:            "all metrics stale",
			cluster:         "testcluster",
			nodes:           []string{"node005"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user", "cpu_system"},
			wantStates: map[string]schema.MonitoringState{
				"node005": schema.MonitoringStateFailed,
			},
		},
		{
			name:            "multiple nodes mixed states",
			cluster:         "testcluster",
			nodes:           []string{"node001", "node002", "node003", "node004", "node005"},
			expectedMetrics: []string{"load", "mem_used"},
			wantStates: map[string]schema.MonitoringState{
				"node001": schema.MonitoringStateFull,
				"node002": schema.MonitoringStateFull,   // Only checking first 2 metrics which are healthy
				"node003": schema.MonitoringStateFull,   // Only checking first 2 metrics which exist
				"node004": schema.MonitoringStateFailed, // Node doesn't exist
				"node005": schema.MonitoringStateFailed, // Both metrics are stale
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := ms.HealthCheck(tt.cluster, tt.nodes, tt.expectedMetrics)
			if err != nil {
				t.Errorf("HealthCheck() error = %v", err)
				return
			}

			// Check that we got results for all nodes
			if len(results) != len(tt.nodes) {
				t.Errorf("HealthCheck() returned %d results, want %d", len(results), len(tt.nodes))
			}

			// Check each node's state
			for _, node := range tt.nodes {
				state, ok := results[node]
				if !ok {
					t.Errorf("HealthCheck() missing result for node %s", node)
					continue
				}

				// Check status
				if wantStatus, ok := tt.wantStates[node]; ok {
					if state.State != wantStatus {
						t.Errorf("HealthCheck() node %s status = %v, want %v", node, state.State, wantStatus)
					}
				}
			}
		})
	}
}

// TestGetHealthyMetrics tests the GetHealthyMetrics function which returns lists of missing and degraded metrics
func TestGetHealthyMetrics(t *testing.T) {
	metrics := map[string]MetricConfig{
		"load":     {Frequency: 10, Aggregation: AvgAggregation, offset: 0},
		"mem_used": {Frequency: 10, Aggregation: AvgAggregation, offset: 1},
		"cpu_user": {Frequency: 10, Aggregation: AvgAggregation, offset: 2},
	}

	ms := &MemoryStore{
		Metrics: metrics,
		root: Level{
			metrics:  make([]*buffer, len(metrics)),
			children: make(map[string]*Level),
		},
	}

	now := time.Now().Unix()
	startTime := now - 100
	staleTime := now - 100

	// Setup node with mixed health states
	node := ms.root.findLevelOrCreate([]string{"testcluster", "testnode"}, len(metrics))

	// Metric 0 (load): healthy - recent data
	node.metrics[0] = newBuffer(startTime, 10)
	for ts := startTime; ts <= now; ts += 10 {
		node.metrics[0].write(ts, schema.Float(1.0))
	}

	// Metric 1 (mem_used): degraded - stale data
	node.metrics[1] = newBuffer(staleTime-50, 10)
	for ts := staleTime - 50; ts <= staleTime; ts += 10 {
		node.metrics[1].write(ts, schema.Float(2.0))
	}

	// Metric 2 (cpu_user): missing - no buffer (nil)

	tests := []struct {
		name            string
		selector        []string
		expectedMetrics []string
		wantDegraded    []string
		wantMissing     []string
		wantErr         bool
	}{
		{
			name:            "mixed health states",
			selector:        []string{"testcluster", "testnode"},
			expectedMetrics: []string{"load", "mem_used", "cpu_user"},
			wantDegraded:    []string{"mem_used"},
			wantMissing:     []string{"cpu_user"},
			wantErr:         false,
		},
		{
			name:            "node not found",
			selector:        []string{"testcluster", "nonexistent"},
			expectedMetrics: []string{"load"},
			wantDegraded:    nil,
			wantMissing:     nil,
			wantErr:         true,
		},
		{
			name:            "check only healthy metric",
			selector:        []string{"testcluster", "testnode"},
			expectedMetrics: []string{"load"},
			wantDegraded:    []string{},
			wantMissing:     []string{},
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			degraded, missing, err := ms.GetHealthyMetrics(tt.selector, tt.expectedMetrics)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetHealthyMetrics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Check degraded list
			if len(degraded) != len(tt.wantDegraded) {
				t.Errorf("GetHealthyMetrics() degraded = %v, want %v", degraded, tt.wantDegraded)
			} else {
				for i, d := range tt.wantDegraded {
					if degraded[i] != d {
						t.Errorf("GetHealthyMetrics() degraded[%d] = %v, want %v", i, degraded[i], d)
					}
				}
			}

			// Check missing list
			if len(missing) != len(tt.wantMissing) {
				t.Errorf("GetHealthyMetrics() missing = %v, want %v", missing, tt.wantMissing)
			} else {
				for i, m := range tt.wantMissing {
					if missing[i] != m {
						t.Errorf("GetHealthyMetrics() missing[%d] = %v, want %v", i, missing[i], m)
					}
				}
			}
		})
	}
}

// TestBufferHealthChecks tests the buffer-level health check functions
func TestBufferHealthChecks(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name        string
		setupBuffer func() *buffer
		wantExists  bool
		wantHealthy bool
		description string
	}{
		{
			name: "nil buffer",
			setupBuffer: func() *buffer {
				return nil
			},
			wantExists:  false,
			wantHealthy: false,
			description: "nil buffer should not exist and not be healthy",
		},
		{
			name: "empty buffer",
			setupBuffer: func() *buffer {
				b := newBuffer(now, 10)
				b.data = nil
				return b
			},
			wantExists:  false,
			wantHealthy: false,
			description: "empty buffer should not exist and not be healthy",
		},
		{
			name: "healthy buffer with recent data",
			setupBuffer: func() *buffer {
				b := newBuffer(now-30, 10)
				// Write data up to now (within MaxMissingDataPoints * frequency = 50 seconds)
				for ts := now - 30; ts <= now; ts += 10 {
					b.write(ts, schema.Float(1.0))
				}
				return b
			},
			wantExists:  true,
			wantHealthy: true,
			description: "buffer with recent data should be healthy",
		},
		{
			name: "stale buffer beyond threshold",
			setupBuffer: func() *buffer {
				b := newBuffer(now-200, 10)
				// Write data that ends 100 seconds ago (beyond MaxMissingDataPoints * frequency = 50 seconds)
				for ts := now - 200; ts <= now-100; ts += 10 {
					b.write(ts, schema.Float(1.0))
				}
				return b
			},
			wantExists:  true,
			wantHealthy: false,
			description: "buffer with stale data should exist but not be healthy",
		},
		{
			name: "buffer at threshold boundary",
			setupBuffer: func() *buffer {
				b := newBuffer(now-50, 10)
				// Write data that ends exactly at threshold (MaxMissingDataPoints * frequency = 50 seconds)
				for ts := now - 50; ts <= now-50; ts += 10 {
					b.write(ts, schema.Float(1.0))
				}
				return b
			},
			wantExists:  true,
			wantHealthy: true,
			description: "buffer at threshold boundary should still be healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.setupBuffer()

			exists := b.bufferExists()
			if exists != tt.wantExists {
				t.Errorf("bufferExists() = %v, want %v: %s", exists, tt.wantExists, tt.description)
			}

			if b != nil && b.data != nil && len(b.data) > 0 {
				healthy := b.isBufferHealthy()
				if healthy != tt.wantHealthy {
					t.Errorf("isBufferHealthy() = %v, want %v: %s", healthy, tt.wantHealthy, tt.description)
				}
			}
		})
	}
}

func TestBufferPoolClean(t *testing.T) {
	// Use a fresh pool for testing
	pool := NewPersistentBufferPool()

	now := time.Now().Unix()

	// Create some buffers and put them in the pool with different lastUsed times
	b1 := &buffer{lastUsed: now - 3600, data: make([]schema.Float, 0)}   // 1 hour ago
	b2 := &buffer{lastUsed: now - 7200, data: make([]schema.Float, 0)}   // 2 hours ago
	b3 := &buffer{lastUsed: now - 180000, data: make([]schema.Float, 0)} // 50 hours ago
	b4 := &buffer{lastUsed: now - 200000, data: make([]schema.Float, 0)} // 55 hours ago
	b5 := &buffer{lastUsed: now, data: make([]schema.Float, 0)}

	pool.Put(b1)
	pool.Put(b2)
	pool.Put(b3)
	pool.Put(b4)
	pool.Put(b5)

	if pool.GetSize() != 5 {
		t.Fatalf("Expected pool size 5, got %d", pool.GetSize())
	}

	// Clean buffers older than 48 hours
	timeUpdate := time.Now().Add(-48 * time.Hour).Unix()
	pool.Clean(timeUpdate)

	// Expected: b1, b2, b5 should remain. b3, b4 should be cleaned.
	if pool.GetSize() != 3 {
		t.Fatalf("Expected pool size 3 after clean, got %d", pool.GetSize())
	}

	validBufs := map[int64]bool{
		b1.lastUsed: true,
		b2.lastUsed: true,
		b5.lastUsed: true,
	}

	for i := 0; i < 3; i++ {
		b := pool.Get()
		if !validBufs[b.lastUsed] {
			t.Errorf("Found unexpected buffer with lastUsed %d", b.lastUsed)
		}
	}

	if pool.GetSize() != 0 {
		t.Fatalf("Expected pool to be empty, got %d", pool.GetSize())
	}
}
