// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileTargetDoesNotReplacePublishedParquet(t *testing.T) {
	root := t.TempDir()
	target, err := NewFileTarget(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.WriteFile("test.parquet", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := target.WriteFile("test.parquet", []byte("replacement")); err == nil {
		t.Fatal("published Parquet file was replaced")
	}
	content, err := os.ReadFile(filepath.Join(root, "test.parquet"))
	if err != nil || string(content) != "original" {
		t.Fatal("original Parquet content changed")
	}
}

func TestConcurrentFileTargetPublishKeepsOneCompleteOutput(t *testing.T) {
	root := t.TempDir()
	target, err := NewFileTarget(root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	contents := [][]byte{bytes.Repeat([]byte("first"), 1000), bytes.Repeat([]byte("second"), 1000)}
	for _, content := range contents {
		group.Add(1)
		go func(data []byte) { defer group.Done(); <-start; results <- target.WriteFile("same.parquet", data) }(content)
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, fs.ErrExist) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("got %d successful writers for one immutable output", successes)
	}
	actual, err := os.ReadFile(filepath.Join(root, "same.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, contents[0]) && !bytes.Equal(actual, contents[1]) {
		t.Fatal("published output is incomplete or mixed")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary output was leaked")
	}
}

func TestFileTargetUpdatesCompleteClusterMetadata(t *testing.T) {
	root := t.TempDir()
	target, err := NewFileTarget(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"old metadata", "new metadata"} {
		if err := target.WriteFile("cluster.json", []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := os.ReadFile(filepath.Join(root, "cluster.json"))
	if err != nil || string(actual) != "new metadata" {
		t.Fatal("cluster metadata update failed")
	}
	info, err := os.Stat(filepath.Join(root, "cluster.json"))
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatal("output permissions changed")
	}
}
