// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return
	}

	err = out.Sync()
	if err != nil {
		return
	}

	si, err := os.Stat(src)
	if err != nil {
		return
	}
	err = os.Chmod(dst, si.Mode())
	if err != nil {
		return
	}

	return
}

// CopyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
// Symlinks are ignored and skipped.
func CopyDir(src string, dst string) (err error) {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("source is not a directory")
	}

	_, err = os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		return fmt.Errorf("destination already exists")
	}

	err = os.MkdirAll(dst, si.Mode())
	if err != nil {
		return
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = CopyDir(srcPath, dstPath)
			if err != nil {
				return
			}
		} else {
			// Skip symlinks.
			if entry.Mode()&os.ModeSymlink != 0 {
				continue
			}

			err = CopyFile(srcPath, dstPath)
			if err != nil {
				return
			}
		}
	}

	return
}

var jobs []*schema.Job

func setup(t *testing.T) archive.ArchiveBackend {
	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	CopyDir("./testdata/archive/", jobarchive)
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	if err := archive.Init(json.RawMessage(archiveCfg), false); err != nil {
		t.Fatal(err)
	}

	jobs = make([]*schema.Job, 2)
	jobs[0] = &schema.Job{}
	jobs[0].JobID = 1403244
	jobs[0].Cluster = "emmy"
	jobs[0].StartTime = time.Unix(1608923076, 0)

	jobs[1] = &schema.Job{}
	jobs[0].JobID = 1404397
	jobs[0].Cluster = "emmy"
	jobs[0].StartTime = time.Unix(1609300556, 0)

	return archive.GetHandle()
}

func TestCleanUp(t *testing.T) {
	a := setup(t)
	if !a.Exists(jobs[0]) {
		t.Error("Job does not exist")
	}

	a.CleanUp(jobs)

	if a.Exists(jobs[0]) || a.Exists(jobs[1]) {
		t.Error("Jobs still exist")
	}
}

// func TestCompress(t *testing.T) {
// 	a := setup(t)
// 	if !a.Exists(jobs[0]) {
// 		t.Error("Job does not exist")
// 	}
//
// 	a.Compress(jobs)
//
// 	if a.Exists(jobs[0]) || a.Exists(jobs[1]) {
// 		t.Error("Jobs still exist")
// 	}
// }
