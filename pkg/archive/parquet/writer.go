// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	pq "github.com/parquet-go/parquet-go"
)

// ParquetWriter batches ParquetJobRows and flushes them to a target
// when the estimated size exceeds maxSizeBytes.
type ParquetWriter struct {
	target       ParquetTarget
	maxSizeBytes int64
	rows         []ParquetJobRow
	currentSize  int64
	fileCounter  int
	datePrefix   string
}

// NewParquetWriter creates a new writer that flushes batches to the given target.
// maxSizeMB sets the approximate maximum size per parquet file in megabytes.
func NewParquetWriter(target ParquetTarget, maxSizeMB int) *ParquetWriter {
	return &ParquetWriter{
		target:       target,
		maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
		datePrefix:   time.Now().Format("2006-01-02"),
	}
}

// AddJob adds a row to the current batch. If the estimated batch size
// exceeds the configured maximum, the batch is flushed to the target first.
func (pw *ParquetWriter) AddJob(row ParquetJobRow) error {
	rowSize := estimateRowSize(&row)

	if pw.currentSize+rowSize > pw.maxSizeBytes && len(pw.rows) > 0 {
		if err := pw.Flush(); err != nil {
			return err
		}
	}

	pw.rows = append(pw.rows, row)
	pw.currentSize += rowSize
	return nil
}

// Flush writes the current batch to a parquet file on the target.
func (pw *ParquetWriter) Flush() error {
	if len(pw.rows) == 0 {
		return nil
	}

	pw.fileCounter++
	fileName := fmt.Sprintf("cc-archive-%s-%03d.parquet", pw.datePrefix, pw.fileCounter)

	data, err := writeParquetBytes(pw.rows)
	if err != nil {
		return fmt.Errorf("write parquet buffer: %w", err)
	}

	if err := pw.target.WriteFile(fileName, data); err != nil {
		return fmt.Errorf("write parquet file %q: %w", fileName, err)
	}

	cclog.Infof("Parquet retention: wrote %s (%d jobs, %d bytes)", fileName, len(pw.rows), len(data))
	pw.rows = pw.rows[:0]
	pw.currentSize = 0
	return nil
}

// Close flushes any remaining rows and finalizes the writer.
func (pw *ParquetWriter) Close() error {
	return pw.Flush()
}

func writeParquetBytes(rows []ParquetJobRow) ([]byte, error) {
	var buf bytes.Buffer

	writer := pq.NewGenericWriter[ParquetJobRow](&buf,
		pq.Compression(&pq.Zstd),
		pq.SortingWriterConfig(pq.SortingColumns(
			pq.Ascending("sub_cluster"),
			pq.Ascending("project"),
			pq.Ascending("start_time"),
		)),
	)

	if _, err := writer.Write(rows); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func estimateRowSize(row *ParquetJobRow) int64 {
	// Fixed fields: ~100 bytes for numeric fields + strings estimate
	size := int64(200)
	size += int64(len(row.Cluster) + len(row.SubCluster) + len(row.Partition) +
		len(row.Project) + len(row.User) + len(row.State))
	size += int64(len(row.ResourcesJSON))
	size += int64(len(row.StatisticsJSON))
	size += int64(len(row.TagsJSON))
	size += int64(len(row.MetaDataJSON))
	size += int64(len(row.FootprintJSON))
	size += int64(len(row.EnergyFootJSON))
	size += int64(len(row.MetricDataGz))
	return size
}

// prefixedTarget wraps a ParquetTarget and prepends a path prefix to all file names.
type prefixedTarget struct {
	inner  ParquetTarget
	prefix string
}

func (pt *prefixedTarget) WriteFile(name string, data []byte) error {
	return pt.inner.WriteFile(path.Join(pt.prefix, name), data)
}

// ClusterAwareParquetWriter organizes Parquet output by cluster.
// Each cluster gets its own subdirectory with a cluster.json config file.
type ClusterAwareParquetWriter struct {
	target       ParquetTarget
	maxSizeMB    int
	writers      map[string]*ParquetWriter
	clusterCfgs  map[string]*schema.Cluster
}

// NewClusterAwareParquetWriter creates a writer that routes jobs to per-cluster ParquetWriters.
func NewClusterAwareParquetWriter(target ParquetTarget, maxSizeMB int) *ClusterAwareParquetWriter {
	return &ClusterAwareParquetWriter{
		target:      target,
		maxSizeMB:   maxSizeMB,
		writers:     make(map[string]*ParquetWriter),
		clusterCfgs: make(map[string]*schema.Cluster),
	}
}

// SetClusterConfig stores a cluster configuration to be written as cluster.json on Close.
func (cw *ClusterAwareParquetWriter) SetClusterConfig(name string, cfg *schema.Cluster) {
	cw.clusterCfgs[name] = cfg
}

// AddJob routes the job row to the appropriate per-cluster writer.
func (cw *ClusterAwareParquetWriter) AddJob(row ParquetJobRow) error {
	cluster := row.Cluster
	pw, ok := cw.writers[cluster]
	if !ok {
		pw = NewParquetWriter(&prefixedTarget{inner: cw.target, prefix: cluster}, cw.maxSizeMB)
		cw.writers[cluster] = pw
	}
	return pw.AddJob(row)
}

// Close writes cluster.json files and flushes all per-cluster writers.
func (cw *ClusterAwareParquetWriter) Close() error {
	for name, cfg := range cw.clusterCfgs {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal cluster config %q: %w", name, err)
		}
		if err := cw.target.WriteFile(path.Join(name, "cluster.json"), data); err != nil {
			return fmt.Errorf("write cluster.json for %q: %w", name, err)
		}
	}

	for cluster, pw := range cw.writers {
		if err := pw.Close(); err != nil {
			return fmt.Errorf("close writer for cluster %q: %w", cluster, err)
		}
	}
	return nil
}
