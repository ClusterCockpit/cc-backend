// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFindFilesRetainsLatestPossibleOverlap(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"100.json", "200.bin"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		from int64
		want []string
	}{
		{50, []string{"100.json", "200.bin"}},
		{150, []string{"100.json", "200.bin"}},
		{200, []string{"200.bin"}},
		{250, []string{"200.bin"}},
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, err := findFiles(entries, tc.from, true)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("findFiles(from=%d) = %v, %v; want %v", tc.from, got, err, tc.want)
		}
	}
}
