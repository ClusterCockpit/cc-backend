// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package util

import (
	"os"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

func DiskUsage(dirpath string) float64 {
	var size int64

	dir, err := os.Open(dirpath)
	if err != nil {
		log.Errorf("DiskUsage() error: %v", err)
		return 0
	}
	defer dir.Close()

	files, err := dir.Readdir(-1)
	if err != nil {
		log.Errorf("DiskUsage() error: %v", err)
		return 0
	}

	for _, file := range files {
		size += file.Size()
	}

	return float64(size) * 1e-6
}
