// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"io"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func DecodeJobData(r io.Reader, k string) (schema.JobData, error) {
	data := cache.Get(k, func() (value interface{}, ttl time.Duration, size int) {
		var d schema.JobData
		if err := json.NewDecoder(r).Decode(&d); err != nil {
			return err, 0, 1000
		}

		return d, 1 * time.Hour, d.Size()
	})

	if err, ok := data.(error); ok {
		return nil, err
	}

	return data.(schema.JobData), nil
}

func DecodeJobMeta(r io.Reader) (*schema.JobMeta, error) {
	var d schema.JobMeta
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return &d, err
	}

	// Sanitize parameters

	return &d, nil
}

func DecodeCluster(r io.Reader) (*schema.Cluster, error) {
	var c schema.Cluster
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		return &c, err
	}

	// Sanitize parameters

	return &c, nil
}

func EncodeJobData(w io.Writer, d *schema.JobData) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(d); err != nil {
		return err
	}

	return nil
}

func EncodeJobMeta(w io.Writer, d *schema.JobMeta) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(d); err != nil {
		return err
	}

	return nil
}
