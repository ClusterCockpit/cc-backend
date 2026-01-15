// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"

	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

func TestInit(t *testing.T) {
	fp := "../../configs/config.json"
	ccconf.Init(fp)
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		Init(cfg)
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if Keys.Addr != "0.0.0.0:443" {
		t.Errorf("wrong addr\ngot: %s \nwant: 0.0.0.0:443", Keys.Addr)
	}
}

func TestInitMinimal(t *testing.T) {
	fp := "../../configs/config-demo.json"
	ccconf.Init(fp)
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		Init(cfg)
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if Keys.Addr != "127.0.0.1:8080" {
		t.Errorf("wrong addr\ngot: %s \nwant: 127.0.0.1:8080", Keys.Addr)
	}
}
