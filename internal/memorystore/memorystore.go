// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package memorystore provides an efficient in-memory time-series metric storage system
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
package memorystore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/resampler"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
)

var (
	singleton  sync.Once
	msInstance *MemoryStore
	// shutdownFunc stores the context cancellation function created in Init
	// and is called during Shutdown to cancel all background goroutines
	shutdownFunc context.CancelFunc
)



type Metric struct {
	Name         string
	Value        schema.Float
	MetricConfig MetricConfig
}

type MemoryStore struct {
	Metrics map[string]MetricConfig
	root    Level
}

func Init(rawConfig json.RawMessage, wg *sync.WaitGroup) {
	startupTime := time.Now()

	if rawConfig != nil {
		config.Validate(configSchema, rawConfig)
		dec := json.NewDecoder(bytes.NewReader(rawConfig))
		// dec.DisallowUnknownFields()
		if err := dec.Decode(&Keys); err != nil {
			cclog.Abortf("[METRICSTORE]> Metric Store Config Init: Could not decode config file '%s'.\nError: %s\n", rawConfig, err.Error())
		}
	}

	// Set NumWorkers from config or use default
	if Keys.NumWorkers <= 0 {
		maxWorkers := 10
		Keys.NumWorkers = min(runtime.NumCPU()/2+1, maxWorkers)
	}
	cclog.Debugf("[METRICSTORE]> Using %d workers for checkpoint/archive operations\n", Keys.NumWorkers)

	// Helper function to add metric configuration
	addMetricConfig := func(mc schema.MetricConfig) {
		agg, err := AssignAggregationStrategy(mc.Aggregation)
		if err != nil {
			cclog.Warnf("Could not find aggregation strategy for metric config '%s': %s", mc.Name, err.Error())
		}

		AddMetric(mc.Name, MetricConfig{
			Frequency:   int64(mc.Timestep),
			Aggregation: agg,
		})
	}

	for _, c := range archive.Clusters {
		for _, mc := range c.MetricConfig {
			addMetricConfig(*mc)
		}

		for _, sc := range c.SubClusters {
			for _, mc := range sc.MetricConfig {
				addMetricConfig(mc)
			}
		}
	}

	// Pass the config.MetricStoreKeys
	InitMetrics(Metrics)

	ms := GetMemoryStore()

	d, err := time.ParseDuration(Keys.Checkpoints.Restore)
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
	runtime.GC()

	ctx, shutdown := context.WithCancel(context.Background())

	wg.Add(4)

	Retention(wg, ctx)
	Checkpointing(wg, ctx)
	Archiving(wg, ctx)
	DataStaging(wg, ctx)

	// Note: Signal handling has been removed from this function.
	// The caller is responsible for handling shutdown signals and calling
	// the shutdown() function when appropriate.
	// Store the shutdown function for later use by Shutdown()
	shutdownFunc = shutdown

	if Keys.Nats != nil {
		for _, natsConf := range Keys.Nats {
			// TODO: When multiple nats configs share a URL, do a single connect.
			wg.Add(1)
			nc := natsConf
			go func() {
				// err := ReceiveNats(conf.Nats, decodeLine, runtime.NumCPU()-1, ctx)
				err := ReceiveNats(nc, ms, 1, ctx)
				if err != nil {
					cclog.Fatal(err)
				}
				wg.Done()
			}()
		}
	}
}

// InitMetrics creates a new, initialized instance of a MemoryStore.
// Will panic if values in the metric configurations are invalid.
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

func GetMemoryStore() *MemoryStore {
	if msInstance == nil {
		cclog.Fatalf("[METRICSTORE]> MemoryStore not initialized!")
	}

	return msInstance
}

func Shutdown() {
	// Cancel the context to signal all background goroutines to stop
	if shutdownFunc != nil {
		shutdownFunc()
	}

	cclog.Infof("[METRICSTORE]> Writing to '%s'...\n", Keys.Checkpoints.RootDir)
	var files int
	var err error

	ms := GetMemoryStore()

	if Keys.Checkpoints.FileFormat == "json" {
		files, err = ms.ToCheckpoint(Keys.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
	} else {
		files, err = GetAvroStore().ToCheckpoint(Keys.Checkpoints.RootDir, true)
		close(LineProtocolMessages)
	}

	if err != nil {
		cclog.Errorf("[METRICSTORE]> Writing checkpoint failed: %s\n", err.Error())
	}
	cclog.Infof("[METRICSTORE]> Done! (%d files written)\n", files)
}

func getName(m *MemoryStore, i int) string {
	for key, val := range m.Metrics {
		if val.offset == i {
			return key
		}
	}
	return ""
}

func Retention(wg *sync.WaitGroup, ctx context.Context) {
	ms := GetMemoryStore()

	go func() {
		defer wg.Done()
		d, err := time.ParseDuration(Keys.RetentionInMemory)
		if err != nil {
			cclog.Fatal(err)
		}
		if d <= 0 {
			return
		}

		ticks := func() <-chan time.Time {
			d := d / 2
			if d <= 0 {
				return nil
			}
			return time.NewTicker(d).C
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				t := time.Now().Add(-d)
				cclog.Infof("[METRICSTORE]> start freeing buffers (older than %s)...\n", t.Format(time.RFC3339))
				freed, err := ms.Free(nil, t.Unix())
				if err != nil {
					cclog.Errorf("[METRICSTORE]> freeing up buffers failed: %s\n", err.Error())
				} else {
					cclog.Infof("[METRICSTORE]> done: %d buffers freed\n", freed)
				}
			}
		}
	}()
}

// Write all values in `metrics` to the level specified by `selector` for time `ts`.
// Look at `findLevelOrCreate` for how selectors work.
func (m *MemoryStore) Write(selector []string, ts int64, metrics []Metric) error {
	var ok bool
	for i, metric := range metrics {
		if metric.MetricConfig.Frequency == 0 {
			metric.MetricConfig, ok = m.Metrics[metric.Name]
			if !ok {
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
		return nil, 0, 0, 0, errors.New("[METRICSTORE]> unkown metric: " + metric)
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
		return nil, 0, 0, 0, errors.New("[METRICSTORE]> metric or host not found")
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

func (m *MemoryStore) FreeAll() error {
	for k := range m.root.children {
		delete(m.root.children, k)
	}

	return nil
}

func (m *MemoryStore) SizeInBytes() int64 {
	return m.root.sizeInBytes()
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
