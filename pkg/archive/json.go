// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"encoding/json"
	"io"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func DecodeJobData(r io.Reader, k string) (schema.JobData, error) {
	data := cache.Get(k, func() (value any, ttl time.Duration, size int) {
		var d schema.JobData
		if err := json.NewDecoder(r).Decode(&d); err != nil {
			cclog.Warn("Error while decoding raw job data json")
			return err, 0, 1000
		}

		return d, 1 * time.Hour, d.Size()
	})

	if err, ok := data.(error); ok {
		cclog.Warn("Error in decoded job data set")
		return schema.JobData{}, err
	}

	return data.(schema.JobData), nil
}

func DecodeJobStats(r io.Reader, k string) (schema.ScopedJobStats, error) {
	jobData, err := DecodeJobData(r, k)
	if err != nil {
		return schema.ScopedJobStats{}, err
	}

	// Convert schema.JobData to schema.ScopedJobStats
	scopedJobStats := schema.ScopedJobStats{
		Metrics: scopedStatsFromMetrics(jobData.Metrics),
	}

	for _, group := range jobData.Groups {
		statsGroup := schema.ScopedStatsGroup{Key: group.Key}
		for _, inst := range group.Instances {
			statsGroup.Instances = append(statsGroup.Instances, schema.ScopedStatsGroupInstance{
				Name:    inst.Name,
				Type:    inst.Type,
				Metrics: scopedStatsFromMetrics(inst.Metrics),
			})
		}
		scopedJobStats.Groups = append(scopedJobStats.Groups, statsGroup)
	}

	return scopedJobStats, nil
}

// scopedStatsFromMetrics reduces the full time series of every metric/scope to
// the per-series statistics. Scopes without any series are dropped so that
// callers can rely on len(stats[metric][scope]) being non-zero when present.
func scopedStatsFromMetrics(metrics map[string]schema.ScopedMetrics) map[string]schema.ScopedMetricStats {
	stats := make(map[string]schema.ScopedMetricStats, len(metrics))
	for metric, metricData := range metrics {
		scoped := make(schema.ScopedMetricStats, len(metricData))
		for scope, jobMetric := range metricData {
			if len(jobMetric.Series) == 0 {
				continue
			}

			series := make([]*schema.ScopedStats, 0, len(jobMetric.Series))
			for i := range jobMetric.Series {
				series = append(series, &schema.ScopedStats{
					Hostname: jobMetric.Series[i].Hostname,
					ID:       jobMetric.Series[i].ID,
					Data:     &jobMetric.Series[i].Statistics,
				})
			}
			scoped[scope] = series
		}

		if len(scoped) > 0 {
			stats[metric] = scoped
		}
	}

	return stats
}

func DecodeJobMeta(r io.Reader) (*schema.Job, error) {
	var d schema.Job
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		cclog.Warn("Error while decoding raw job meta json")
		return &d, err
	}

	// Sanitize parameters

	return &d, nil
}

func DecodeCluster(r io.Reader) (*schema.Cluster, error) {
	var c schema.Cluster
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		cclog.Warn("Error while decoding raw cluster json")
		return &c, err
	}

	// Sanitize parameters

	return &c, nil
}

func EncodeJobData(w io.Writer, d *schema.JobData) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(d); err != nil {
		cclog.Warn("Error while encoding new job data json")
		return err
	}

	return nil
}

func EncodeJobMeta(w io.Writer, d *schema.Job) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(d); err != nil {
		cclog.Warn("Error while encoding new job meta json")
		return err
	}

	return nil
}

func EncodeCluster(w io.Writer, c *schema.Cluster) error {
	if err := json.NewEncoder(w).Encode(c); err != nil {
		cclog.Warn("Error while encoding cluster json")
		return err
	}
	return nil
}
