// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"encoding/json"
	"fmt"
	"io"

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

func Validate(k Kind, r io.Reader) (err error) {
	var s *jsonschema.Schema

	switch k {
	case Meta:
		s, err = jsonschema.Compile("https://raw.githubusercontent.com/ClusterCockpit/cc-specifications/master/datastructures/job-meta.schema.json")
	case Data:
		s, err = jsonschema.Compile("https://raw.githubusercontent.com/ClusterCockpit/cc-specifications/master/datastructures/job-data.schema.json")
	case ClusterCfg:
		s, err = jsonschema.Compile("https://raw.githubusercontent.com/ClusterCockpit/cc-specifications/master/datastructures/cluster.schema.json")
	case Config:
		s, err = jsonschema.Compile("../../configs/config.schema.json")
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
