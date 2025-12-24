// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func Validate(schema string, instance json.RawMessage) {
	sch, err := jsonschema.CompileString("schema.json", schema)
	if err != nil {
		cclog.Fatalf("%#v", err)
	}

	var v any
	if err := json.Unmarshal([]byte(instance), &v); err != nil {
		cclog.Fatal(err)
	}

	if err = sch.Validate(v); err != nil {
		cclog.Fatalf("%#v", err)
	}
}
