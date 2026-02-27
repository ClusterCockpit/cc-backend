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

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
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

// flattenCheckpointFile recursively converts a CheckpointFile tree into Parquet rows.
// The scope path is built from the hierarchy: host level is "node", then child names
// map to scope/scope_id (e.g., "socket0" → scope="socket", scope_id="0").
func flattenCheckpointFile(cf *CheckpointFile, cluster, hostname, scope, scopeID string, rows []ParquetMetricRow) []ParquetMetricRow {
	for metricName, cm := range cf.Metrics {
		ts := cm.Start
		for _, v := range cm.Data {
			if !v.IsNaN() {
				rows = append(rows, ParquetMetricRow{
					Cluster:   cluster,
					Hostname:  hostname,
					Metric:    metricName,
					Scope:     scope,
					ScopeID:   scopeID,
					Timestamp: ts,
					Frequency: cm.Frequency,
					Value:     float32(v),
				})
			}
			ts += cm.Frequency
		}
	}

	for childName, childCf := range cf.Children {
		childScope, childScopeID := parseScopeFromName(childName)
		rows = flattenCheckpointFile(childCf, cluster, hostname, childScope, childScopeID, rows)
	}

	return rows
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

// writeParquetArchive writes rows to a Parquet file with Zstd compression.
func writeParquetArchive(filename string, rows []ParquetMetricRow) error {
	if err := os.MkdirAll(filepath.Dir(filename), CheckpointDirPerms); err != nil {
		return fmt.Errorf("creating archive directory: %w", err)
	}

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
	if err != nil {
		return fmt.Errorf("creating parquet file: %w", err)
	}
	defer f.Close()

	bw := bufio.NewWriterSize(f, 1<<20) // 1MB write buffer

	writer := pq.NewGenericWriter[ParquetMetricRow](bw,
		pq.Compression(&pq.Zstd),
		pq.SortingWriterConfig(pq.SortingColumns(
			pq.Ascending("cluster"),
			pq.Ascending("hostname"),
			pq.Ascending("metric"),
			pq.Ascending("timestamp"),
		)),
	)

	if _, err := writer.Write(rows); err != nil {
		return fmt.Errorf("writing parquet rows: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing parquet writer: %w", err)
	}

	if err := bw.Flush(); err != nil {
		return fmt.Errorf("flushing parquet file: %w", err)
	}

	return nil
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

// archiveCheckpointsToParquet reads checkpoint files for a host directory,
// converts them to Parquet rows. Returns the rows and filenames that were processed.
func archiveCheckpointsToParquet(dir, cluster, host string, from int64) ([]ParquetMetricRow, []string, error) {
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

	var rows []ParquetMetricRow

	for _, checkpoint := range files {
		filename := filepath.Join(dir, checkpoint)
		cf, err := loadCheckpointFileFromDisk(filename)
		if err != nil {
			cclog.Warnf("[METRICSTORE]> skipping unreadable checkpoint %s: %v", filename, err)
			continue
		}

		rows = flattenCheckpointFile(cf, cluster, host, "node", "", rows)
	}

	return rows, files, nil
}
