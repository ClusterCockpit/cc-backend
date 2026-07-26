// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstore provides an efficient in-memory time-series metric storage system
// with support for hierarchical data organization, checkpointing, and archiving.
//
// The package organizes metrics in a tree structure (cluster → host → component) and
// provides concurrent read/write access to metric data with configurable aggregation strategies.
// Background goroutines handle periodic checkpointing (JSON or WAL/binary format), archiving old data,
// and enforcing retention policies.
//
// Key features:
//   - In-memory metric storage with configurable retention
//   - Hierarchical data organization (selectors)
//   - Concurrent checkpoint/archive workers
//   - Support for sum and average aggregation
//   - NATS integration for metric ingestion via InfluxDB line protocol
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
	"strings"
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
//   - rawConfig: JSON configuration for the metric store (see MetricStoreConfig); may be nil to use defaults
//   - metrics: Map of metric names to their configurations (frequency and aggregation strategy)
//   - provider: NodeProvider consulted during the checkpoint restore (and later by
//     Free/CleanupCheckpoints) to preserve data for nodes with running jobs; may be
//     nil, in which case all provider-aware paths fall back to their plain behavior
//   - wg: WaitGroup that will be incremented for each background goroutine started
//
// The function will call cclog.Fatal on critical errors during initialization.
// Use Shutdown() to cleanly stop all background workers started by Init().
//
// Note: Signal handling must be implemented by the caller. Call Shutdown() when
// receiving termination signals to ensure checkpoint data is persisted.
func Init(rawConfig json.RawMessage, metrics map[string]MetricConfig, provider NodeProvider, wg *sync.WaitGroup) {
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
	if provider != nil {
		ms.SetNodeProvider(provider)
	}

	d, err := time.ParseDuration(Keys.RetentionInMemory)
	if err != nil {
		cclog.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	cclog.Infof("[METRICSTORE]> Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	files, err := ms.FromCheckpointFiles(Keys.Checkpoints.RootDir, restoreFrom.Unix())
	loadedData := ms.SizeInBytes() / 1024 / 1024 // In MB
	if err != nil {
		cclog.Fatalf("[METRICSTORE]> Loading checkpoints failed: %s\n", err.Error())
	} else {
		cclog.Infof("[METRICSTORE]> Checkpoints loaded (%d files, %d MB, that took %fs)\n", files, loadedData, time.Since(startupTime).Seconds())
	}

	// Try to use less memory by forcing a GC run here and then
	// lowering the target percentage. The default of 100 means
	// that only once the ratio of new allocations execeds the
	// previously active heap, a GC is triggered.
	// Forcing a GC here will set the "previously active heap"
	// to a minumum.
	// runtime.GC()

	ctx, shutdown := context.WithCancel(context.Background())

	Retention(wg, ctx)
	Checkpointing(wg, ctx)
	CleanUp(wg, ctx)
	WALStaging(wg, ctx)
	MemoryUsageTracker(wg, ctx)

	// Note: Signal handling has been removed from this function.
	// The caller is responsible for handling shutdown signals and calling
	// the shutdown() function when appropriate.
	// Store the shutdown function for later use by Shutdown()
	shutdownFuncMu.Lock()
	shutdownFunc = shutdown
	shutdownFuncMu.Unlock()

	if Keys.Subscriptions != nil {
		wg.Go(func() {
			if err := ReceiveNats(ms, Keys.NumWorkers, ctx); err != nil {
				cclog.Fatal(err)
			}
		})
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
		cclog.Warnf("[METRICSTORE]> MemoryStore not initialized!")
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
// The provider supplies the set of nodes in use by running jobs, which is
// consulted by Free (selective buffer retention), FromCheckpoint (full-history
// loading for used hosts), and CleanupCheckpoints (skipping used hosts).
// Server startup passes the provider to Init() directly; this setter serves
// callers that do not run Init (tests, the -cleanup-checkpoints CLI path).
// If not set, all provider-aware paths fall back to their plain behavior.
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
//  2. Close the WAL messages channel if using WAL format
//  3. Write a final checkpoint to preserve in-memory data
//  4. Log any errors encountered during shutdown
//
// Note: This function blocks until the final checkpoint is written.
func Shutdown() {
	totalStart := time.Now()

	shutdownFuncMu.Lock()
	if shutdownFunc == nil {
		// Already shut down (or never initialized): nothing to do. This keeps
		// Shutdown idempotent so it is safe to call from more than one path.
		shutdownFuncMu.Unlock()
		return
	}
	shutdownFunc()
	shutdownFunc = nil
	shutdownFuncMu.Unlock()
	cclog.Infof("[METRICSTORE]> Background workers cancelled (%v)", time.Since(totalStart))

	if Keys.Checkpoints.FileFormat == "wal" {
		// Signal producers to stop sending before closing channels,
		// preventing send-on-closed-channel panics from in-flight NATS workers.
		walShuttingDown.Store(true)
		// Brief grace period for in-flight DecodeLine calls to complete.
		time.Sleep(100 * time.Millisecond)

		for _, ch := range walShardChs {
			close(ch)
		}
		drainStart := time.Now()
		WaitForWALStagingDrain()
		cclog.Infof("[METRICSTORE]> WAL staging goroutines exited (%v)", time.Since(drainStart))
	}

	cclog.Infof("[METRICSTORE]> Writing checkpoint to '%s'...", Keys.Checkpoints.RootDir)
	checkpointStart := time.Now()
	var files int
	var err error

	ms := GetMemoryStore()

	lastCheckpointMu.Lock()
	from := lastCheckpoint
	lastCheckpointMu.Unlock()

	if Keys.Checkpoints.FileFormat == "wal" {
		var successDirs []string
		files, successDirs, err = ms.ToCheckpointWAL(Keys.Checkpoints.RootDir, from.Unix(), time.Now().Unix())
		// The final binary snapshot now captures all in-memory data for these
		// hosts, making their current.wal redundant. The staging goroutines have
		// already exited, so remove the WAL files directly (the channel-based
		// RotateWALFiles is no longer safe to call). Without this, current.wal
		// files survive shutdown and keep growing across restarts.
		RotateWALFilesAfterShutdown(successDirs)
	} else {
		files, err = ms.ToCheckpoint(Keys.Checkpoints.RootDir, from.Unix(), time.Now().Unix())
	}

	if err != nil {
		cclog.Errorf("[METRICSTORE]> Writing checkpoint failed: %s", err.Error())
	}
	cclog.Infof("[METRICSTORE]> Done! (%d files written in %v, total shutdown: %v)", files, time.Since(checkpointStart), time.Since(totalStart))
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

	wg.Go(func() {
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

				// Clean up the buffer pool
				bufferPool.Clean(state.lastRetentionTime)
			}
		}
	})
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

	wg.Go(func() {
		normalInterval := DefaultMemoryUsageTrackerInterval
		fastInterval := 30 * time.Second

		if normalInterval <= 0 {
			return
		}

		ticker := time.NewTicker(normalInterval)
		defer ticker.Stop()
		currentInterval := normalInterval

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

				freedEmergency := 0

				if actualMemoryGB > float64(Keys.MemoryCap) {
					cclog.Warnf("[METRICSTORE]> memory usage %.2f GB exceeds cap %d GB, starting emergency buffer freeing", actualMemoryGB, Keys.MemoryCap)

					// Use progressive time-based Free with increasing threshold
					// instead of ForceFree loop — fewer tree traversals, more effective
					d, parseErr := time.ParseDuration(Keys.RetentionInMemory)
					if parseErr != nil {
						cclog.Errorf("[METRICSTORE]> cannot parse retention duration: %s", parseErr)
					} else {
						thresholds := []float64{0.75, 0.5, 0.25}
						for _, fraction := range thresholds {
							threshold := time.Now().Add(-time.Duration(float64(d) * fraction))
							freed, freeErr := ms.Free(nil, threshold.Unix())
							if freeErr != nil {
								cclog.Errorf("[METRICSTORE]> error while freeing buffers at %.0f%% retention: %s", fraction*100, freeErr)
							}
							freedEmergency += freed

							bufferPool.Clear()
							runtime.GC()
							runtime.ReadMemStats(&mem)
							actualMemoryGB = float64(mem.Alloc) / 1e9

							if actualMemoryGB < float64(Keys.MemoryCap) {
								break
							}
						}
					}

					bufferPool.Clear()
					debug.FreeOSMemory()

					runtime.ReadMemStats(&mem)
					actualMemoryGB = float64(mem.Alloc) / 1e9

					if actualMemoryGB >= float64(Keys.MemoryCap) {
						cclog.Errorf("[METRICSTORE]> after emergency frees (%d buffers), memory usage %.2f GB still at/above cap %d GB", freedEmergency, actualMemoryGB, Keys.MemoryCap)
					} else {
						cclog.Infof("[METRICSTORE]> emergency freeing complete: %d buffers freed, memory now %.2f GB", freedEmergency, actualMemoryGB)
					}
				}

				// Adaptive ticker: check more frequently when memory is high
				memoryRatio := actualMemoryGB / float64(Keys.MemoryCap)
				if memoryRatio > 0.8 && currentInterval != fastInterval {
					ticker.Reset(fastInterval)
					currentInterval = fastInterval
					cclog.Infof("[METRICSTORE]> memory at %.0f%% of cap, switching to fast check interval (30s)", memoryRatio*100)
				} else if memoryRatio <= 0.8 && currentInterval != normalInterval {
					ticker.Reset(normalInterval)
					currentInterval = normalInterval
					cclog.Infof("[METRICSTORE]> memory at %.0f%% of cap, switching to normal check interval", memoryRatio*100)
				}
			}
		}
	})
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
		return ms.Free(nil, t.Unix())

	// Else free every cluster/node except the used ones, pruning node Levels
	// that become empty in the same locked traversal.
	default:
		return ms.root.freeExcludingUsed(t.Unix(), excludeSelectors)
	}
}

// isNodeUsed reports whether cluster/host appears in the used-nodes map
// returned by NodeProvider.GetUsedNodes. Host lists are sorted per the
// interface contract, so lookup is a binary search. A nil map means no
// node is in use.
func isNodeUsed(used map[string][]string, cluster, host string) bool {
	hosts, ok := used[cluster]
	if !ok {
		return false
	}
	_, found := slices.BinarySearch(hosts, host)
	return found
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
func (m *MemoryStore) Read(selector util.Selector, metric string, from, to, resolution int64, resampleAlgo string) ([]schema.Float, int64, int64, int64, error) {
	if from > to {
		return nil, 0, 0, 0, errors.New("[METRICSTORE]> invalid time range")
	}

	minfo, ok := m.Metrics[metric]
	if !ok {
		return nil, 0, 0, 0, errors.New("[METRICSTORE]> unknown metric: " + metric)
	}

	// data spans the full requested window; every scope's read() writes into the
	// same index-aligned slice (NaN where it has no value), so aggregation never
	// needs trimming. dataFrom/dataTo track the real (non-NaN) extent of the first
	// scope seen; later scopes that report a different extent are misaligned. We
	// no longer abort on misalignment — the full NaN-padded window is still
	// returned for display — but we log it so the condition stays visible.
	n, data := 0, make([]schema.Float, (to-from)/minfo.Frequency+1)
	var dataFrom, dataTo int64

	err := m.root.findBuffers(selector, minfo.offset, func(b *buffer, path []string) error {
		cdata, cfrom, cto, err := b.read(from, to, data, false)
		if err != nil {
			return err
		}

		if n == 0 {
			dataFrom, dataTo = cfrom, cto
		} else if cfrom != dataFrom || cto != dataTo {
			node := strings.Join(path, "/")
			missingfront, missingback := int((dataFrom-cfrom)/minfo.Frequency), int((dataTo-cto)/minfo.Frequency)
			switch {
			case missingfront != 0:
				cclog.Warnf("%s", fmt.Errorf("%w: metric=%s node=%s buf#%d freq=%d expected[%d,%d] actual[%d,%d] missingfront=%d pts",
					ErrDataDoesNotAlignMissingFront, metric, node, n, minfo.Frequency, dataFrom, dataTo, cfrom, cto, missingfront))
			case missingback != 0:
				cclog.Warnf("%s", fmt.Errorf("%w: metric=%s node=%s buf#%d freq=%d expected[%d,%d] actual[%d,%d] missingback=%d pts",
					ErrDataDoesNotAlignMissingBack, metric, node, n, minfo.Frequency, dataFrom, dataTo, cfrom, cto, missingback))
			default:
				cclog.Warnf("%s", fmt.Errorf("%w: metric=%s node=%s buf#%d expected[%d,%d] actual[%d,%d]",
					ErrDataDoesNotAlignDataLenMismatch, metric, node, n, dataFrom, dataTo, cfrom, cto))
			}
		}

		fmt.Printf("Gather data - cto: %d, cfrom: %d, dto: %d, dfrom: %d\n", cto, cfrom, dataTo, dataFrom)

		data = cdata
		n += 1
		return nil
	}, nil)

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

	resampleFn, rfErr := resampler.GetResampler(resampleAlgo)
	if rfErr != nil {
		return nil, 0, 0, 0, rfErr
	}
	data, resolution, err = resampleFn(data, minfo.Frequency, resolution)
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
	m.root.lock.Lock()
	defer m.root.lock.Unlock()

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
