// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"bytes"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Syslog priorities used by cc-lib's log level prefixes. They match the
// priorities journald reports, so both backends deliver the same numbers.
const (
	prioCrit  uint8 = 2
	prioErr   uint8 = 3
	prioWarn  uint8 = 4
	prioInfo  uint8 = 6
	prioDebug uint8 = 7
)

const (
	// minBufferSize is the smallest ring capacity accepted.
	minBufferSize = 100
	// slotBytes is the message capacity preallocated per ring slot. Records up
	// to this size are stored without allocating.
	slotBytes = 128
	// maxRecordBytes caps a single record so that a pathological log line
	// cannot blow up the buffer.
	maxRecordBytes = 4096
)

// record is one captured log line. msg is owned by the ring and reused as the
// buffer wraps, so it must never escape without being copied.
type record struct {
	tsMicros int64
	prio     uint8
	msg      []byte
}

// ring is a fixed-size circular buffer of log records.
//
// The append path runs inside log.Logger's own mutex, around a write to
// stderr that costs microseconds, so a single mutex shared by all levels is
// not a contention concern. It is held for a timestamp and a short memmove
// into the preallocated slot: no allocation in steady state, and records are
// strictly ordered by timestamp, which lets the read path stop early at the
// 'since' cutoff.
type ring struct {
	buf   []record
	arena []byte
	next  uint64 // total records ever appended
	mu    sync.Mutex
}

func newRing(capacity int) *ring {
	if capacity < minBufferSize {
		capacity = minBufferSize
	}

	r := &ring{
		buf:   make([]record, capacity),
		arena: make([]byte, capacity*slotBytes),
	}
	for i := range r.buf {
		lo := i * slotBytes
		r.buf[i].msg = r.arena[lo : lo : lo+slotBytes]
	}

	return r
}

// append stores a copy of msg. msg must not be retained by the caller.
func (r *ring) append(prio uint8, msg []byte) {
	if len(msg) > maxRecordBytes {
		msg = msg[:maxRecordBytes]
	}

	r.mu.Lock()
	slot := &r.buf[r.next%uint64(len(r.buf))]
	slot.tsMicros = time.Now().UnixMicro()
	slot.prio = prio
	slot.msg = append(slot.msg[:0], msg...)
	r.next++
	r.mu.Unlock()
}

// resize changes the capacity, keeping the most recent records.
func (r *ring) resize(capacity int) {
	if capacity < minBufferSize {
		capacity = minBufferSize
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if capacity == len(r.buf) {
		return
	}

	kept := r.countLocked()
	if kept > uint64(capacity) {
		kept = uint64(capacity)
	}

	buf := make([]record, capacity)
	arena := make([]byte, capacity*slotBytes)
	for i := range buf {
		lo := i * slotBytes
		buf[i].msg = arena[lo : lo : lo+slotBytes]
	}

	// Copy the newest 'kept' records in chronological order.
	for i := uint64(0); i < kept; i++ {
		old := &r.buf[(r.next-kept+i)%uint64(len(r.buf))]
		slot := &buf[i]
		slot.tsMicros = old.tsMicros
		slot.prio = old.prio
		slot.msg = append(slot.msg[:0], old.msg...)
	}

	r.buf = buf
	r.arena = arena
	r.next = kept
}

// countLocked returns the number of records currently retained.
func (r *ring) countLocked() uint64 {
	if r.next < uint64(len(r.buf)) {
		return r.next
	}
	return uint64(len(r.buf))
}

// collect returns the records matching q, oldest first, at most q.Lines of
// them. sinceMicros bounds the age of the oldest record returned.
func (r *ring) collect(q Query, sinceMicros int64) []Entry {
	limit := q.Lines
	if limit <= 0 {
		return []Entry{}
	}

	needle := strings.ToLower(q.Search)

	r.mu.Lock()
	defer r.mu.Unlock()

	total := r.countLocked()
	capacity := total
	if uint64(limit) < capacity {
		capacity = uint64(limit)
	}
	entries := make([]Entry, 0, capacity)

	// Walk newest to oldest so the 'since' cutoff can stop the scan.
	for i := uint64(0); i < total && len(entries) < limit; i++ {
		rec := &r.buf[(r.next-1-i)%uint64(len(r.buf))]
		if rec.tsMicros < sinceMicros {
			break
		}
		if q.Level >= 0 && int(rec.prio) > q.Level {
			continue
		}
		if needle != "" && !containsFold(rec.msg, needle) {
			continue
		}

		entries = append(entries, Entry{
			Timestamp: strconv.FormatInt(rec.tsMicros, 10),
			Priority:  int(rec.prio),
			Message:   string(rec.msg),
		})
	}

	slices.Reverse(entries)
	return entries
}

// containsFold reports whether hay contains needle, comparing ASCII letters
// case-insensitively. needle must already be lower case. Unlike
// strings.Contains on a lowered copy this does not allocate, which matters
// because a query scans the whole ring.
func containsFold(hay []byte, needle string) bool {
	n := len(needle)
	if n == 0 {
		return true
	}

	for i := 0; i+n <= len(hay); i++ {
		j := 0
		for ; j < n; j++ {
			c := hay[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != needle[j] {
				break
			}
		}
		if j == n {
			return true
		}
	}

	return false
}

// newline terminates every record written by log.Logger.
var newline = []byte("\n")

// levelSink is the io.Writer attached to one cclog logger. log.Logger emits
// exactly one Write per record, so one Write is one entry even if the message
// itself spans multiple lines.
type levelSink struct {
	r      *ring
	prefix []byte
	prio   uint8
}

func (s *levelSink) Write(p []byte) (int, error) {
	n := len(p)

	msg := p
	if len(s.prefix) > 0 && bytes.HasPrefix(msg, s.prefix) {
		// The level is kept in Entry.Priority and rendered as a badge, so the
		// textual prefix is redundant.
		msg = msg[len(s.prefix):]
	}
	msg = bytes.TrimSuffix(msg, newline)

	s.r.append(s.prio, msg)

	// io.MultiWriter reports a short write unless the full length is returned.
	return n, nil
}

// memorySource serves the in-process ring buffer.
type memorySource struct {
	r *ring
}

func (s *memorySource) Mode() Mode { return ModeMemory }

func (s *memorySource) Query(_ context.Context, q Query) ([]Entry, error) {
	sinceMicros := int64(math.MinInt64)
	if q.Since != "" {
		since, err := parseSince(q.Since, time.Now())
		if err != nil {
			return nil, err
		}
		sinceMicros = since.UnixMicro()
	}

	return s.r.collect(q, sinceMicros), nil
}
