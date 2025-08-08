// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package memorystore

type MetricStoreConfig struct {
	Checkpoints struct {
		FileFormat string `json:"file-format"`
		Interval   string `json:"interval"`
		RootDir    string `json:"directory"`
		Restore    string `json:"restore"`
	} `json:"checkpoints"`
	Debug struct {
		DumpToFile string `json:"dump-to-file"`
		EnableGops bool   `json:"gops"`
	} `json:"debug"`
	RetentionInMemory string `json:"retention-in-memory"`
	Archive           struct {
		Interval      string `json:"interval"`
		RootDir       string `json:"directory"`
		DeleteInstead bool   `json:"delete-instead"`
	} `json:"archive"`
}

var Keys MetricStoreConfig
