// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"fmt"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	pq "github.com/parquet-go/parquet-go"
)

// NodeStateParquetWriter batches ParquetNodeStateRows and flushes them to a target
// when the estimated size exceeds maxSizeBytes.
type NodeStateParquetWriter struct {
	target       ParquetTarget
	maxSizeBytes int64
	rows         []ParquetNodeStateRow
	currentSize  int64
	fileCounter  int
	datePrefix   string
}

// NewNodeStateParquetWriter creates a new writer for node state parquet files.
func NewNodeStateParquetWriter(target ParquetTarget, maxSizeMB int) *NodeStateParquetWriter {
	return &NodeStateParquetWriter{
		target:       target,
		maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
		datePrefix:   time.Now().Format("2006-01-02"),
	}
}

// AddRow adds a row to the current batch. If the estimated batch size
// exceeds the configured maximum, the batch is flushed first.
func (pw *NodeStateParquetWriter) AddRow(row ParquetNodeStateRow) error {
	rowSize := estimateNodeStateRowSize(&row)

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
func (pw *NodeStateParquetWriter) Flush() error {
	if len(pw.rows) == 0 {
		return nil
	}

	pw.fileCounter++
	fileName := fmt.Sprintf("cc-nodestate-%s-%03d.parquet", pw.datePrefix, pw.fileCounter)

	data, err := writeNodeStateParquetBytes(pw.rows)
	if err != nil {
		return fmt.Errorf("write parquet buffer: %w", err)
	}

	if err := pw.target.WriteFile(fileName, data); err != nil {
		return fmt.Errorf("write parquet file %q: %w", fileName, err)
	}

	cclog.Infof("NodeState retention: wrote %s (%d rows, %d bytes)", fileName, len(pw.rows), len(data))
	pw.rows = pw.rows[:0]
	pw.currentSize = 0
	return nil
}

// Close flushes any remaining rows and finalizes the writer.
func (pw *NodeStateParquetWriter) Close() error {
	return pw.Flush()
}

func writeNodeStateParquetBytes(rows []ParquetNodeStateRow) ([]byte, error) {
	var buf bytes.Buffer

	writer := pq.NewGenericWriter[ParquetNodeStateRow](&buf,
		pq.Compression(&pq.Zstd),
		pq.SortingWriterConfig(pq.SortingColumns(
			pq.Ascending("cluster"),
			pq.Ascending("subcluster"),
			pq.Ascending("hostname"),
			pq.Ascending("time_stamp"),
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

func estimateNodeStateRowSize(row *ParquetNodeStateRow) int64 {
	size := int64(100) // fixed numeric fields
	size += int64(len(row.NodeState) + len(row.HealthState) + len(row.HealthMetrics))
	size += int64(len(row.Hostname) + len(row.Cluster) + len(row.SubCluster))
	return size
}
