// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ClusterCockpit/cc-lib/schema"
)

// Default buffer capacity.
// `buffer.data` will only ever grow up to it's capacity and a new link
// in the buffer chain will be created if needed so that no copying
// of data or reallocation needs to happen on writes.
const (
	BufferCap int = 512
)

// So that we can reuse allocations
var bufferPool sync.Pool = sync.Pool{
	New: func() any {
		return &buffer{
			data: make([]schema.Float, 0, BufferCap),
		}
	},
}

var (
	ErrNoData           error = errors.New("[METRICSTORE]> no data for this metric/level")
	ErrDataDoesNotAlign error = errors.New("[METRICSTORE]> data from lower granularities does not align")
)

// Each metric on each level has it's own buffer.
// This is where the actual values go.
// If `cap(data)` is reached, a new buffer is created and
// becomes the new head of a buffer list.
type buffer struct {
	prev      *buffer
	next      *buffer
	data      []schema.Float
	frequency int64
	start     int64
	archived  bool
	closed    bool
}

func newBuffer(ts, freq int64) *buffer {
	b := bufferPool.Get().(*buffer)
	b.frequency = freq
	b.start = ts - (freq / 2)
	b.prev = nil
	b.next = nil
	b.archived = false
	b.closed = false
	b.data = b.data[:0]
	return b
}

// If a new buffer was created, the new head is returnd.
// Otherwise, the existing buffer is returnd.
// Normaly, only "newer" data should be written, but if the value would
// end up in the same buffer anyways it is allowed.
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
		b.close()
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

func (b *buffer) close() {}

/*
func (b *buffer) close() {
	if b.closed {
		return
	}

	b.closed = true
	n, sum, min, max := 0, 0., math.MaxFloat64, -math.MaxFloat64
	for _, x := range b.data {
		if x.IsNaN() {
			continue
		}

		n += 1
		f := float64(x)
		sum += f
		min = math.Min(min, f)
		max = math.Max(max, f)
	}

	b.statisticts.samples = n
	if n > 0 {
		b.statisticts.avg = Float(sum / float64(n))
		b.statisticts.min = Float(min)
		b.statisticts.max = Float(max)
	} else {
		b.statisticts.avg = NaN
		b.statisticts.min = NaN
		b.statisticts.max = NaN
	}
}
*/

// func interpolate(idx int, data []Float) Float {
// 	if idx == 0 || idx+1 == len(data) {
// 		return NaN
// 	}
// 	return (data[idx-1] + data[idx+1]) / 2.0
// }

// Return all known values from `from` to `to`. Gaps of information are represented as NaN.
// Simple linear interpolation is done between the two neighboring cells if possible.
// If values at the start or end are missing, instead of NaN values, the second and thrid
// return values contain the actual `from`/`to`.
// This function goes back the buffer chain if `from` is older than the currents buffer start.
// The loaded values are added to `data` and `data` is returned, possibly with a shorter length.
// If `data` is not long enough to hold all values, this function will panic!
func (b *buffer) read(from, to int64, data []schema.Float) ([]schema.Float, int64, int64, error) {
	if from < b.firstWrite() {
		if b.prev != nil {
			return b.prev.read(from, to, data)
		}
		from = b.firstWrite()
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
			idx = 0
		}

		if idx >= len(b.data) {
			if b.next == nil || to <= b.next.start {
				break
			}
			data[i] += schema.NaN
		} else if t < b.start {
			data[i] += schema.NaN
			// } else if b.data[idx].IsNaN() {
			// 	data[i] += interpolate(idx, b.data)
		} else {
			data[i] += b.data[idx]
		}
		i++
	}

	fmt.Printf("Given From : %d, To: %d\n", from, to)

	return data[:i], from, t, nil
}

// Returns true if this buffer needs to be freed.
func (b *buffer) free(t int64) (delme bool, n int) {
	if b.prev != nil {
		delme, m := b.prev.free(t)
		n += m
		if delme {
			b.prev.next = nil
			if cap(b.prev.data) == BufferCap {
				bufferPool.Put(b.prev)
			}
			b.prev = nil
		}
	}

	end := b.end()
	if end < t {
		return true, n + 1
	}

	return false, n
}

// Call `callback` on every buffer that contains data in the range from `from` to `to`.
func (b *buffer) iterFromTo(from, to int64, callback func(b *buffer) error) error {
	if b == nil {
		return nil
	}

	if err := b.prev.iterFromTo(from, to, callback); err != nil {
		return err
	}

	if from <= b.end() && b.start <= to {
		return callback(b)
	}

	return nil
}

func (b *buffer) count() int64 {
	res := int64(len(b.data))
	if b.prev != nil {
		res += b.prev.count()
	}
	return res
}
