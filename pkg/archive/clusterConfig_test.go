// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive_test

import (
	"encoding/json"
	"testing"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/davecgh/go-spew/spew"
)

func TestClusterConfig(t *testing.T) {
	if err := archive.Init(json.RawMessage("{\"kind\":\"testdata/archive\"}"), false); err != nil {
		t.Fatal(err)
	}

	c := archive.GetCluster("fritz")
	spew.Dump(c)
	t.Fail()
}
