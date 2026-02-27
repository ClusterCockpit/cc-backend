// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file implements checkpoint persistence for the in-memory metric store.
//
// Checkpoints enable graceful restarts by periodically saving in-memory metric
// data to disk in JSON or binary format. The checkpoint system:
//
// Key Features:
//   - Periodic background checkpointing via the Checkpointing() worker
//   - Two format families: JSON (human-readable) and WAL+binary (compact, crash-safe)
//   - Parallel checkpoint creation and loading using worker pools
//   - Hierarchical file organization: checkpoint_dir/cluster/host/timestamp.{json|bin}
//   - WAL file: checkpoint_dir/cluster/host/current.wal (append-only, per-entry)
//   - Only saves unarchived data (archived data is already persisted elsewhere)
//   - GC optimization during loading to prevent excessive heap growth
//
// Checkpoint Workflow:
//  1. Init() loads checkpoints within retention window at startup
//  2. Checkpointing() worker periodically saves new data
//  3. Shutdown() writes final checkpoint before exit
//
// File Organization:
//
//	checkpoints/
//	  cluster1/
//	    host001/
//	      1234567890.json  (JSON format: full subtree snapshot)
//	      1234567890.bin   (binary format: full subtree snapshot)
//	      current.wal      (WAL format: append-only per-entry log)
//	    host002/
//	      ...
package metricstore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

const (
	CheckpointFilePerms = 0o644                    // File permissions for checkpoint files
	CheckpointDirPerms  = 0o755                    // Directory permissions for checkpoint directories
	GCTriggerInterval   = DefaultGCTriggerInterval // Interval for triggering GC during checkpoint loading
)

// CheckpointMetrics represents metric data in a checkpoint file.
// Whenever the structure changes, update MarshalJSON as well!
type CheckpointMetrics struct {
	Data      []schema.Float `json:"data"`
	Frequency int64          `json:"frequency"`
	Start     int64          `json:"start"`
}

// CheckpointFile represents the hierarchical structure of a checkpoint file.
// It mirrors the Level tree structure from the MemoryStore.
type CheckpointFile struct {
	Metrics  map[string]*CheckpointMetrics `json:"metrics"`
	Children map[string]*CheckpointFile    `json:"children"`
	From     int64                         `json:"from"`
	To       int64                         `json:"to"`
}

// lastCheckpoint tracks the timestamp of the last checkpoint creation.
var (
	lastCheckpoint   time.Time
	lastCheckpointMu sync.Mutex
)

// Checkpointing starts a background worker that periodically saves metric data to disk.
//
// Format behaviour:
//   - "json": Periodic checkpointing based on Keys.Checkpoints.Interval
//   - "wal":  Periodic binary snapshots + WAL rotation at Keys.Checkpoints.Interval
func Checkpointing(wg *sync.WaitGroup, ctx context.Context) {
	lastCheckpointMu.Lock()
	lastCheckpoint = time.Now()
	lastCheckpointMu.Unlock()

	ms := GetMemoryStore()

	wg.Go(func() {

		d, err := time.ParseDuration(Keys.Checkpoints.Interval)
		if err != nil {
			cclog.Fatalf("[METRICSTORE]> invalid checkpoint interval '%s': %s", Keys.Checkpoints.Interval, err.Error())
		}
		if d <= 0 {
			cclog.Warnf("[METRICSTORE]> checkpoint interval is zero or negative (%s), checkpointing disabled", d)
			return
		}

		ticker := time.NewTicker(d)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lastCheckpointMu.Lock()
				from := lastCheckpoint
				lastCheckpointMu.Unlock()

				now := time.Now()
				cclog.Infof("[METRICSTORE]> start checkpointing (starting at %s)...", from.Format(time.RFC3339))

				if Keys.Checkpoints.FileFormat == "wal" {
					n, hostDirs, err := ms.ToCheckpointWAL(Keys.Checkpoints.RootDir, from.Unix(), now.Unix())
					if err != nil {
						cclog.Errorf("[METRICSTORE]> binary checkpointing failed: %s", err.Error())
					} else {
						cclog.Infof("[METRICSTORE]> done: %d binary snapshot files created", n)
						lastCheckpointMu.Lock()
						lastCheckpoint = now
						lastCheckpointMu.Unlock()
						// Rotate WAL files for successfully checkpointed hosts.
						RotateWALFiles(hostDirs)
					}
				} else {
					n, err := ms.ToCheckpoint(Keys.Checkpoints.RootDir, from.Unix(), now.Unix())
					if err != nil {
						cclog.Errorf("[METRICSTORE]> checkpointing failed: %s", err.Error())
					} else {
						cclog.Infof("[METRICSTORE]> done: %d checkpoint files created", n)
						lastCheckpointMu.Lock()
						lastCheckpoint = now
						lastCheckpointMu.Unlock()
					}
				}
			}
		}
	})
}

// MarshalJSON provides optimized JSON encoding for CheckpointMetrics.
//
// Since schema.Float has custom MarshalJSON, serializing []Float has significant overhead.
// This method manually constructs JSON to avoid allocations and interface conversions.
func (cm *CheckpointMetrics) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, 128+len(cm.Data)*8)
	buf = append(buf, `{"frequency":`...)
	buf = strconv.AppendInt(buf, cm.Frequency, 10)
	buf = append(buf, `,"start":`...)
	buf = strconv.AppendInt(buf, cm.Start, 10)
	buf = append(buf, `,"data":[`...)
	for i, x := range cm.Data {
		if i != 0 {
			buf = append(buf, ',')
		}
		if x.IsNaN() {
			buf = append(buf, `null`...)
		} else {
			buf = strconv.AppendFloat(buf, float64(x), 'f', 1, 32)
		}
	}
	buf = append(buf, `]}`...)
	return buf, nil
}

// ToCheckpoint writes metric data to checkpoint files in parallel (JSON format).
//
// Metrics at root and cluster levels are skipped. One file per host is created.
// Uses worker pool (Keys.NumWorkers) for parallel processing. Only locks one host
// at a time, allowing concurrent writes/reads to other hosts.
//
// Returns the number of checkpoint files created and any errors encountered.
func (m *MemoryStore) ToCheckpoint(dir string, from, to int64) (int, error) {
	// Pre-calculate capacity by counting cluster/host pairs
	m.root.lock.RLock()
	totalHosts := 0
	for _, l1 := range m.root.children {
		l1.lock.RLock()
		totalHosts += len(l1.children)
		l1.lock.RUnlock()
	}
	m.root.lock.RUnlock()

	levels := make([]*Level, 0, totalHosts)
	selectors := make([][]string, 0, totalHosts)

	m.root.lock.RLock()
	for sel1, l1 := range m.root.children {
		l1.lock.RLock()
		for sel2, l2 := range l1.children {
			levels = append(levels, l2)
			selectors = append(selectors, []string{sel1, sel2})
		}
		l1.lock.RUnlock()
	}
	m.root.lock.RUnlock()

	type workItem struct {
		level    *Level
		dir      string
		selector []string
	}

	n, errs := int32(0), int32(0)

	var wg sync.WaitGroup
	wg.Add(Keys.NumWorkers)
	work := make(chan workItem, Keys.NumWorkers*2)
	for worker := 0; worker < Keys.NumWorkers; worker++ {
		go func() {
			defer wg.Done()

			for workItem := range work {
				if err := workItem.level.toCheckpoint(workItem.dir, from, to, m); err != nil {
					if err == ErrNoNewArchiveData {
						continue
					}

					cclog.Errorf("[METRICSTORE]> error while checkpointing %#v: %s", workItem.selector, err.Error())
					atomic.AddInt32(&errs, 1)
				} else {
					atomic.AddInt32(&n, 1)
				}
			}
		}()
	}

	for i := 0; i < len(levels); i++ {
		dir := path.Join(dir, path.Join(selectors[i]...))
		work <- workItem{
			level:    levels[i],
			dir:      dir,
			selector: selectors[i],
		}
	}

	close(work)
	wg.Wait()

	if errs > 0 {
		return int(n), fmt.Errorf("[METRICSTORE]> %d errors happened while creating checkpoints (%d successes)", errs, n)
	}
	return int(n), nil
}

// toCheckpointFile recursively converts a Level tree to CheckpointFile structure.
// Skips metrics that are already archived. Returns nil if no unarchived data exists.
func (l *Level) toCheckpointFile(from, to int64, m *MemoryStore) (*CheckpointFile, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	retval := &CheckpointFile{
		From:     from,
		To:       to,
		Metrics:  make(map[string]*CheckpointMetrics),
		Children: make(map[string]*CheckpointFile),
	}

	for metric, minfo := range m.Metrics {
		b := l.metrics[minfo.offset]
		if b == nil {
			continue
		}

		allArchived := true
		b.iterFromTo(from, to, func(b *buffer) error {
			if !b.archived {
				allArchived = false
				return fmt.Errorf("stop") // Early termination signal
			}
			return nil
		})

		if allArchived {
			continue
		}

		data := make([]schema.Float, (to-from)/b.frequency+1)
		data, start, end, err := b.read(from, to, data)
		if err != nil {
			return nil, err
		}

		for i := int((end - start) / b.frequency); i < len(data); i++ {
			data[i] = schema.NaN
		}

		retval.Metrics[metric] = &CheckpointMetrics{
			Frequency: b.frequency,
			Start:     start,
			Data:      data,
		}
	}

	for name, child := range l.children {
		val, err := child.toCheckpointFile(from, to, m)
		if err != nil {
			return nil, err
		}

		if val != nil {
			retval.Children[name] = val
		}
	}

	if len(retval.Children) == 0 && len(retval.Metrics) == 0 {
		return nil, nil
	}

	return retval, nil
}

// toCheckpoint writes a Level's data to a JSON checkpoint file.
// Creates directory if needed. Returns ErrNoNewArchiveData if nothing to save.
func (l *Level) toCheckpoint(dir string, from, to int64, m *MemoryStore) error {
	cf, err := l.toCheckpointFile(from, to, m)
	if err != nil {
		return err
	}

	if cf == nil {
		return ErrNoNewArchiveData
	}

	filepath := path.Join(dir, fmt.Sprintf("%d.json", from))
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, CheckpointDirPerms)
		if err == nil {
			f, err = os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
		}
	}
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	if err = json.NewEncoder(bw).Encode(cf); err != nil {
		return err
	}

	return bw.Flush()
}

// enqueueCheckpointHosts traverses checkpoint directory and enqueues cluster/host pairs.
// Returns error if directory structure is invalid.
func enqueueCheckpointHosts(dir string, work chan<- [2]string) error {
	clustersDir, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, clusterDir := range clustersDir {
		if !clusterDir.IsDir() {
			return errors.New("[METRICSTORE]> expected only directories at first level of checkpoints/ directory")
		}

		hostsDir, err := os.ReadDir(filepath.Join(dir, clusterDir.Name()))
		if err != nil {
			return err
		}

		for _, hostDir := range hostsDir {
			if !hostDir.IsDir() {
				return errors.New("[METRICSTORE]> expected only directories at second level of checkpoints/ directory")
			}

			work <- [2]string{clusterDir.Name(), hostDir.Name()}
		}
	}

	return nil
}

// FromCheckpoint loads checkpoint files from disk into memory in parallel.
//
// Uses worker pool to load cluster/host combinations. Returns number of files
// loaded and any errors.
func (m *MemoryStore) FromCheckpoint(dir string, from int64) (int, error) {
	var wg sync.WaitGroup
	work := make(chan [2]string, Keys.NumWorkers*4)
	n, errs := int32(0), int32(0)

	wg.Add(Keys.NumWorkers)
	for worker := 0; worker < Keys.NumWorkers; worker++ {
		go func() {
			defer wg.Done()
			for host := range work {
				lvl := m.root.findLevelOrCreate(host[:], len(m.Metrics))
				nn, err := lvl.fromCheckpoint(m, filepath.Join(dir, host[0], host[1]), from)
				if err != nil {
					cclog.Errorf("[METRICSTORE]> error while loading checkpoints for %s/%s: %s", host[0], host[1], err.Error())
					atomic.AddInt32(&errs, 1)
				}
				atomic.AddInt32(&n, int32(nn))
			}
		}()
	}

	err := enqueueCheckpointHosts(dir, work)
	close(work)
	wg.Wait()

	if err != nil {
		return int(n), err
	}

	if errs > 0 {
		return int(n), fmt.Errorf("[METRICSTORE]> %d errors happened while creating checkpoints (%d successes)", errs, n)
	}
	return int(n), nil
}

// FromCheckpointFiles is the main entry point for loading checkpoints at startup.
//
// Creates checkpoint directory if it doesn't exist. This function must be called
// before any writes or reads, and can only be called once.
func (m *MemoryStore) FromCheckpointFiles(dir string, from int64) (int, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, CheckpointDirPerms)
		if err != nil {
			cclog.Fatalf("[METRICSTORE]> Error creating directory: %#v\n", err)
		}
		cclog.Debugf("[METRICSTORE]> %#v Directory created successfully", dir)
	}

	return m.FromCheckpoint(dir, from)
}

func (l *Level) loadJSONFile(m *MemoryStore, f *os.File, from int64) error {
	br := bufio.NewReader(f)
	cf := &CheckpointFile{}
	if err := json.NewDecoder(br).Decode(cf); err != nil {
		return err
	}

	if cf.To != 0 && cf.To < from {
		return nil
	}

	if err := l.loadFile(cf, m); err != nil {
		return err
	}

	return nil
}

func (l *Level) loadFile(cf *CheckpointFile, m *MemoryStore) error {
	for name, metric := range cf.Metrics {
		n := len(metric.Data)
		b := &buffer{
			frequency: metric.Frequency,
			start:     metric.Start,
			data:      metric.Data[0:n:n],
			prev:      nil,
			next:      nil,
			archived:  true,
		}

		minfo, ok := m.Metrics[name]
		if !ok {
			continue
		}

		prev := l.metrics[minfo.offset]
		if prev == nil {
			l.metrics[minfo.offset] = b
		} else {
			if prev.start > b.start {
				return fmt.Errorf("[METRICSTORE]> buffer start time %d is before previous buffer start %d", b.start, prev.start)
			}

			b.prev = prev
			prev.next = b
		}
		l.metrics[minfo.offset] = b
	}

	if len(cf.Children) > 0 && l.children == nil {
		l.children = make(map[string]*Level)
	}

	for sel, childCf := range cf.Children {
		child, ok := l.children[sel]
		if !ok {
			child = &Level{
				metrics:  make([]*buffer, len(m.Metrics)),
				children: nil,
			}
			l.children[sel] = child
		}

		if err := child.loadFile(childCf, m); err != nil {
			return err
		}
	}

	return nil
}

// fromCheckpoint loads all checkpoint files (JSON, binary snapshot, WAL) for a
// single host directory. Snapshot files are loaded first (sorted by timestamp),
// then current.wal is replayed on top.
func (l *Level) fromCheckpoint(m *MemoryStore, dir string, from int64) (int, error) {
	direntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	allFiles := make([]fs.DirEntry, 0)
	var walEntry fs.DirEntry
	filesLoaded := 0

	for _, e := range direntries {
		if e.IsDir() {
			// Legacy: skip subdirectories (only used by old Avro format).
			// These are ignored; their data is not loaded.
			cclog.Debugf("[METRICSTORE]> skipping subdirectory %s in checkpoint dir %s", e.Name(), dir)
			continue
		}

		name := e.Name()
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".bin") {
			allFiles = append(allFiles, e)
		} else if name == "current.wal" {
			walEntry = e
		}
		// Silently ignore other files (e.g., .tmp, .bin.tmp from interrupted writes).
	}

	files, err := findFiles(allFiles, from, true)
	if err != nil {
		return filesLoaded, err
	}

	loaders := map[string]func(*MemoryStore, *os.File, int64) error{
		".json": l.loadJSONFile,
		".bin":  l.loadBinaryFile,
	}

	for _, filename := range files {
		ext := filepath.Ext(filename)
		loader := loaders[ext]
		if loader == nil {
			cclog.Warnf("[METRICSTORE]> unknown extension for checkpoint file %s", filename)
			continue
		}

		err := func() error {
			f, err := os.Open(path.Join(dir, filename))
			if err != nil {
				return err
			}
			defer f.Close()
			return loader(m, f, from)
		}()
		if err != nil {
			return filesLoaded, err
		}
		filesLoaded++
	}

	// Replay WAL after all snapshot files so it fills in data since the last snapshot.
	if walEntry != nil {
		err := func() error {
			f, err := os.Open(path.Join(dir, walEntry.Name()))
			if err != nil {
				return err
			}
			defer f.Close()
			return l.loadWALFile(m, f, from)
		}()
		if err != nil {
			// WAL errors are non-fatal: the snapshot already loaded the bulk of data.
			cclog.Warnf("[METRICSTORE]> WAL replay error for %s: %v (data since last snapshot may be missing)", dir, err)
		} else {
			filesLoaded++
		}
	}

	return filesLoaded, nil
}

// parseTimestampFromFilename extracts a Unix timestamp from a checkpoint filename.
// Supports ".json" (format: "<ts>.json") and ".bin" (format: "<ts>.bin").
func parseTimestampFromFilename(name string) (int64, error) {
	switch {
	case strings.HasSuffix(name, ".json"):
		return strconv.ParseInt(name[:len(name)-5], 10, 64)
	case strings.HasSuffix(name, ".bin"):
		return strconv.ParseInt(name[:len(name)-4], 10, 64)
	default:
		return 0, fmt.Errorf("unknown checkpoint extension for file %q", name)
	}
}

// findFiles returns filenames from direntries whose timestamps satisfy the filter.
// If findMoreRecentFiles is true, returns files with timestamps >= t (plus the
// last file before t if t falls between two files).
func findFiles(direntries []fs.DirEntry, t int64, findMoreRecentFiles bool) ([]string, error) {
	nums := map[string]int64{}
	for _, e := range direntries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".bin") {
			continue
		}

		ts, err := parseTimestampFromFilename(name)
		if err != nil {
			return nil, err
		}
		nums[name] = ts
	}

	sort.Slice(direntries, func(i, j int) bool {
		a, b := direntries[i], direntries[j]
		return nums[a.Name()] < nums[b.Name()]
	})

	if len(nums) == 0 {
		return nil, nil
	}

	filenames := make([]string, 0)

	for i, e := range direntries {
		ts1 := nums[e.Name()]

		if findMoreRecentFiles && t <= ts1 {
			filenames = append(filenames, e.Name())
		} else if !findMoreRecentFiles && ts1 <= t && ts1 != 0 {
			filenames = append(filenames, e.Name())
		}

		if i == len(direntries)-1 {
			continue
		}

		enext := direntries[i+1]
		ts2 := nums[enext.Name()]

		if findMoreRecentFiles {
			if ts1 < t && t < ts2 {
				filenames = append(filenames, e.Name())
			}
		}
	}

	return filenames, nil
}
