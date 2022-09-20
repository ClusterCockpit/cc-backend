// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

type Kind int

const (
	Meta Kind = iota + 1
	Data
	Config
	ClusterCfg
)

//go:embed schemas/*
var schemaFiles embed.FS

func Load(s string) (io.ReadCloser, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	f := u.Path
	return schemaFiles.Open(f)
}

func init() {
	jsonschema.Loaders["embedFS"] = Load
}

func Validate(k Kind, r io.Reader) (err error) {
	var s *jsonschema.Schema

	switch k {
	case Meta:
		s, err = jsonschema.Compile("embedFS://schemas/job-meta.schema.json")
	case Data:
		s, err = jsonschema.Compile("embedFS://schemas/job-data.schema.json")
	case ClusterCfg:
		s, err = jsonschema.Compile("embedFS://schemas/cluster.schema.json")
	case Config:
		s, err = jsonschema.Compile("embedFS://schemas/config.schema.json")
	default:
		return fmt.Errorf("unkown schema kind ")
	}

	if err != nil {
		return err
	}

	var v interface{}
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		log.Errorf("schema.Validate() - Failed to decode %v", err)
		return err
	}

	if err = s.Validate(v); err != nil {
		return fmt.Errorf("%#v", err)
	}

	return nil
}
