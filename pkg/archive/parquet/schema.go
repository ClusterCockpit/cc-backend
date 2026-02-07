// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

type ParquetJobRow struct {
	JobID          int64   `parquet:"job_id"`
	Cluster        string  `parquet:"cluster"`
	SubCluster     string  `parquet:"sub_cluster"`
	Partition      string  `parquet:"partition,optional"`
	Project        string  `parquet:"project"`
	User           string  `parquet:"user"`
	State          string  `parquet:"job_state"`
	StartTime      int64   `parquet:"start_time"`
	Duration       int32   `parquet:"duration"`
	Walltime       int64   `parquet:"walltime"`
	NumNodes       int32   `parquet:"num_nodes"`
	NumHWThreads   int32   `parquet:"num_hwthreads"`
	NumAcc         int32   `parquet:"num_acc"`
	Exclusive      int32   `parquet:"exclusive"`
	Energy         float64 `parquet:"energy"`
	SMT            int32   `parquet:"smt"`
	ResourcesJSON  []byte  `parquet:"resources_json"`
	StatisticsJSON []byte  `parquet:"statistics_json,optional"`
	TagsJSON       []byte  `parquet:"tags_json,optional"`
	MetaDataJSON   []byte  `parquet:"meta_data_json,optional"`
	FootprintJSON  []byte  `parquet:"footprint_json,optional"`
	EnergyFootJSON []byte  `parquet:"energy_footprint_json,optional"`
	MetricDataGz   []byte  `parquet:"metric_data_gz"`
}
