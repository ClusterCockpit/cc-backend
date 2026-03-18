// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	pq "github.com/parquet-go/parquet-go"
)

// ParquetMetricRow is the long-format schema for archived metric data.
// One row per (host, metric, scope, scope_id, timestamp) data point.
// Sorted by (cluster, hostname, metric, timestamp) for optimal compression.
type ParquetMetricRow struct {
	Cluster   string  `parquet:"cluster"`
	Hostname  string  `parquet:"hostname"`
	Metric    string  `parquet:"metric"`
	Scope     string  `parquet:"scope"`
	ScopeID   string  `parquet:"scope_id"`
	Timestamp int64   `parquet:"timestamp"`
	Frequency int64   `parquet:"frequency"`
	Value     float32 `parquet:"value"`
}

// parseScopeFromName infers scope and scope_id from a child level name.
// Examples: "socket0" → ("socket", "0"), "core12" → ("core", "12"),
// "a0" (accelerator) → ("accelerator", "0").
// If the name doesn't match known patterns, it's used as-is for scope with empty scope_id.
func parseScopeFromName(name string) (string, string) {
	prefixes := []struct {
		prefix string
		scope  string
	}{
		{"socket", "socket"},
		{"memoryDomain", "memoryDomain"},
		{"core", "core"},
		{"hwthread", "hwthread"},
		{"cpu", "hwthread"},
		{"accelerator", "accelerator"},
	}

	for _, p := range prefixes {
		if len(name) > len(p.prefix) && name[:len(p.prefix)] == p.prefix {
			id := name[len(p.prefix):]
			if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
				return p.scope, id
			}
		}
	}

	return name, ""
}

// parquetArchiveWriter supports incremental writes to a Parquet file.
// Uses streaming writes to avoid accumulating all rows in memory.
type parquetArchiveWriter struct {
	writer *pq.GenericWriter[ParquetMetricRow]
	bw     *bufio.Writer
	f      *os.File
	batch  []ParquetMetricRow // reusable batch buffer
	count  int
}

const parquetBatchSize = 1024

// newParquetArchiveWriter creates a streaming Parquet writer with Zstd compression.
func newParquetArchiveWriter(filename string) (*parquetArchiveWriter, error) {
	if err := os.MkdirAll(filepath.Dir(filename), CheckpointDirPerms); err != nil {
		return nil, fmt.Errorf("creating archive directory: %w", err)
	}

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
	if err != nil {
		return nil, fmt.Errorf("creating parquet file: %w", err)
	}

	bw := bufio.NewWriterSize(f, 1<<20) // 1MB write buffer

	writer := pq.NewGenericWriter[ParquetMetricRow](bw,
		pq.Compression(&pq.Zstd),
	)

	return &parquetArchiveWriter{
		writer: writer,
		bw:     bw,
		f:      f,
		batch:  make([]ParquetMetricRow, 0, parquetBatchSize),
	}, nil
}

// WriteCheckpointFile streams a CheckpointFile tree directly to Parquet rows,
// writing metrics in sorted order without materializing all rows in memory.
// Produces one row group per call (typically one host's data).
func (w *parquetArchiveWriter) WriteCheckpointFile(cf *CheckpointFile, cluster, hostname, scope, scopeID string) error {
	w.writeLevel(cf, cluster, hostname, scope, scopeID)

	// Flush remaining batch
	if len(w.batch) > 0 {
		if _, err := w.writer.Write(w.batch); err != nil {
			return fmt.Errorf("writing parquet rows: %w", err)
		}
		w.count += len(w.batch)
		w.batch = w.batch[:0]
	}

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("flushing parquet row group: %w", err)
	}

	return nil
}

// writeLevel recursively writes metrics from a CheckpointFile level.
// Metric names and child names are sorted for deterministic, compression-friendly output.
func (w *parquetArchiveWriter) writeLevel(cf *CheckpointFile, cluster, hostname, scope, scopeID string) {
	// Sort metric names for deterministic order
	metricNames := make([]string, 0, len(cf.Metrics))
	for name := range cf.Metrics {
		metricNames = append(metricNames, name)
	}
	sort.Strings(metricNames)

	for _, metricName := range metricNames {
		cm := cf.Metrics[metricName]
		ts := cm.Start
		for _, v := range cm.Data {
			if !v.IsNaN() {
				w.batch = append(w.batch, ParquetMetricRow{
					Cluster:   cluster,
					Hostname:  hostname,
					Metric:    metricName,
					Scope:     scope,
					ScopeID:   scopeID,
					Timestamp: ts,
					Frequency: cm.Frequency,
					Value:     float32(v),
				})

				if len(w.batch) >= parquetBatchSize {
					w.writer.Write(w.batch)
					w.count += len(w.batch)
					w.batch = w.batch[:0]
				}
			}
			ts += cm.Frequency
		}
	}

	// Sort child names for deterministic order
	childNames := make([]string, 0, len(cf.Children))
	for name := range cf.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)

	for _, childName := range childNames {
		childScope, childScopeID := parseScopeFromName(childName)
		w.writeLevel(cf.Children[childName], cluster, hostname, childScope, childScopeID)
	}
}

// Close finalises the Parquet file (footer, buffered I/O, file handle).
func (w *parquetArchiveWriter) Close() error {
	if err := w.writer.Close(); err != nil {
		w.f.Close()
		return fmt.Errorf("closing parquet writer: %w", err)
	}

	if err := w.bw.Flush(); err != nil {
		w.f.Close()
		return fmt.Errorf("flushing parquet file: %w", err)
	}

	return w.f.Close()
}

// loadCheckpointFileFromDisk reads a JSON or binary checkpoint file and returns
// a CheckpointFile. Used by the Parquet archiver to read checkpoint data
// before converting it to Parquet format.
func loadCheckpointFileFromDisk(filename string) (*CheckpointFile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ext := filepath.Ext(filename)
	switch ext {
	case ".json":
		cf := &CheckpointFile{}
		br := bufio.NewReader(f)
		if err := json.NewDecoder(br).Decode(cf); err != nil {
			return nil, fmt.Errorf("decoding JSON checkpoint %s: %w", filename, err)
		}
		return cf, nil

	case ".bin":
		br := bufio.NewReader(f)
		var magic uint32
		if err := binary.Read(br, binary.LittleEndian, &magic); err != nil {
			return nil, fmt.Errorf("reading magic from %s: %w", filename, err)
		}
		if magic != snapFileMagic {
			return nil, fmt.Errorf("invalid snapshot magic in %s: 0x%08X", filename, magic)
		}
		var fileFrom, fileTo int64
		if err := binary.Read(br, binary.LittleEndian, &fileFrom); err != nil {
			return nil, fmt.Errorf("reading from-timestamp from %s: %w", filename, err)
		}
		if err := binary.Read(br, binary.LittleEndian, &fileTo); err != nil {
			return nil, fmt.Errorf("reading to-timestamp from %s: %w", filename, err)
		}
		cf, err := readBinaryLevel(br)
		if err != nil {
			return nil, fmt.Errorf("reading binary level from %s: %w", filename, err)
		}
		cf.From = fileFrom
		cf.To = fileTo
		return cf, nil

	default:
		return nil, fmt.Errorf("unsupported checkpoint extension: %s", ext)
	}
}

// loadCheckpointFiles reads checkpoint files for a host directory and returns
// the loaded CheckpointFiles and their filenames. Processes one file at a time
// to avoid holding all checkpoint data in memory simultaneously.
func loadCheckpointFiles(dir string, from int64) ([]*CheckpointFile, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	files, err := findFiles(entries, from, false)
	if err != nil {
		return nil, nil, err
	}

	if len(files) == 0 {
		return nil, nil, nil
	}

	var checkpoints []*CheckpointFile
	var processedFiles []string

	for _, checkpoint := range files {
		filename := filepath.Join(dir, checkpoint)
		cf, err := loadCheckpointFileFromDisk(filename)
		if err != nil {
			continue
		}
		checkpoints = append(checkpoints, cf)
		processedFiles = append(processedFiles, checkpoint)
	}

	return checkpoints, processedFiles, nil
}
