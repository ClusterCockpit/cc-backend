// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file implements checkpoint persistence for the in-memory metric store.
//
// Checkpoints enable graceful restarts by periodically saving in-memory metric
// data to disk in either JSON or Avro format. The checkpoint system:
//
// Key Features:
//   - Periodic background checkpointing via the Checkpointing() worker
//   - Two formats: JSON (human-readable) and Avro (compact, efficient)
//   - Parallel checkpoint creation and loading using worker pools
//   - Hierarchical file organization: checkpoint_dir/cluster/host/timestamp.{json|avro}
//   - Only saves unarchived data (archived data is already persisted elsewhere)
//   - Automatic format detection and fallback during loading
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
//	      1234567890.json  (timestamp = checkpoint start time)
//	      1234567950.json
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
	"github.com/linkedin/goavro/v2"
)

const (
	CheckpointFilePerms = 0o644 // File permissions for checkpoint files
	CheckpointDirPerms  = 0o755 // Directory permissions for checkpoint directories
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
// The behavior depends on the configured file format:
//   - JSON: Periodic checkpointing based on Keys.Checkpoints.Interval
//   - Avro: Initial delay + periodic checkpointing at DefaultAvroCheckpointInterval
//
// The worker respects context cancellation and signals completion via the WaitGroup.
func Checkpointing(wg *sync.WaitGroup, ctx context.Context) {
	lastCheckpointMu.Lock()
	lastCheckpoint = time.Now()
	lastCheckpointMu.Unlock()

	if Keys.Checkpoints.FileFormat == "json" {
		ms := GetMemoryStore()

		wg.Add(1)
		go func() {
			defer wg.Done()
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

					cclog.Infof("[METRICSTORE]> start checkpointing (starting at %s)...", from.Format(time.RFC3339))
					now := time.Now()
					n, err := ms.ToCheckpoint(Keys.Checkpoints.RootDir,
						from.Unix(), now.Unix())
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
		}()
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(CheckpointBufferMinutes) * time.Minute):
				GetAvroStore().ToCheckpoint(Keys.Checkpoints.RootDir, false)
			}

			ticker := time.NewTicker(DefaultAvroCheckpointInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					GetAvroStore().ToCheckpoint(Keys.Checkpoints.RootDir, false)
				}
			}
		}()
	}
}

// UnmarshalJSON provides optimized JSON decoding for CheckpointMetrics.
//
// Mirrors the optimized MarshalJSON by manually parsing JSON to avoid
// per-element interface dispatch and allocation overhead of the generic
// json.Unmarshal path for []schema.Float.
func (cm *CheckpointMetrics) UnmarshalJSON(input []byte) error {
	// Minimal manual JSON parsing for the known structure:
	// {"frequency":N,"start":N,"data":[...]}
	// Field order may vary, so we parse field names.

	if len(input) < 2 || input[0] != '{' {
		return fmt.Errorf("expected JSON object")
	}

	i := 1 // skip '{'
	for i < len(input) {
		// Skip whitespace
		for i < len(input) && (input[i] == ' ' || input[i] == '\t' || input[i] == '\n' || input[i] == '\r') {
			i++
		}
		if i >= len(input) || input[i] == '}' {
			break
		}
		if input[i] == ',' {
			i++
			continue
		}

		// Parse field name
		if input[i] != '"' {
			return fmt.Errorf("expected field name at pos %d", i)
		}
		i++
		nameStart := i
		for i < len(input) && input[i] != '"' {
			i++
		}
		fieldName := string(input[nameStart:i])
		i++ // skip closing '"'

		// Skip ':'
		for i < len(input) && (input[i] == ' ' || input[i] == ':') {
			i++
		}

		switch fieldName {
		case "frequency":
			numStart := i
			for i < len(input) && input[i] != ',' && input[i] != '}' {
				i++
			}
			v, err := strconv.ParseInt(string(input[numStart:i]), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid frequency: %w", err)
			}
			cm.Frequency = v

		case "start":
			numStart := i
			for i < len(input) && input[i] != ',' && input[i] != '}' {
				i++
			}
			v, err := strconv.ParseInt(string(input[numStart:i]), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid start: %w", err)
			}
			cm.Start = v

		case "data":
			if input[i] != '[' {
				return fmt.Errorf("expected '[' for data array at pos %d", i)
			}
			i++ // skip '['

			cm.Data = make([]schema.Float, 0, 256)
			for i < len(input) {
				// Skip whitespace
				for i < len(input) && (input[i] == ' ' || input[i] == '\t' || input[i] == '\n' || input[i] == '\r') {
					i++
				}
				if i >= len(input) {
					break
				}
				if input[i] == ']' {
					i++
					break
				}
				if input[i] == ',' {
					i++
					continue
				}

				// Parse value: number or null
				if input[i] == 'n' {
					// "null"
					cm.Data = append(cm.Data, schema.NaN)
					i += 4
				} else {
					numStart := i
					for i < len(input) && input[i] != ',' && input[i] != ']' && input[i] != ' ' {
						i++
					}
					v, err := strconv.ParseFloat(string(input[numStart:i]), 64)
					if err != nil {
						return fmt.Errorf("invalid data value: %w", err)
					}
					cm.Data = append(cm.Data, schema.Float(v))
				}
			}

		default:
			// Skip unknown field value
			depth := 0
			inStr := false
			for i < len(input) {
				if inStr {
					if input[i] == '\\' {
						i++
					} else if input[i] == '"' {
						inStr = false
					}
				} else {
					switch input[i] {
					case '"':
						inStr = true
					case '{', '[':
						depth++
					case '}', ']':
						if depth == 0 {
							goto doneSkip
						}
						depth--
					case ',':
						if depth == 0 {
							goto doneSkip
						}
					}
				}
				i++
			}
		doneSkip:
		}
	}

	return nil
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

// ToCheckpoint writes metric data to checkpoint files in parallel.
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
// Returns the set of cluster names found and any error if directory structure is invalid.
func enqueueCheckpointHosts(dir string, work chan<- [2]string) (map[string]struct{}, error) {
	clustersDir, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	clusters := make(map[string]struct{}, len(clustersDir))

	for _, clusterDir := range clustersDir {
		if !clusterDir.IsDir() {
			return nil, errors.New("[METRICSTORE]> expected only directories at first level of checkpoints/ directory")
		}

		clusters[clusterDir.Name()] = struct{}{}

		hostsDir, err := os.ReadDir(filepath.Join(dir, clusterDir.Name()))
		if err != nil {
			return nil, err
		}

		for _, hostDir := range hostsDir {
			if !hostDir.IsDir() {
				return nil, errors.New("[METRICSTORE]> expected only directories at second level of checkpoints/ directory")
			}

			work <- [2]string{clusterDir.Name(), hostDir.Name()}
		}
	}

	return clusters, nil
}

// FromCheckpoint loads checkpoint files from disk into memory in parallel.
//
// Pre-creates cluster-level entries to reduce lock contention during parallel loading.
// Uses worker pool to load cluster/host combinations. Returns number of files loaded and any errors.
func (m *MemoryStore) FromCheckpoint(dir string, from int64) (int, error) {
	// Pre-create cluster-level entries to eliminate write-lock contention on m.root
	// during parallel loading. Workers only contend at the cluster level (independent).
	clusterDirs, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for _, d := range clusterDirs {
		if d.IsDir() {
			m.root.findLevelOrCreate([]string{d.Name()}, len(m.Metrics))
		}
	}

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

	_, err = enqueueCheckpointHosts(dir, work)
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
// Automatically detects checkpoint format (JSON vs Avro) and falls back if needed.
// Creates checkpoint directory if it doesn't exist. This function must be called
// before any writes or reads, and can only be called once.
func (m *MemoryStore) FromCheckpointFiles(dir string, from int64) (int, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// The directory does not exist, so create it using os.MkdirAll()
		err := os.MkdirAll(dir, CheckpointDirPerms) // CheckpointDirPerms sets the permissions for the directory
		if err != nil {
			cclog.Fatalf("[METRICSTORE]> Error creating directory: %#v\n", err)
		}
		cclog.Debugf("[METRICSTORE]> %#v Directory created successfully", dir)
	}

	return m.FromCheckpoint(dir, from)
}

func (l *Level) loadAvroFile(m *MemoryStore, f *os.File, from int64) error {
	br := bufio.NewReader(f)

	fileName := f.Name()[strings.LastIndex(f.Name(), "/")+1:]
	resolution, err := strconv.ParseInt(fileName[0:strings.Index(fileName, "_")], 10, 64)
	if err != nil {
		return fmt.Errorf("[METRICSTORE]> error while reading avro file (resolution parsing) : %s", err)
	}

	fromTimestamp, err := strconv.ParseInt(fileName[strings.Index(fileName, "_")+1:len(fileName)-5], 10, 64)

	// Same logic according to lineprotocol
	fromTimestamp -= (resolution / 2)

	if err != nil {
		return fmt.Errorf("[METRICSTORE]> error converting timestamp from the avro file : %s", err)
	}

	// fmt.Printf("File : %s with resolution : %d\n", fileName, resolution)

	var recordCounter int64 = 0

	// Create a new OCF reader from the buffered reader
	ocfReader, err := goavro.NewOCFReader(br)
	if err != nil {
		return fmt.Errorf("[METRICSTORE]> error creating OCF reader: %w", err)
	}

	metricsData := make(map[string]schema.FloatArray)

	for ocfReader.Scan() {
		datum, err := ocfReader.Read()
		if err != nil {
			return fmt.Errorf("[METRICSTORE]> error while reading avro file : %s", err)
		}

		record, ok := datum.(map[string]any)
		if !ok {
			return fmt.Errorf("[METRICSTORE]> failed to assert datum as map[string]interface{}")
		}

		for key, value := range record {
			metricsData[key] = append(metricsData[key], schema.ConvertToFloat(value.(float64)))
		}

		recordCounter += 1
	}

	to := (fromTimestamp + (recordCounter / (60 / resolution) * 60))
	if to < from {
		return nil
	}

	for key, floatArray := range metricsData {
		metricName := ReplaceKey(key)

		if strings.Contains(metricName, SelectorDelimiter) {
			subString := strings.Split(metricName, SelectorDelimiter)

			lvl := l

			for i := 0; i < len(subString)-1; i++ {

				sel := subString[i]

				if lvl.children == nil {
					lvl.children = make(map[string]*Level)
				}

				child, ok := lvl.children[sel]
				if !ok {
					child = &Level{
						metrics:  make([]*buffer, len(m.Metrics)),
						children: nil,
					}
					lvl.children[sel] = child
				}
				lvl = child
			}

			leafMetricName := subString[len(subString)-1]
			err = lvl.createBuffer(m, leafMetricName, floatArray, fromTimestamp, resolution)
			if err != nil {
				return fmt.Errorf("[METRICSTORE]> error while creating buffers from avroReader : %s", err)
			}
		} else {
			err = l.createBuffer(m, metricName, floatArray, fromTimestamp, resolution)
			if err != nil {
				return fmt.Errorf("[METRICSTORE]> error while creating buffers from avroReader : %s", err)
			}
		}

	}

	return nil
}

func (l *Level) createBuffer(m *MemoryStore, metricName string, floatArray schema.FloatArray, from int64, resolution int64) error {
	n := len(floatArray)
	b := &buffer{
		frequency: resolution,
		start:     from,
		data:      floatArray[0:n:n],
		prev:      nil,
		next:      nil,
		archived:  true,
	}

	minfo, ok := m.Metrics[metricName]
	if !ok {
		return nil
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

		missingCount := ((int(b.start) - int(prev.start)) - len(prev.data)*int(b.frequency))
		if missingCount > 0 {
			missingCount /= int(b.frequency)

			for range missingCount {
				prev.data = append(prev.data, schema.NaN)
			}

			prev.data = prev.data[0:len(prev.data):len(prev.data)]
		}
	}
	l.metrics[minfo.offset] = b

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

func (l *Level) fromCheckpoint(m *MemoryStore, dir string, from int64) (int, error) {
	direntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, err
	}

	allFiles := make([]fs.DirEntry, 0, len(direntries))
	filesLoaded := 0
	for _, e := range direntries {
		if e.IsDir() {
			// Host-level directories should only contain files, not subdirectories.
			// Skip unexpected subdirectories with a warning.
			cclog.Warnf("[METRICSTORE]> unexpected subdirectory '%s' in checkpoint dir '%s', skipping", e.Name(), dir)
			continue
		} else if strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".avro") {
			allFiles = append(allFiles, e)
		}
	}

	files, err := findFiles(allFiles, from, true)
	if err != nil {
		return filesLoaded, err
	}

	if len(files) == 0 {
		return 0, nil
	}

	// Separate files by type
	var jsonFiles, avroFiles []string
	for _, filename := range files {
		switch filepath.Ext(filename) {
		case ".json":
			jsonFiles = append(jsonFiles, filename)
		case ".avro":
			avroFiles = append(avroFiles, filename)
		default:
			cclog.Warnf("[METRICSTORE]> unknown extension for file %s", filename)
		}
	}

	// Parallel JSON decoding: decode files concurrently, then apply sequentially
	if len(jsonFiles) > 0 {
		type decodedFile struct {
			cf  *CheckpointFile
			err error
		}

		decoded := make([]decodedFile, len(jsonFiles))
		var decodeWg sync.WaitGroup

		for i, filename := range jsonFiles {
			decodeWg.Add(1)
			go func(idx int, fname string) {
				defer decodeWg.Done()
				f, err := os.Open(path.Join(dir, fname))
				if err != nil {
					decoded[idx] = decodedFile{err: err}
					return
				}
				defer f.Close()

				cf := &CheckpointFile{}
				if err := json.NewDecoder(bufio.NewReader(f)).Decode(cf); err != nil {
					decoded[idx] = decodedFile{err: fmt.Errorf("decoding %s: %w", fname, err)}
					return
				}

				decoded[idx] = decodedFile{cf: cf}
			}(i, filename)
		}

		decodeWg.Wait()

		// Apply decoded files sequentially to maintain buffer ordering
		for i, d := range decoded {
			if d.err != nil {
				return filesLoaded, d.err
			}

			if d.cf.To != 0 && d.cf.To < from {
				continue
			}

			if err := l.loadFile(d.cf, m); err != nil {
				return filesLoaded, fmt.Errorf("loading %s: %w", jsonFiles[i], err)
			}
			filesLoaded++
		}
	}

	// Load Avro files sequentially (they modify Level state directly)
	for _, filename := range avroFiles {
		err := func() error {
			f, err := os.Open(path.Join(dir, filename))
			if err != nil {
				return err
			}
			defer f.Close()

			return l.loadAvroFile(m, f, from)
		}()
		if err != nil {
			return filesLoaded, err
		}
		filesLoaded++
	}

	return filesLoaded, nil
}

// findFiles filters and sorts checkpoint files by timestamp.
//
// When findMoreRecentFiles is true, returns files with timestamp >= t (for loading),
// plus the immediately preceding file if it straddles the boundary.
// When false, returns files with timestamp <= t (for cleanup).
//
// Filters before sorting so only relevant files are sorted, keeping performance
// stable regardless of total directory size.
func findFiles(direntries []fs.DirEntry, t int64, findMoreRecentFiles bool) ([]string, error) {
	type fileEntry struct {
		name string
		ts   int64
	}

	// Parse timestamps and pre-filter in a single pass
	var candidates []fileEntry
	var bestPreceding *fileEntry // Track the file just before the cutoff (for boundary straddling)

	for _, e := range direntries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".avro") {
			continue
		}

		ts, err := strconv.ParseInt(name[strings.Index(name, "_")+1:len(name)-5], 10, 64)
		if err != nil {
			return nil, err
		}

		if findMoreRecentFiles {
			if ts >= t {
				candidates = append(candidates, fileEntry{name, ts})
			} else {
				// Track the most recent file before the cutoff for boundary straddling
				if bestPreceding == nil || ts > bestPreceding.ts {
					bestPreceding = &fileEntry{name, ts}
				}
			}
		} else {
			if ts <= t && ts != 0 {
				candidates = append(candidates, fileEntry{name, ts})
			}
		}
	}

	// Include the boundary-straddling file if we found one and there are also files after the cutoff
	if findMoreRecentFiles && bestPreceding != nil && len(candidates) > 0 {
		candidates = append(candidates, *bestPreceding)
	}

	if len(candidates) == 0 {
		// If searching for recent files and we only have a preceding file, include it
		if findMoreRecentFiles && bestPreceding != nil {
			return []string{bestPreceding.name}, nil
		}
		return nil, nil
	}

	// Sort only the filtered candidates
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ts < candidates[j].ts
	})

	filenames := make([]string, len(candidates))
	for i, c := range candidates {
		filenames[i] = c.name
	}

	return filenames, nil
}
