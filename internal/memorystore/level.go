// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

import (
	"sync"
	"unsafe"

	"github.com/ClusterCockpit/cc-lib/util"
)

// Could also be called "node" as this forms a node in a tree structure.
// Called Level because "node" might be confusing here.
// Can be both a leaf or a inner node. In this tree structue, inner nodes can
// also hold data (in `metrics`).
type Level struct {
	children map[string]*Level
	metrics  []*buffer
	lock     sync.RWMutex
}

// Find the correct level for the given selector, creating it if
// it does not exist. Example selector in the context of the
// ClusterCockpit could be: []string{ "emmy", "host123", "cpu0" }.
// This function would probably benefit a lot from `level.children` beeing a `sync.Map`?
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

func (l *Level) free(t int64) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	n := 0
	for i, b := range l.metrics {
		if b != nil {
			delme, m := b.free(t)
			n += m
			if delme {
				if cap(b.data) == BufferCap {
					bufferPool.Put(b)
				}
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

func (l *Level) sizeInBytes() int64 {
	l.lock.RLock()
	defer l.lock.RUnlock()
	size := int64(0)

	for _, b := range l.metrics {
		if b != nil {
			size += b.count() * int64(unsafe.Sizeof(util.Float(0)))
		}
	}

	for _, child := range l.children {
		size += child.sizeInBytes()
	}

	return size
}

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
