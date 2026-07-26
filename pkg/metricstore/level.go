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
	"slices"
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
	n, _, err := l.freeAndCheckEmpty(t)
	return n, err
}

// freeAndCheckEmpty performs free() and atomically checks if the level is empty
// while still holding its own lock, avoiding a TOCTOU race between free() and
// a separate isEmpty() call.
func (l *Level) freeAndCheckEmpty(t int64) (int, bool, error) {
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

	for key, child := range l.children {
		m, empty, err := child.freeAndCheckEmpty(t)
		n += m
		if err != nil {
			return n, false, err
		}
		if empty {
			delete(l.children, key)
		}
	}

	// Check emptiness while still holding the lock
	empty := len(l.children) == 0
	if empty {
		for _, b := range l.metrics {
			if b != nil {
				empty = false
				break
			}
		}
	}

	return n, empty, nil
}

// freeExcludingUsed frees buffers older than t across the whole tree except for
// nodes listed in used (cluster name -> sorted hostnames), and deletes node
// Levels that become empty. The receiver must be the root level. Returns the
// total number of buffers freed.
//
// This is the exclusion-aware counterpart of freeAndCheckEmpty used by the
// retention path when a NodeProvider reports nodes in use by running jobs:
// used nodes are never freed, so never empty, so never pruned. Holding the root
// write lock for the full pass matches the existing root free (ms.Free(nil, t)).
//
// Holding the root lock for the entire traversal is intentional: it serializes
// this pass against findLevelOrCreate (which RLocks root first), preventing a
// writer from grabbing a node pointer that this pass then deletes as empty,
// which would silently lose data. Do not "optimize" this to per-cluster locking.
func (l *Level) freeExcludingUsed(t int64, used map[string][]string) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	total := 0
	for cluster, clusterLvl := range l.children {
		n, empty, err := clusterLvl.freeNodesExcludingUsed(t, used[cluster])
		total += n
		if err != nil {
			return total, err
		}
		if empty {
			delete(l.children, cluster)
		}
	}
	return total, nil
}

// freeNodesExcludingUsed operates on a cluster level. For each node child whose
// name is not in usedHosts (sorted, per NodeProvider contract), it frees buffers
// older than t and deletes the node when it becomes empty. Returns the number of
// buffers freed and whether the cluster level itself is now empty.
func (l *Level) freeNodesExcludingUsed(t int64, usedHosts []string) (int, bool, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	n := 0
	for node, nodeLvl := range l.children {
		if _, found := slices.BinarySearch(usedHosts, node); found {
			continue // used node: preserve entirely
		}
		m, empty, err := nodeLvl.freeAndCheckEmpty(t)
		n += m
		if err != nil {
			return n, false, err
		}
		if empty {
			delete(l.children, node)
		}
	}

	// Cluster is empty only if it has no children and no buffers of its own
	// (inner levels may hold aggregated metrics).
	empty := len(l.children) == 0
	if empty {
		for _, b := range l.metrics {
			if b != nil {
				empty = false
				break
			}
		}
	}
	return n, empty, nil
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
			size += b.bufferCount() * int64(unsafe.Sizeof(buffer{}))
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
//   - f:        Callback invoked on each matching buffer; path holds the matched
//               level keys (e.g. ["cluster","node001","cpu0"]) so callers can
//               identify which node/component the buffer belongs to
//   - path:     Accumulated level keys from the root; pass nil at the top level
//
// Returns:
//   - error: First error returned by callback, or nil if all succeeded
//
// Example:
//
//	// Find all cpu0 buffers across all hosts:
//	findBuffers([]Selector{{Any: true}, {String: "cpu0"}}, metricOffset, callback, nil)
func (l *Level) findBuffers(selector util.Selector, offset int, f func(b *buffer, path []string) error, path []string) error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	if len(selector) == 0 {
		b := l.metrics[offset]
		if b != nil {
			return f(b, path)
		}

		for key, lvl := range l.children {
			err := lvl.findBuffers(nil, offset, f, appendPath(path, key))
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
			err := lvl.findBuffers(selector[1:], offset, f, appendPath(path, sel.String))
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
				err := lvl.findBuffers(selector[1:], offset, f, appendPath(path, key))
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	if sel.Any && l.children != nil {
		for key, lvl := range l.children {
			if err := lvl.findBuffers(selector[1:], offset, f, appendPath(path, key)); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

// appendPath returns a new slice holding path followed by key. It copies rather
// than appending in place so sibling recursions never share/overwrite backing
// storage while the tree is walked concurrently.
func appendPath(path []string, key string) []string {
	np := make([]string, len(path)+1)
	copy(np, path)
	np[len(path)] = key
	return np
}
