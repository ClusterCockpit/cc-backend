// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

type ParquetNodeStateRow struct {
	TimeStamp       int64  `parquet:"time_stamp"`
	NodeState       string `parquet:"node_state"`
	HealthState     string `parquet:"health_state"`
	HealthMetrics   string `parquet:"health_metrics,optional"`
	CpusAllocated   int32  `parquet:"cpus_allocated"`
	MemoryAllocated int64  `parquet:"memory_allocated"`
	GpusAllocated   int32  `parquet:"gpus_allocated"`
	JobsRunning     int32  `parquet:"jobs_running"`
	Hostname        string `parquet:"hostname"`
	Cluster         string `parquet:"cluster"`
	SubCluster      string `parquet:"subcluster"`
}
