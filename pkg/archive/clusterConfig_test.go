// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"testing"
)

func TestClusterConfig(t *testing.T) {
	var fsa FsArchive
	version, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
}
