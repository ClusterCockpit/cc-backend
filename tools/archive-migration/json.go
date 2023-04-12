// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"io"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func DecodeJobData(r io.Reader) (*JobData, error) {
	var d JobData
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, err
	}

	return &d, nil
}

func DecodeJobMeta(r io.Reader) (*JobMeta, error) {
	var d JobMeta
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, err
	}

	return &d, nil
}

func DecodeCluster(r io.Reader) (*Cluster, error) {
	var c Cluster
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		return nil, err
	}

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

func EncodeCluster(w io.Writer, c *schema.Cluster) error {
	// Sanitize parameters
	if err := json.NewEncoder(w).Encode(c); err != nil {
		return err
	}

	return nil
}
