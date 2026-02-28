// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstore provides buffer.go: Time-series data buffer implementation.
//
// # Buffer Architecture
//
// Each metric at each hierarchical level (cluster/host/cpu/etc.) uses a linked-list
// chain of fixed-size buffers to store time-series data. This design:
//
//   - Avoids reallocation/copying when growing (new links added instead)
//   - Enables efficient pooling (buffers returned to sync.Pool)
//   - Supports traversal back in time (via prev pointers)
//   - Maintains temporal ordering (newer data in later buffers)
//
// # Buffer Chain Example
//
//	[oldest buffer] <- prev -- [older] <- prev -- [newest buffer (head)]
//	  start=1000               start=1512           start=2024
//	  data=[v0...v511]         data=[v0...v511]     data=[v0...v42]
//
// When the head buffer reaches capacity (BufferCap = 512), a new buffer becomes
// the new head and the old head is linked via prev.
//
// # Pooling Strategy
//
// sync.Pool reduces GC pressure for the common case (BufferCap-sized allocations).
// Non-standard capacity buffers are not pooled (e.g., from checkpoint deserialization).
//
// # Time Alignment
//
// Timestamps are aligned to measurement frequency intervals:
//
//	index = (timestamp - buffer.start) / buffer.frequency
//	actualTime = buffer.start + (frequency / 2) + (index * frequency)
//
// Missing data points are represented as NaN values. The read() function performs
// linear interpolation where possible.
package metricstore

import (
	"errors"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// BufferCap is the default buffer capacity.
// buffer.data will only ever grow up to its capacity and a new link
// in the buffer chain will be created if needed so that no copying
// of data or reallocation needs to happen on writes.
const BufferCap int = DefaultBufferCapacity

// maxPoolSize caps the number of buffers held in the pool at any time.
// Prevents unbounded memory growth after large retention-cleanup bursts.
const maxPoolSize = 4096

// BufferPool is the global instance.
// It is initialized immediately when the package loads.
var bufferPool = NewPersistentBufferPool()

type PersistentBufferPool struct {
	pool []*buffer
	mu   sync.Mutex
}

// NewPersistentBufferPool creates a dynamic pool for buffers.
func NewPersistentBufferPool() *PersistentBufferPool {
	return &PersistentBufferPool{
		pool: make([]*buffer, 0),
	}
}

func (p *PersistentBufferPool) Get() *buffer {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.pool)
	if n == 0 {
		// Pool is empty, allocate a new one
		return &buffer{
			data: make([]schema.Float, 0, BufferCap),
		}
	}

	// Reuse existing buffer from the pool
	b := p.pool[n-1]
	p.pool[n-1] = nil // Avoid memory leak
	p.pool = p.pool[:n-1]
	return b
}

// Put returns b to the pool. The caller must set b.lastUsed = time.Now().Unix()
// before calling Put so that Clean() can evict idle entries correctly.
func (p *PersistentBufferPool) Put(b *buffer) {
	// Reset the buffer before putting it back
	b.data = b.data[:0]

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pool) >= maxPoolSize {
		// Pool is full; drop the buffer and let GC collect it.
		return
	}
	p.pool = append(p.pool, b)
}

// GetSize returns the exact number of buffers currently sitting in the pool.
func (p *PersistentBufferPool) GetSize() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pool)
}

// Clear drains all buffers currently in the pool, allowing the GC to collect them.
func (p *PersistentBufferPool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.pool {
		p.pool[i] = nil
	}
	p.pool = p.pool[:0]
}

// Clean removes buffers from the pool that haven't been used in the given duration.
// It uses a simple LRU approach based on the lastUsed timestamp.
func (p *PersistentBufferPool) Clean(threshold int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Filter in place, retaining only buffers returned to the pool recently enough.
	active := p.pool[:0]
	for _, b := range p.pool {
		if b.lastUsed >= threshold {
			active = append(active, b)
		}
	}

	// Nullify the rest to prevent memory leaks
	for i := len(active); i < len(p.pool); i++ {
		p.pool[i] = nil
	}

	p.pool = active
}

var (
	// ErrNoData indicates no time-series data exists for the requested metric/level.
	ErrNoData error = errors.New("[METRICSTORE]> no data for this metric/level")

	// ErrDataDoesNotAlign indicates that aggregated data from child scopes
	// does not align with the parent scope's expected timestamps/intervals.
	ErrDataDoesNotAlign error = errors.New("[METRICSTORE]> data from lower granularities does not align")
)

// buffer stores time-series data for a single metric at a specific hierarchical level.
//
// Buffers form doubly-linked chains ordered by time. When capacity is reached,
// a new buffer becomes the head and the old head is linked via prev/next.
//
// Fields:
//   - prev:      Link to older buffer in the chain (nil if this is oldest)
//   - next:      Link to newer buffer in the chain (nil if this is newest/head)
//   - data:      Time-series values (schema.Float supports NaN for missing data)
//   - frequency: Measurement interval in seconds
//   - start:     Start timestamp (adjusted by -frequency/2 for alignment)
//   - archived:  True if data has been persisted to disk archive
//   - closed:    True if buffer is no longer accepting writes
//
// Index calculation: index = (timestamp - start) / frequency
// Actual data timestamp: start + (frequency / 2) + (index * frequency)
type buffer struct {
	prev      *buffer
	next      *buffer
	data      []schema.Float
	frequency int64
	start     int64
	archived  bool
	closed    bool
	lastUsed  int64
}

func newBuffer(ts, freq int64) *buffer {
	b := bufferPool.Get()
	b.frequency = freq
	b.start = ts - (freq / 2)
	b.prev = nil
	b.next = nil
	b.archived = false
	b.closed = false
	b.data = b.data[:0]
	return b
}

// write appends a timestamped value to the buffer chain.
//
// Returns the head buffer (which may be newly created if capacity was reached).
// Timestamps older than the buffer's start are rejected. If the calculated index
// exceeds capacity, a new buffer is allocated and linked as the new head.
//
// Missing timestamps are automatically filled with NaN values to maintain alignment.
// Overwrites are allowed if the index is already within the existing data slice.
//
// Parameters:
//   - ts:    Unix timestamp in seconds
//   - value: Metric value (can be schema.NaN for missing data)
//
// Returns:
//   - *buffer: The new head buffer (same as b if no new buffer created)
//   - error:   Non-nil if timestamp is before buffer start
func (b *buffer) write(ts int64, value schema.Float) (*buffer, error) {
	if ts < b.start {
		return nil, errors.New("[METRICSTORE]> cannot write value to buffer from past")
	}

	// idx := int((ts - b.start + (b.frequency / 3)) / b.frequency)
	idx := int((ts - b.start) / b.frequency)
	if idx >= cap(b.data) {
		newbuf := newBuffer(ts, b.frequency)
		newbuf.prev = b
		b.next = newbuf
		b = newbuf
		idx = 0
	}

	// Overwriting value or writing value from past
	if idx < len(b.data) {
		b.data[idx] = value
		return b, nil
	}

	// Fill up unwritten slots with NaN
	for i := len(b.data); i < idx; i++ {
		b.data = append(b.data, schema.NaN)
	}

	b.data = append(b.data, value)
	return b, nil
}

func (b *buffer) end() int64 {
	return b.firstWrite() + int64(len(b.data))*b.frequency
}

func (b *buffer) firstWrite() int64 {
	return b.start + (b.frequency / 2)
}

// read retrieves time-series data from the buffer chain for the specified time range.
//
// Traverses the buffer chain backwards (via prev links) if 'from' precedes the current
// buffer's start. Missing data points are represented as NaN. Values are accumulated
// into the provided 'data' slice (using +=, so caller must zero-initialize if needed).
//
// The function adjusts the actual time range returned if data is unavailable at the
// boundaries (returned via adjusted from/to timestamps).
//
// Parameters:
//   - from: Start timestamp (Unix seconds)
//   - to:   End timestamp (Unix seconds, exclusive)
//   - data: Pre-allocated slice to accumulate results (must be large enough)
//
// Returns:
//   - []schema.Float: Slice of data (may be shorter than input 'data' slice)
//   - int64:          Actual start timestamp with available data
//   - int64:          Actual end timestamp (exclusive)
//   - error:          Non-nil on failure
//
// Panics if 'data' slice is too small to hold all values in [from, to).
func (b *buffer) read(from, to int64, data []schema.Float) ([]schema.Float, int64, int64, error) {
	// Walk back to the buffer that covers 'from', adjusting if we hit the oldest.
	for from < b.firstWrite() {
		if b.prev == nil {
			from = b.firstWrite()
			break
		}
		b = b.prev
	}

	i := 0
	t := from
	for ; t < to; t += b.frequency {
		idx := int((t - b.start) / b.frequency)
		if idx >= cap(b.data) {
			if b.next == nil {
				break
			}
			b = b.next
			// Recalculate idx in the new buffer; a gap between buffers may exist.
			idx = int((t - b.start) / b.frequency)
		}

		if idx >= len(b.data) {
			if b.next == nil || to <= b.next.start {
				break
			}
			data[i] += schema.NaN // NaN + anything = NaN; propagates missing data
		} else if t < b.start {
			data[i] += schema.NaN // gap before this buffer's first write
		} else {
			data[i] += b.data[idx]
		}
		i++
	}

	return data[:i], from, t, nil
}

// free removes buffers older than the specified timestamp from the chain.
//
// Recursively traverses backwards (via prev) and unlinks buffers whose end time
// is before the retention threshold. Freed buffers are returned to the pool if
// they have the standard capacity (BufferCap).
//
// Parameters:
//   - t: Retention threshold timestamp (Unix seconds)
//
// Returns:
//   - delme: True if the current buffer itself should be deleted by caller
//   - n:     Number of buffers freed in this subtree
func (b *buffer) free(t int64) (delme bool, n int) {
	if b.prev != nil {
		delme, m := b.prev.free(t)
		n += m
		if delme {
			b.prev.next = nil
			if cap(b.prev.data) != BufferCap {
				b.prev.data = make([]schema.Float, 0, BufferCap)
			}
			b.prev.lastUsed = time.Now().Unix()
			bufferPool.Put(b.prev)
			b.prev = nil
		}
	}

	end := b.end()
	if end < t {
		return true, n + 1
	}

	return false, n
}

// forceFreeOldest recursively finds the end of the linked list (the oldest buffer)
// and removes it.
// Returns:
//
//	delme: true if 'b' itself is the oldest and should be removed by the caller
//	n:     the number of buffers freed (will be 1 or 0)
func (b *buffer) forceFreeOldest() (delme bool, n int) {
	// If there is a previous buffer, recurse down to find the oldest
	if b.prev != nil {
		delPrev, freed := b.prev.forceFreeOldest()

		// If the previous buffer signals it should be deleted:
		if delPrev {
			b.prev.next = nil
			if cap(b.prev.data) != BufferCap {
				b.prev.data = make([]schema.Float, 0, BufferCap)
			}
			b.prev.lastUsed = time.Now().Unix()
			bufferPool.Put(b.prev)
			b.prev = nil
		}
		return false, freed
	}

	// If b.prev is nil, THIS buffer is the oldest.
	// We return true so the parent (or the Level loop) knows to delete reference to 'b'.
	return true, 1
}

// iterFromTo invokes callback on every buffer in the chain that overlaps [from, to].
//
// Traverses backwards (via prev) first, then processes current buffer if it overlaps
// the time range. Used for checkpoint/archive operations that need to serialize buffers
// within a specific time window.
//
// Parameters:
//   - from:     Start timestamp (Unix seconds, inclusive)
//   - to:       End timestamp (Unix seconds, inclusive)
//   - callback: Function to invoke on each overlapping buffer
//
// Returns:
//   - error: First error returned by callback, or nil if all succeeded
func (b *buffer) iterFromTo(from, to int64, callback func(b *buffer) error) error {
	if b == nil {
		return nil
	}

	// Collect overlapping buffers walking backwards (newest → oldest).
	var matching []*buffer
	for cur := b; cur != nil; cur = cur.prev {
		if from <= cur.end() && cur.start <= to {
			matching = append(matching, cur)
		}
	}

	// Invoke callback in chronological order (oldest → newest).
	for i := len(matching) - 1; i >= 0; i-- {
		if err := callback(matching[i]); err != nil {
			return err
		}
	}
	return nil
}

func (b *buffer) count() int64 {
	var res int64
	for ; b != nil; b = b.prev {
		res += int64(len(b.data))
	}
	return res
}
