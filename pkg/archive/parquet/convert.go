// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// JobToParquetRow converts job metadata and metric data into a flat ParquetJobRow.
// Nested fields are marshaled to JSON; metric data is gzip-compressed JSON.
func JobToParquetRow(meta *schema.Job, data *schema.JobData) (*ParquetJobRow, error) {
	resourcesJSON, err := json.Marshal(meta.Resources)
	if err != nil {
		return nil, fmt.Errorf("marshal resources: %w", err)
	}

	var statisticsJSON []byte
	if meta.Statistics != nil {
		statisticsJSON, err = json.Marshal(meta.Statistics)
		if err != nil {
			return nil, fmt.Errorf("marshal statistics: %w", err)
		}
	}

	var tagsJSON []byte
	if len(meta.Tags) > 0 {
		tagsJSON, err = json.Marshal(meta.Tags)
		if err != nil {
			return nil, fmt.Errorf("marshal tags: %w", err)
		}
	}

	var metaDataJSON []byte
	if meta.MetaData != nil {
		metaDataJSON, err = json.Marshal(meta.MetaData)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
	}

	var footprintJSON []byte
	if meta.Footprint != nil {
		footprintJSON, err = json.Marshal(meta.Footprint)
		if err != nil {
			return nil, fmt.Errorf("marshal footprint: %w", err)
		}
	}

	var energyFootJSON []byte
	if meta.EnergyFootprint != nil {
		energyFootJSON, err = json.Marshal(meta.EnergyFootprint)
		if err != nil {
			return nil, fmt.Errorf("marshal energy footprint: %w", err)
		}
	}

	metricDataGz, err := compressJobData(data)
	if err != nil {
		return nil, fmt.Errorf("compress metric data: %w", err)
	}

	return &ParquetJobRow{
		JobID:          meta.JobID,
		Cluster:        meta.Cluster,
		SubCluster:     meta.SubCluster,
		Partition:      meta.Partition,
		Project:        meta.Project,
		User:           meta.User,
		State:          string(meta.State),
		StartTime:      meta.StartTime,
		Duration:       meta.Duration,
		Walltime:       meta.Walltime,
		NumNodes:       meta.NumNodes,
		NumHWThreads:   meta.NumHWThreads,
		NumAcc:         meta.NumAcc,
		Energy:         meta.Energy,
		SMT:            meta.SMT,
		ResourcesJSON:  resourcesJSON,
		StatisticsJSON: statisticsJSON,
		TagsJSON:       tagsJSON,
		MetaDataJSON:   metaDataJSON,
		FootprintJSON:  footprintJSON,
		EnergyFootJSON: energyFootJSON,
		MetricDataGz:   metricDataGz,
	}, nil
}

// ParquetRowToJob converts a ParquetJobRow back into job metadata and metric data.
// This is the reverse of JobToParquetRow.
func ParquetRowToJob(row *ParquetJobRow) (*schema.Job, *schema.JobData, error) {
	meta := &schema.Job{
		JobID:        row.JobID,
		Cluster:      row.Cluster,
		SubCluster:   row.SubCluster,
		Partition:    row.Partition,
		Project:      row.Project,
		User:         row.User,
		State:        schema.JobState(row.State),
		StartTime:    row.StartTime,
		Duration:     row.Duration,
		Walltime:     row.Walltime,
		NumNodes:     row.NumNodes,
		NumHWThreads: row.NumHWThreads,
		NumAcc:       row.NumAcc,
		Energy:       row.Energy,
		SMT:          row.SMT,
	}

	if len(row.ResourcesJSON) > 0 {
		if err := json.Unmarshal(row.ResourcesJSON, &meta.Resources); err != nil {
			return nil, nil, fmt.Errorf("unmarshal resources: %w", err)
		}
	}

	if len(row.StatisticsJSON) > 0 {
		if err := json.Unmarshal(row.StatisticsJSON, &meta.Statistics); err != nil {
			return nil, nil, fmt.Errorf("unmarshal statistics: %w", err)
		}
	}

	if len(row.TagsJSON) > 0 {
		if err := json.Unmarshal(row.TagsJSON, &meta.Tags); err != nil {
			return nil, nil, fmt.Errorf("unmarshal tags: %w", err)
		}
	}

	if len(row.MetaDataJSON) > 0 {
		if err := json.Unmarshal(row.MetaDataJSON, &meta.MetaData); err != nil {
			return nil, nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	if len(row.FootprintJSON) > 0 {
		if err := json.Unmarshal(row.FootprintJSON, &meta.Footprint); err != nil {
			return nil, nil, fmt.Errorf("unmarshal footprint: %w", err)
		}
	}

	if len(row.EnergyFootJSON) > 0 {
		if err := json.Unmarshal(row.EnergyFootJSON, &meta.EnergyFootprint); err != nil {
			return nil, nil, fmt.Errorf("unmarshal energy footprint: %w", err)
		}
	}

	data, err := decompressJobData(row.MetricDataGz)
	if err != nil {
		return nil, nil, fmt.Errorf("decompress metric data: %w", err)
	}

	return meta, data, nil
}

func decompressJobData(data []byte) (*schema.JobData, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(gz); err != nil {
		return nil, err
	}

	var jobData schema.JobData
	if err := json.Unmarshal(buf.Bytes(), &jobData); err != nil {
		return nil, err
	}

	return &jobData, nil
}

func compressJobData(data *schema.JobData) ([]byte, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := gz.Write(jsonBytes); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
