// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"io"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func DecodeJobData(r io.Reader, k string) (schema.JobData, error) {
	data := cache.Get(k, func() (value interface{}, ttl time.Duration, size int) {
		var d schema.JobData
		if err := json.NewDecoder(r).Decode(&d); err != nil {
			log.Warn("Error while decoding raw job data json")
			return err, 0, 1000
		}

		return d, 1 * time.Hour, d.Size()
	})

	if err, ok := data.(error); ok {
		log.Warn("Error in decoded job data set")
		return nil, err
	}

	return data.(schema.JobData), nil
}

func DecodeJobStats(r io.Reader, k string) (schema.ScopedJobStats, error) {
	jobData, err := DecodeJobData(r, k)
	// Convert schema.JobData to schema.ScopedJobStats
	if jobData != nil {
		scopedJobStats := make(schema.ScopedJobStats)
		for metric, metricData := range jobData {
			if _, ok := scopedJobStats[metric]; !ok {
				scopedJobStats[metric] = make(map[schema.MetricScope][]*schema.ScopedStats)
			}

			for scope, jobMetric := range metricData {
				if _, ok := scopedJobStats[metric][scope]; !ok {
					scopedJobStats[metric][scope] = make([]*schema.ScopedStats, 0)
				}

				for _, series := range jobMetric.Series {
					scopedJobStats[metric][scope] = append(scopedJobStats[metric][scope], &schema.ScopedStats{
						Hostname: series.Hostname,
						Id:       series.Id,
						Data:     &series.Statistics,
					})
				}

				// So that one can later check len(scopedJobStats[metric][scope]): Remove from map if empty
				if len(scopedJobStats[metric][scope]) == 0 {
					delete(scopedJobStats[metric], scope)
					if len(scopedJobStats[metric]) == 0 {
						delete(scopedJobStats, metric)
					}
				}
			}
		}
		return scopedJobStats, nil
	}
	return nil, err
}

func DecodeJobMeta(r io.Reader) (*schema.Job, error) {
	var d schema.Job
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		log.Warn("Error while decoding raw job meta json")
		return &d, err
	}

	// Sanitize parameters

	return &d, nil
}

func DecodeCluster(r io.Reader) (*schema.Cluster, error) {
	var c schema.Cluster
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		log.Warn("Error while decoding raw cluster json")
		return &c, err
	}

	// Sanitize parameters

	return &c, nil
}

func EncodeJobData(w io.Writer, d *schema.JobData) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(d); err != nil {
		log.Warn("Error while encoding new job data json")
		return err
	}

	return nil
}

func EncodeJobMeta(w io.Writer, d *schema.Job) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(d); err != nil {
		log.Warn("Error while encoding new job meta json")
		return err
	}

	return nil
}
