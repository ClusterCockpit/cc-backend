// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package config

import (
	"testing"

	_ "github.com/santhosh-tekuri/jsonschema/v5/httploader"
)

func TestInit(t *testing.T) {
	fp := "../../configs/config.json"
	Init(fp)
	if Keys.Addr != "0.0.0.0:443" {
		t.Errorf("wrong addr\ngot: %s \nwant: 0.0.0.0:443", Keys.Addr)
	}
}

func TestInitMinimal(t *testing.T) {
	fp := "../../docs/config.json"
	Init(fp)
	if Keys.Addr != "0.0.0.0:8080" {
		t.Errorf("wrong addr\ngot: %s \nwant: 0.0.0.0:8080", Keys.Addr)
	}
}
