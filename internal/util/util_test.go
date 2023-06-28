// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package util_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/util"
)

func TestCheckFileExists(t *testing.T) {
	tmpdir := t.TempDir()
	if !util.CheckFileExists(tmpdir) {
		t.Fatal("expected true, got false")
	}

	filePath := filepath.Join(tmpdir, "version.txt")

	if err := os.WriteFile(filePath, []byte(fmt.Sprintf("%d", 1)), 0666); err != nil {
		t.Fatal(err)
	}
	if !util.CheckFileExists(filePath) {
		t.Fatal("expected true, got false")
	}

	filePath = filepath.Join(tmpdir, "version-test.txt")
	if util.CheckFileExists(filePath) {
		t.Fatal("expected false, got true")
	}
}

func TestGetFileSize(t *testing.T) {
	tmpdir := t.TempDir()
	filePath := filepath.Join(tmpdir, "data.json")

	if s := util.GetFilesize(filePath); s > 0 {
		t.Fatalf("expected 0, got %d", s)
	}

	if err := os.WriteFile(filePath, []byte(fmt.Sprintf("%d", 1)), 0666); err != nil {
		t.Fatal(err)
	}
	if s := util.GetFilesize(filePath); s == 0 {
		t.Fatal("expected not 0, got 0")
	}
}

func TestGetFileCount(t *testing.T) {
	tmpdir := t.TempDir()

	if c := util.GetFilecount(tmpdir); c != 0 {
		t.Fatalf("expected 0, got %d", c)
	}

	filePath := filepath.Join(tmpdir, "data-1.json")
	if err := os.WriteFile(filePath, []byte(fmt.Sprintf("%d", 1)), 0666); err != nil {
		t.Fatal(err)
	}
	filePath = filepath.Join(tmpdir, "data-2.json")
	if err := os.WriteFile(filePath, []byte(fmt.Sprintf("%d", 1)), 0666); err != nil {
		t.Fatal(err)
	}
	if c := util.GetFilecount(tmpdir); c != 2 {
		t.Fatalf("expected 2, got %d", c)
	}

	if c := util.GetFilecount(filePath); c != 0 {
		t.Fatalf("expected 0, got %d", c)
	}
}
