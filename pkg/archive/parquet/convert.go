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
		Exclusive:      meta.Exclusive,
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
