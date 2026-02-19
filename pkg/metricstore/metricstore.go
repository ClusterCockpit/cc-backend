// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstore provides an efficient in-memory time-series metric storage system
// with support for hierarchical data organization, checkpointing, and archiving.
//
// The package organizes metrics in a tree structure (cluster → host → component) and
// provides concurrent read/write access to metric data with configurable aggregation strategies.
// Background goroutines handle periodic checkpointing (JSON or Avro format), archiving old data,
// and enforcing retention policies.
//
// Key features:
//   - In-memory metric storage with configurable retention
//   - Hierarchical data organization (selectors)
//   - Concurrent checkpoint/archive workers
//   - Support for sum and average aggregation
//   - NATS integration for metric ingestion
package metricstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/resampler"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

// GlobalState holds the global state for the metric store with thread-safe access.
type GlobalState struct {
	mu                sync.RWMutex
	lastRetentionTime int64
	selectorsExcluded bool
}

var (
	singleton  sync.Once
	msInstance *MemoryStore
	// shutdownFunc stores the context cancellation function created in Init
	// and is called during Shutdown to cancel all background goroutines
	shutdownFunc   context.CancelFunc
	shutdownFuncMu sync.Mutex // Protects shutdownFunc from concurrent access
	// Create a global instance
	state = &GlobalState{}
)

// NodeProvider provides information about nodes currently in use by running jobs.
//
// This interface allows metricstore to query job information without directly
// depending on the repository package, breaking the import cycle.
//
// Implementations should return nodes that are actively processing jobs started
// before the given timestamp. These nodes will be excluded from retention-based
// garbage collection to prevent data loss for jobs that are still running or
// recently completed.
type NodeProvider interface {
	// GetUsedNodes returns a map of cluster names to sorted lists of unique hostnames
	// that are currently in use by jobs that started before the given timestamp.
	//
	// Parameters:
	//   - ts: Unix timestamp threshold - returns nodes with jobs started before this time
	//
	// Returns:
	//   - Map of cluster names to lists of node hostnames that should be excluded from garbage collection
	//   - Error if the query fails
	GetUsedNodes(ts int64) (map[string][]string, error)
}

// Metric represents a single metric data point to be written to the store.
type Metric struct {
	Name  string
	Value schema.Float
	// MetricConfig contains frequency and aggregation settings for this metric.
	// If Frequency is 0, configuration will be looked up from MemoryStore.Metrics during Write().
	MetricConfig MetricConfig
}

// MemoryStore is the main in-memory time-series metric storage implementation.
//
// It organizes metrics in a hierarchical tree structure where each level represents
// a component of the system hierarchy (e.g., cluster → host → CPU). Each level can
// store multiple metrics as time-series buffers.
//
// The store is initialized as a singleton via InitMetrics() and accessed via GetMemoryStore().
// All public methods are safe for concurrent use.
type MemoryStore struct {
	Metrics      map[string]MetricConfig
	root         Level
	nodeProvider NodeProvider
}

// Init initializes the metric store from configuration and starts background workers.
//
// This function must be called exactly once before any other metricstore operations.
// It performs the following initialization steps:
//  1. Validates and decodes the metric store configuration
//  2. Configures worker pool size (defaults to NumCPU/2+1, max 10)
//  3. Loads metric configurations from all registered clusters
//  4. Restores checkpoints within the retention window
//  5. Starts background workers for retention, checkpointing, archiving, and monitoring
//  6. Optionally subscribes to NATS for real-time metric ingestion
//
// Parameters:
//   - rawConfig: JSON configuration for the metric store (see MetricStoreConfig)
//   - wg: WaitGroup that will be incremented for each background goroutine started
//
// The function will call cclog.Fatal on critical errors during initialization.
// Use Shutdown() to cleanly stop all background workers started by Init().
//
// Note: Signal handling must be implemented by the caller. Call Shutdown() when
// receiving termination signals to ensure checkpoint data is persisted.
func Init(rawConfig json.RawMessage, metrics map[string]MetricConfig, wg *sync.WaitGroup) {
	startupTime := time.Now()

	if rawConfig != nil {
		config.Validate(configSchema, rawConfig)
		dec := json.NewDecoder(bytes.NewReader(rawConfig))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&Keys); err != nil {
			cclog.Abortf("[METRICSTORE]> Metric Store Config Init: Could not decode config file '%s'.\nError: %s\n", rawConfig, err.Error())
		}
	}

	// Set NumWorkers from config or use default
	if Keys.NumWorkers <= 0 {
		Keys.NumWorkers = min(runtime.NumCPU()/2+1, DefaultMaxWorkers)
	}
	cclog.Debugf("[METRICSTORE]> Using %d workers for checkpoint/archive operations\n", Keys.NumWorkers)

	// Pass the config.MetricStoreKeys
	InitMetrics(metrics)

	ms := GetMemoryStore()

	d, err := time.ParseDuration(Keys.RetentionInMemory)
	if err != nil {
		cclog.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	cclog.Infof("[METRICSTORE]> Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))

	// Lower GC target during loading to prevent excessive heap growth.
	// During checkpoint loading the heap grows rapidly, causing the GC to
	// double its target repeatedly. A lower percentage keeps it tighter.
	oldGCPercent := debug.SetGCPercent(20)

	files, err := ms.FromCheckpointFiles(Keys.Checkpoints.RootDir, restoreFrom.Unix())
	loadedData := ms.SizeInBytes() / 1024 / 1024 // In MB
	if err != nil {
		cclog.Fatalf("[METRICSTORE]> Loading checkpoints failed: %s\n", err.Error())
	} else {
		cclog.Infof("[METRICSTORE]> Checkpoints loaded (%d files, %d MB, that took %fs)\n", files, loadedData, time.Since(startupTime).Seconds())
	}

	// Restore GC target and force a collection to set a tight baseline
	// for the "previously active heap" size, reducing long-term memory waste.
	debug.SetGCPercent(oldGCPercent)
	runtime.GC()

	ctx, shutdown := context.WithCancel(context.Background())

	Retention(wg, ctx)
	Checkpointing(wg, ctx)
	CleanUp(wg, ctx)
	DataStaging(wg, ctx)
	MemoryUsageTracker(wg, ctx)

	// Note: Signal handling has been removed from this function.
	// The caller is responsible for handling shutdown signals and calling
	// the shutdown() function when appropriate.
	// Store the shutdown function for later use by Shutdown()
	shutdownFuncMu.Lock()
	shutdownFunc = shutdown
	shutdownFuncMu.Unlock()

	if Keys.Subscriptions != nil {
		err = ReceiveNats(ms, 1, ctx)
		if err != nil {
			cclog.Fatal(err)
		}
	}
}

// InitMetrics initializes the singleton MemoryStore instance with the given metric configurations.
//
// This function must be called before GetMemoryStore() and can only be called once due to
// the singleton pattern. It assigns each metric an internal offset for efficient buffer indexing.
//
// Parameters:
//   - metrics: Map of metric names to their configurations (frequency and aggregation strategy)
//
// Panics if any metric has Frequency == 0, which indicates an invalid configuration.
//
// After this call, the global msInstance is ready for use via GetMemoryStore().
func InitMetrics(metrics map[string]MetricConfig) {
	singleton.Do(func() {
		offset := 0
		for key, cfg := range metrics {
			if cfg.Frequency == 0 {
				panic("[METRICSTORE]> invalid frequency")
			}

			metrics[key] = MetricConfig{
				Frequency:   cfg.Frequency,
				Aggregation: cfg.Aggregation,
				offset:      offset,
			}
			offset += 1
		}

		msInstance = &MemoryStore{
			root: Level{
				metrics:  make([]*buffer, len(metrics)),
				children: make(map[string]*Level),
			},
			Metrics: metrics,
		}
	})
}

// GetMemoryStore returns the singleton MemoryStore instance.
//
// Returns the initialized MemoryStore singleton. Calls cclog.Fatal if InitMetrics() was not called first.
//
// This function is safe for concurrent use after initialization.
func GetMemoryStore() *MemoryStore {
	if msInstance == nil {
		cclog.Fatalf("[METRICSTORE]> MemoryStore not initialized!")
	}

	return msInstance
}

func (ms *MemoryStore) GetMetricFrequency(metricName string) (int64, error) {
	if metric, ok := ms.Metrics[metricName]; ok {
		return metric.Frequency, nil
	}
	return 0, fmt.Errorf("[METRICSTORE]> metric %s not found", metricName)
}

// SetNodeProvider sets the NodeProvider implementation for the MemoryStore.
// This must be called during initialization to provide job state information
// for selective buffer retention during Free operations.
// If not set, the Free function will fall back to freeing all buffers.
func (ms *MemoryStore) SetNodeProvider(provider NodeProvider) {
	ms.nodeProvider = provider
}

// Shutdown performs a graceful shutdown of the metric store.
//
// This function cancels all background goroutines started by Init() and writes
// a final checkpoint to disk before returning. It should be called when the
// application receives a termination signal.
//
// The function will:
//  1. Cancel the context to stop all background workers
//  2. Close NATS message channels if using Avro format
//  3. Write a final checkpoint to preserve in-memory data
//  4. Log any errors encountered during shutdown
//
// Note: This function blocks until the final checkpoint is written.
func Shutdown() {
	shutdownFuncMu.Lock()
	defer shutdownFuncMu.Unlock()
	if shutdownFunc != nil {
		shutdownFunc()
	}

	if Keys.Checkpoints.FileFormat != "json" {
		close(LineProtocolMessages)
	}

	cclog.Infof("[METRICSTORE]> Writing to '%s'...\n", Keys.Checkpoints.RootDir)
	var files int
	var err error

	ms := GetMemoryStore()

	if Keys.Checkpoints.FileFormat == "json" {
		files, err = ms.ToCheckpoint(Keys.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
	} else {
		files, err = GetAvroStore().ToCheckpoint(Keys.Checkpoints.RootDir, true)
	}

	if err != nil {
		cclog.Errorf("[METRICSTORE]> Writing checkpoint failed: %s\n", err.Error())
	}
	cclog.Infof("[METRICSTORE]> Done! (%d files written)\n", files)
}

// Retention starts a background goroutine that periodically frees old metric data.
//
// This worker runs at half the retention interval and calls Free() to remove buffers
// older than the configured retention time. It respects the NodeProvider to preserve
// data for nodes with active jobs.
//
// Parameters:
//   - wg: WaitGroup to signal completion when context is cancelled
//   - ctx: Context for cancellation signal
//
// The goroutine exits when ctx is cancelled.
func Retention(wg *sync.WaitGroup, ctx context.Context) {
	ms := GetMemoryStore()

	wg.Add(1)
	go func() {
		defer wg.Done()
		d, err := time.ParseDuration(Keys.RetentionInMemory)
		if err != nil {
			cclog.Fatal(err)
		}
		if d <= 0 {
			return
		}

		tickInterval := d / 2
		if tickInterval <= 0 {
			return
		}
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state.mu.Lock()

				t := time.Now().Add(-d)

				state.lastRetentionTime = t.Unix()

				cclog.Infof("[METRICSTORE]> start freeing buffers (older than %s)...\n", t.Format(time.RFC3339))

				freed, err := Free(ms, t)
				if err != nil {
					cclog.Errorf("[METRICSTORE]> freeing up buffers failed: %s\n", err.Error())
				} else {
					cclog.Infof("[METRICSTORE]> done: %d buffers freed\n", freed)
				}

				state.mu.Unlock()
			}
		}
	}()
}

// MemoryUsageTracker starts a background goroutine that monitors memory usage.
//
// This worker checks actual process memory usage (via runtime.MemStats) periodically
// and force-frees buffers if memory exceeds the configured cap. It uses FreeOSMemory()
// to return memory to the OS after freeing buffers, avoiding aggressive GC that causes
// performance issues.
//
// The tracker logs both actual memory usage (heap allocated) and metric data size for
// visibility into memory overhead from Go runtime structures and allocations.
//
// Parameters:
//   - wg: WaitGroup to signal completion when context is cancelled
//   - ctx: Context for cancellation signal
//
// The goroutine exits when ctx is cancelled.
func MemoryUsageTracker(wg *sync.WaitGroup, ctx context.Context) {
	ms := GetMemoryStore()

	wg.Add(1)
	go func() {
		defer wg.Done()
		d := DefaultMemoryUsageTrackerInterval

		if d <= 0 {
			return
		}

		ticker := time.NewTicker(d)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)
				actualMemoryGB := float64(mem.Alloc) / 1e9
				metricDataGB := ms.SizeInGB()
				cclog.Infof("[METRICSTORE]> memory usage: %.2f GB actual (%.2f GB metric data)", actualMemoryGB, metricDataGB)

				freedExcluded := 0
				freedEmergency := 0
				var err error

				state.mu.RLock()
				lastRetention := state.lastRetentionTime
				selectorsExcluded := state.selectorsExcluded
				state.mu.RUnlock()

				if lastRetention != 0 && selectorsExcluded {
					freedExcluded, err = ms.Free(nil, lastRetention)
					if err != nil {
						cclog.Errorf("[METRICSTORE]> error while force-freeing the excluded buffers: %s", err)
					}

					if freedExcluded > 0 {
						debug.FreeOSMemory()
						cclog.Infof("[METRICSTORE]> done: %d excluded buffers force-freed", freedExcluded)
					}
				}

				runtime.ReadMemStats(&mem)
				actualMemoryGB = float64(mem.Alloc) / 1e9

				if actualMemoryGB > float64(Keys.MemoryCap) {
					cclog.Warnf("[METRICSTORE]> memory usage %.2f GB exceeds cap %d GB, starting emergency buffer freeing", actualMemoryGB, Keys.MemoryCap)

					const maxIterations = 100

					for i := range maxIterations {
						if actualMemoryGB < float64(Keys.MemoryCap) {
							break
						}

						freed, err := ms.ForceFree()
						if err != nil {
							cclog.Errorf("[METRICSTORE]> error while force-freeing buffers: %s", err)
						}
						if freed == 0 {
							cclog.Errorf("[METRICSTORE]> no more buffers to free after %d emergency frees, memory usage %.2f GB still exceeds cap %d GB", freedEmergency, actualMemoryGB, Keys.MemoryCap)
							break
						}
						freedEmergency += freed

						if i%10 == 0 && freedEmergency > 0 {
							runtime.ReadMemStats(&mem)
							actualMemoryGB = float64(mem.Alloc) / 1e9
						}
					}

					// if freedEmergency > 0 {
					// 	debug.FreeOSMemory()
					// }

					runtime.ReadMemStats(&mem)
					actualMemoryGB = float64(mem.Alloc) / 1e9

					if actualMemoryGB >= float64(Keys.MemoryCap) {
						cclog.Errorf("[METRICSTORE]> after %d emergency frees, memory usage %.2f GB still at/above cap %d GB", freedEmergency, actualMemoryGB, Keys.MemoryCap)
					} else {
						cclog.Infof("[METRICSTORE]> emergency freeing complete: %d buffers freed, memory now %.2f GB", freedEmergency, actualMemoryGB)
					}
				}
			}
		}
	}()
}

// Free removes metric data older than the given time while preserving data for active nodes.
//
// This function implements intelligent retention by consulting the NodeProvider (if configured)
// to determine which nodes are currently in use by running jobs. Data for these nodes is
// preserved even if older than the retention time.
//
// Parameters:
//   - ms: The MemoryStore instance
//   - t: Time threshold - buffers with data older than this will be freed
//
// Returns:
//   - Number of buffers freed
//   - Error if NodeProvider query fails
//
// Behavior:
//   - If no NodeProvider is set: frees all buffers older than t
//   - If NodeProvider returns empty map: frees all buffers older than t
//   - Otherwise: preserves buffers for nodes returned by GetUsedNodes(), frees others
func Free(ms *MemoryStore, t time.Time) (int, error) {
	// If no NodeProvider is configured, free all buffers older than t
	if ms.nodeProvider == nil {
		return ms.Free(nil, t.Unix())
	}

	excludeSelectors, err := ms.nodeProvider.GetUsedNodes(t.Unix())
	if err != nil {
		return 0, err
	}

	switch lenMap := len(excludeSelectors); lenMap {

	// If the length of the map returned by GetUsedNodes() is 0,
	// then use default Free method with nil selector
	case 0:
		state.selectorsExcluded = false
		return ms.Free(nil, t.Unix())

	// Else formulate selectors, exclude those from the map
	// and free the rest of the selectors
	default:
		state.selectorsExcluded = true
		selectors := GetSelectors(ms, excludeSelectors)
		return FreeSelected(ms, selectors, t)
	}
}

// FreeSelected frees buffers for specific selectors while preserving others.
//
// This function is used when we want to retain some specific nodes beyond the retention time.
// It iterates through the provided selectors and frees their associated buffers.
//
// Parameters:
//   - ms: The MemoryStore instance
//   - selectors: List of selector paths to free (e.g., [["cluster1", "node1"], ["cluster2", "node2"]])
//   - t: Time threshold for freeing buffers
//
// Returns the total number of buffers freed and any error encountered.
func FreeSelected(ms *MemoryStore, selectors [][]string, t time.Time) (int, error) {
	freed := 0

	for _, selector := range selectors {

		freedBuffers, err := ms.Free(selector, t.Unix())
		if err != nil {
			cclog.Errorf("error while freeing selected buffers: %#v", err)
		}
		freed += freedBuffers

	}

	return freed, nil
}

// GetSelectors returns all selectors at depth 2 (cluster/node level) that are NOT in the exclusion map.
//
// This function generates a list of selectors whose buffers should be freed by excluding
// selectors that correspond to nodes currently in use by running jobs.
//
// Parameters:
//   - ms: The MemoryStore instance
//   - excludeSelectors: Map of cluster names to node hostnames that should NOT be freed
//
// Returns a list of selectors ([]string paths) that can be safely freed.
//
// Example:
//
//	If the tree has paths ["emmy", "node001"] and ["emmy", "node002"],
//	and excludeSelectors contains {"emmy": ["node001"]},
//	then only [["emmy", "node002"]] is returned.
func GetSelectors(ms *MemoryStore, excludeSelectors map[string][]string) [][]string {
	allSelectors := ms.GetPaths(2)

	filteredSelectors := make([][]string, 0, len(allSelectors))

	for _, path := range allSelectors {
		if len(path) < 2 {
			continue
		}

		key := path[0]   // The "Key" (Level 1)
		value := path[1] // The "Value" (Level 2)

		exclude := false

		// Check if the key exists in our exclusion map
		if excludedValues, exists := excludeSelectors[key]; exists {
			// The key exists, now check if the specific value is in the exclusion list
			if slices.Contains(excludedValues, value) {
				exclude = true
			}
		}

		if !exclude {
			filteredSelectors = append(filteredSelectors, path)
		}
	}

	return filteredSelectors
}

// GetPaths returns a list of lists (paths) to the specified depth.
func (ms *MemoryStore) GetPaths(targetDepth int) [][]string {
	var results [][]string

	// Start recursion. Initial path is empty.
	// We treat Root as depth 0.
	ms.root.collectPaths(0, targetDepth, []string{}, &results)

	return results
}

// Write all values in `metrics` to the level specified by `selector` for time `ts`.
// Look at `findLevelOrCreate` for how selectors work.
func (m *MemoryStore) Write(selector []string, ts int64, metrics []Metric) error {
	var ok bool
	for i, metric := range metrics {
		if metric.MetricConfig.Frequency == 0 {
			metric.MetricConfig, ok = m.Metrics[metric.Name]
			if !ok {
				cclog.Debugf("[METRICSTORE]> Unknown metric '%s' in Write() - skipping", metric.Name)
				metric.MetricConfig.Frequency = 0
			}
			metrics[i] = metric
		}
	}

	return m.WriteToLevel(&m.root, selector, ts, metrics)
}

func (m *MemoryStore) GetLevel(selector []string) *Level {
	return m.root.findLevelOrCreate(selector, len(m.Metrics))
}

// WriteToLevel assumes that `minfo` in `metrics` is filled in
func (m *MemoryStore) WriteToLevel(l *Level, selector []string, ts int64, metrics []Metric) error {
	l = l.findLevelOrCreate(selector, len(m.Metrics))
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, metric := range metrics {
		if metric.MetricConfig.Frequency == 0 {
			continue
		}

		b := l.metrics[metric.MetricConfig.offset]
		if b == nil {
			// First write to this metric and level
			b = newBuffer(ts, metric.MetricConfig.Frequency)
			l.metrics[metric.MetricConfig.offset] = b
		}

		nb, err := b.write(ts, metric.Value)
		if err != nil {
			return err
		}

		// Last write created a new buffer...
		if b != nb {
			l.metrics[metric.MetricConfig.offset] = nb
		}
	}
	return nil
}

// Read returns all values for metric `metric` from `from` to `to` for the selected level(s).
// If the level does not hold the metric itself, the data will be aggregated recursively from the children.
// The second and third return value are the actual from/to for the data. Those can be different from
// the range asked for if no data was available.
func (m *MemoryStore) Read(selector util.Selector, metric string, from, to, resolution int64) ([]schema.Float, int64, int64, int64, error) {
	if from > to {
		return nil, 0, 0, 0, errors.New("[METRICSTORE]> invalid time range")
	}

	minfo, ok := m.Metrics[metric]
	if !ok {
		return nil, 0, 0, 0, errors.New("[METRICSTORE]> unknown metric: " + metric)
	}

	n, data := 0, make([]schema.Float, (to-from)/minfo.Frequency+1)

	err := m.root.findBuffers(selector, minfo.offset, func(b *buffer) error {
		cdata, cfrom, cto, err := b.read(from, to, data)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto || len(data) != len(cdata) {
			missingfront, missingback := int((from-cfrom)/minfo.Frequency), int((to-cto)/minfo.Frequency)
			if missingfront != 0 {
				return ErrDataDoesNotAlign
			}

			newlen := len(cdata) - missingback
			if newlen < 1 {
				return ErrDataDoesNotAlign
			}
			cdata = cdata[0:newlen]
			if len(cdata) != len(data) {
				return ErrDataDoesNotAlign
			}

			from, to = cfrom, cto
		}

		data = cdata
		n += 1
		return nil
	})

	if err != nil {
		return nil, 0, 0, 0, err
	} else if n == 0 {
		return nil, 0, 0, 0, ErrNoHostOrMetric
	} else if n > 1 {
		if minfo.Aggregation == AvgAggregation {
			normalize := 1. / schema.Float(n)
			for i := 0; i < len(data); i++ {
				data[i] *= normalize
			}
		} else if minfo.Aggregation != SumAggregation {
			return nil, 0, 0, 0, errors.New("[METRICSTORE]> invalid aggregation")
		}
	}

	data, resolution, err = resampler.LargestTriangleThreeBucket(data, minfo.Frequency, resolution)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return data, from, to, resolution, nil
}

// Free releases all buffers for the selected level and all its children that
// contain only values older than `t`.
func (m *MemoryStore) Free(selector []string, t int64) (int, error) {
	return m.GetLevel(selector).free(t)
}

// ForceFree unconditionally removes the oldest buffer from each metric chain.
func (m *MemoryStore) ForceFree() (int, error) {
	return m.GetLevel(nil).forceFree()
}

func (m *MemoryStore) FreeAll() error {
	for k := range m.root.children {
		delete(m.root.children, k)
	}

	return nil
}

func (m *MemoryStore) SizeInBytes() int64 {
	return m.root.sizeInBytes()
}

func (m *MemoryStore) SizeInGB() float64 {
	return float64(m.root.sizeInBytes()) / 1e9
}

// ListChildren , given a selector, returns a list of all children of the level
// selected.
func (m *MemoryStore) ListChildren(selector []string) []string {
	lvl := &m.root
	for lvl != nil && len(selector) != 0 {
		lvl.lock.RLock()
		next := lvl.children[selector[0]]
		lvl.lock.RUnlock()
		lvl = next
		selector = selector[1:]
	}

	if lvl == nil {
		return nil
	}

	lvl.lock.RLock()
	defer lvl.lock.RUnlock()

	children := make([]string, 0, len(lvl.children))
	for child := range lvl.children {
		children = append(children, child)
	}

	return children
}
