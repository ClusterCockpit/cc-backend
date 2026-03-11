// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

func setup(tb testing.TB) *repository.JobRepository {
	tb.Helper()
	cclog.Init("warn", true)
	dbfile := "../repository/testdata/job.db"
	err := repository.MigrateDB(dbfile)
	noErr(tb, err)
	repository.Connect(dbfile)
	return repository.GetJobRepository()
}

func noErr(tb testing.TB, err error) {
	tb.Helper()

	if err != nil {
		tb.Fatal("Error is not nil:", err)
	}
}

func setupAppTaggerTestDir(t *testing.T) string {
	t.Helper()

	testDir := t.TempDir()
	appsDir := filepath.Join(testDir, "apps")
	err := os.MkdirAll(appsDir, 0o755)
	noErr(t, err)

	srcDir := "../../configs/tagger/apps"
	files, err := os.ReadDir(srcDir)
	noErr(t, err)

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		srcPath := filepath.Join(srcDir, file.Name())
		dstPath := filepath.Join(appsDir, file.Name())

		data, err := os.ReadFile(srcPath)
		noErr(t, err)

		err = os.WriteFile(dstPath, data, 0o644)
		noErr(t, err)
	}

	return appsDir
}

func TestRegister(t *testing.T) {
	appsDir := setupAppTaggerTestDir(t)

	var tagger AppTagger
	tagger.cfgPath = appsDir
	tagger.tagType = tagTypeApp
	tagger.apps = make([]appInfo, 0)

	files, err := os.ReadDir(appsDir)
	noErr(t, err)

	for _, fn := range files {
		if fn.IsDir() {
			continue
		}
		fns := fn.Name()
		f, err := os.Open(filepath.Join(appsDir, fns))
		noErr(t, err)
		tagger.scanApp(f, fns)
		f.Close()
	}

	if len(tagger.apps) != 16 {
		t.Errorf("wrong summary for diagnostic \ngot: %d \nwant: 16", len(tagger.apps))
	}
}

func TestMatch(t *testing.T) {
	appsDir := setupAppTaggerTestDir(t)
	r := setup(t)

	job, err := r.FindByIDDirect(317)
	noErr(t, err)

	var tagger AppTagger
	tagger.cfgPath = appsDir
	tagger.tagType = tagTypeApp
	tagger.apps = make([]appInfo, 0)

	files, err := os.ReadDir(appsDir)
	noErr(t, err)

	for _, fn := range files {
		if fn.IsDir() {
			continue
		}
		fns := fn.Name()
		f, err := os.Open(filepath.Join(appsDir, fns))
		noErr(t, err)
		tagger.scanApp(f, fns)
		f.Close()
	}

	tagger.Match(job)

	if !r.HasTag(317, "app", "vasp") {
		t.Errorf("missing tag vasp")
	}
}
