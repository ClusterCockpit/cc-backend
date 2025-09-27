// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"os"
	"strings"
)

type DefaultMetricsCluster struct {
	Name           string `json:"name"`
	DefaultMetrics string `json:"default_metrics"`
}

type DefaultMetricsConfig struct {
	Clusters []DefaultMetricsCluster `json:"clusters"`
}

func LoadDefaultMetricsConfig() (*DefaultMetricsConfig, error) {
	filePath := "default_metrics.json"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var cfg DefaultMetricsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func ParseMetricsString(s string) []string {
	parts := strings.Split(s, ",")
	var metrics []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			metrics = append(metrics, trimmed)
		}
	}
	return metrics
}
