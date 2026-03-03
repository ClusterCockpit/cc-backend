// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstore provides level.go: Hierarchical tree structure for metric storage.
//
// # Level Architecture
//
// The Level type forms a tree structure where each node represents a level in the
// ClusterCockpit hierarchy: cluster → host → socket → core → hwthread, with special
// nodes for memory domains and accelerators.
//
// Structure:
//
//	Root Level (cluster="emmy")
//	├─ Level (host="node001")
//	│  ├─ Level (socket="0")
//	│  │  ├─ Level (core="0") [stores cpu0 metrics]
//	│  │  └─ Level (core="1") [stores cpu1 metrics]
//	│  └─ Level (socket="1")
//	│     └─ ...
//	└─ Level (host="node002")
//	   └─ ...
//
// Each Level can:
//   - Hold data (metrics slice of buffer pointers)
//   - Have child nodes (children map[string]*Level)
//   - Both simultaneously (inner nodes can store aggregated metrics)
//
// # Selector Paths
//
// Selectors are hierarchical paths: []string{"cluster", "host", "component"}.
// Example: []string{"emmy", "node001", "cpu0"} navigates to the cpu0 core level.
//
// # Concurrency
//
// RWMutex protects children map and metrics slice. Read-heavy workload (metric reads)
// uses RLock. Writes (new levels, buffer updates) use Lock. Double-checked locking
// prevents races during level creation.
package metricstore

import (
	"sync"
	"time"
	"unsafe"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

// Level represents a node in the hierarchical metric storage tree.
//
// Can be both a leaf or inner node. Inner nodes hold data in 'metrics' for aggregated
// values (e.g., socket-level metrics derived from core-level data). Named "Level"
// instead of "node" to avoid confusion with cluster nodes (hosts).
//
// Fields:
//   - children: Map of child level names to Level pointers (e.g., "cpu0" → Level)
//   - metrics:  Slice of buffer pointers (one per metric, indexed by MetricConfig.offset)
//   - lock:     RWMutex for concurrent access (read-heavy, write-rare)
type Level struct {
	children map[string]*Level
	metrics  []*buffer
	lock     sync.RWMutex
}

// findLevelOrCreate navigates to or creates the level specified by selector.
//
// Recursively descends the tree, creating missing levels as needed. Uses double-checked
// locking: RLock first (fast path), then Lock if creation needed (slow path), then
// re-check after acquiring Lock to handle races.
//
// Example selector: []string{"emmy", "node001", "cpu0"}
// Navigates: root → emmy → node001 → cpu0, creating levels as needed.
//
// Parameters:
//   - selector: Hierarchical path (consumed recursively, decreasing depth)
//   - nMetrics: Number of metric slots to allocate in new levels
//
// Returns:
//   - *Level: The target level (existing or newly created)
//
// Note: sync.Map may improve performance for high-concurrency writes, but current
// approach suffices for read-heavy workload.
func (l *Level) findLevelOrCreate(selector []string, nMetrics int) *Level {
	if len(selector) == 0 {
		return l
	}

	// Allow concurrent reads:
	l.lock.RLock()
	var child *Level
	var ok bool
	if l.children == nil {
		// Children map needs to be created...
		l.lock.RUnlock()
	} else {
		child, ok = l.children[selector[0]]
		l.lock.RUnlock()
		if ok {
			return child.findLevelOrCreate(selector[1:], nMetrics)
		}
	}

	// The level does not exist, take write lock for unique access:
	l.lock.Lock()
	// While this thread waited for the write lock, another thread
	// could have created the child node.
	if l.children != nil {
		child, ok = l.children[selector[0]]
		if ok {
			l.lock.Unlock()
			return child.findLevelOrCreate(selector[1:], nMetrics)
		}
	}

	child = &Level{
		metrics:  make([]*buffer, nMetrics),
		children: nil,
	}

	if l.children != nil {
		l.children[selector[0]] = child
	} else {
		l.children = map[string]*Level{selector[0]: child}
	}
	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:], nMetrics)
}

// collectPaths gathers all selector paths at the specified depth in the tree.
//
// Recursively traverses children, collecting paths when currentDepth+1 == targetDepth.
// Each path is a selector that can be used with findLevel() or findBuffers().
//
// Explicitly copies slices to avoid shared underlying arrays between siblings, preventing
// unintended mutations.
//
// Parameters:
//   - currentDepth: Depth of current level (0 = root)
//   - targetDepth:  Depth to collect paths from
//   - currentPath:  Path accumulated so far
//   - results:      Output slice (appended to)
//
// Example: collectPaths(0, 2, []string{}, &results) collects all 2-level paths
// like []string{"emmy", "node001"}, []string{"emmy", "node002"}, etc.
func (l *Level) collectPaths(currentDepth, targetDepth int, currentPath []string, results *[][]string) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	for key, child := range l.children {
		if child == nil {
			continue
		}

		// We explicitly make a new slice and copy data to avoid sharing underlying arrays between siblings
		newPath := make([]string, len(currentPath))
		copy(newPath, currentPath)
		newPath = append(newPath, key)

		// Check depth, and just return if depth reached
		if currentDepth+1 == targetDepth {
			*results = append(*results, newPath)
		} else {
			child.collectPaths(currentDepth+1, targetDepth, newPath, results)
		}
	}
}

// free removes buffers older than the retention threshold from the entire subtree.
//
// Recursively frees buffers in this level's metrics and all child levels. Buffers
// with standard capacity (BufferCap) are returned to the pool. Called by the
// retention worker to enforce retention policies.
//
// Parameters:
//   - t: Retention threshold timestamp (Unix seconds)
//
// Returns:
//   - int:   Total number of buffers freed in this subtree
//   - error: Non-nil on failure (propagated from children)
func (l *Level) free(t int64) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	n := 0
	for i, b := range l.metrics {
		if b != nil {
			delme, m := b.free(t)
			n += m
			if delme {
				if cap(b.data) != BufferCap {
					b.data = make([]schema.Float, 0, BufferCap)
				}
				b.lastUsed = time.Now().Unix()
				bufferPool.Put(b)
				l.metrics[i] = nil
			}
		}
	}

	for _, l := range l.children {
		m, err := l.free(t)
		n += m
		if err != nil {
			return n, err
		}
	}

	return n, nil
}

// forceFree removes the oldest buffer from each metric chain in the subtree.
//
// Unlike free(), which removes based on time threshold, this unconditionally removes
// the oldest buffer in each chain. Used by MemoryUsageTracker when memory cap is
// exceeded and time-based retention is insufficient.
//
// Recursively processes current level's metrics and all child levels.
//
// Returns:
//   - int:   Total number of buffers freed in this subtree
//   - error: Non-nil on failure (propagated from children)
func (l *Level) forceFree() (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	n := 0

	// Iterate over metrics in the current level
	for i, b := range l.metrics {
		if b != nil {
			// Attempt to free the oldest buffer in this chain
			delme, freedCount := b.forceFreeOldest()
			n += freedCount

			// If delme is true, it means 'b' itself (the head) was the oldest
			// and needs to be removed from the slice.
			if delme {
				b.next = nil
				b.prev = nil
				if cap(b.data) != BufferCap {
					b.data = make([]schema.Float, 0, BufferCap)
				}
				b.lastUsed = time.Now().Unix()
				bufferPool.Put(b)
				l.metrics[i] = nil
			}
		}
	}

	// Recursively traverse children
	for _, child := range l.children {
		m, err := child.forceFree()
		n += m
		if err != nil {
			return n, err
		}
	}

	return n, nil
}

// sizeInBytes calculates the total memory usage of all buffers in the subtree.
//
// Recursively sums buffer data sizes (count of Float values × sizeof(Float)) across
// this level's metrics and all child levels. Used by MemoryUsageTracker to enforce
// memory cap limits.
//
// Returns:
//   - int64: Total bytes used by buffer data in this subtree
func (l *Level) sizeInBytes() int64 {
	l.lock.RLock()
	defer l.lock.RUnlock()
	size := int64(0)

	for _, b := range l.metrics {
		if b != nil {
			size += b.count() * int64(unsafe.Sizeof(schema.Float(0)))
		}
	}

	for _, child := range l.children {
		size += child.sizeInBytes()
	}

	return size
}

// findLevel navigates to the level specified by selector, returning nil if not found.
//
// Read-only variant of findLevelOrCreate. Does not create missing levels.
// Recursively descends the tree following the selector path.
//
// Parameters:
//   - selector: Hierarchical path (e.g., []string{"emmy", "node001", "cpu0"})
//
// Returns:
//   - *Level: The target level, or nil if any component in the path does not exist
func (l *Level) findLevel(selector []string) *Level {
	if len(selector) == 0 {
		return l
	}

	l.lock.RLock()
	defer l.lock.RUnlock()

	lvl := l.children[selector[0]]
	if lvl == nil {
		return nil
	}

	return lvl.findLevel(selector[1:])
}

// findBuffers invokes callback on all buffers matching the selector pattern.
//
// Supports flexible selector patterns (from cc-lib/util.Selector):
//   - Exact match: Selector element with String set (e.g., "node001")
//   - Group match: Selector element with Group set (e.g., ["cpu0", "cpu2", "cpu4"])
//   - Wildcard:    Selector element with Any=true (matches all children)
//
// Empty selector (len==0) matches current level's buffer at 'offset' and recursively
// all descendant buffers at the same offset (used for aggregation queries).
//
// Parameters:
//   - selector: Pattern to match (consumed recursively)
//   - offset:   Metric index in metrics slice (from MetricConfig.offset)
//   - f:        Callback invoked on each matching buffer
//
// Returns:
//   - error: First error returned by callback, or nil if all succeeded
//
// Example:
//
//	// Find all cpu0 buffers across all hosts:
//	findBuffers([]Selector{{Any: true}, {String: "cpu0"}}, metricOffset, callback)
func (l *Level) findBuffers(selector util.Selector, offset int, f func(b *buffer) error) error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	if len(selector) == 0 {
		b := l.metrics[offset]
		if b != nil {
			return f(b)
		}

		for _, lvl := range l.children {
			err := lvl.findBuffers(nil, offset, f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	sel := selector[0]
	if len(sel.String) != 0 && l.children != nil {
		lvl, ok := l.children[sel.String]
		if ok {
			err := lvl.findBuffers(selector[1:], offset, f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	if sel.Group != nil && l.children != nil {
		for _, key := range sel.Group {
			lvl, ok := l.children[key]
			if ok {
				err := lvl.findBuffers(selector[1:], offset, f)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	if sel.Any && l.children != nil {
		for _, lvl := range l.children {
			if err := lvl.findBuffers(selector[1:], offset, f); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}
