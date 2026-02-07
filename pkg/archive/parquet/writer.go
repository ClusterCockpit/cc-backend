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
		pq.Compression(&pq.Snappy),
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
