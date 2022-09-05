// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema"
)

type Kind int

const (
	Meta Kind = iota + 1
	Data
	Cluster
)

func Validate(k Kind, v io.Reader) (err error) {
	var s *jsonschema.Schema

	switch k {
	case Meta:
		s, err = jsonschema.Compile("https://raw.githubusercontent.com/ClusterCockpit/cc-specifications/master/datastructures/job-meta.schema.json")
	case Data:
		s, err = jsonschema.Compile("https://raw.githubusercontent.com/ClusterCockpit/cc-specifications/master/datastructures/job-data.schema.json")
	case Cluster:
		s, err = jsonschema.Compile("https://raw.githubusercontent.com/ClusterCockpit/cc-specifications/master/datastructures/cluster.schema.json")
	default:
		return fmt.Errorf("unkown schema kind ")
	}

	if err != nil {
		return err
	}

	if err = s.Validate(v); err != nil {
		return err
	}

	return nil
}
