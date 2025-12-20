// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
)

func setup(tb testing.TB) *repository.JobRepository {
	tb.Helper()
	cclog.Init("warn", true)
	dbfile := "../repository/testdata/job.db"
	err := repository.MigrateDB(dbfile)
	noErr(tb, err)
	repository.Connect("sqlite3", dbfile)
	return repository.GetJobRepository()
}

func noErr(tb testing.TB, err error) {
	tb.Helper()

	if err != nil {
		tb.Fatal("Error is not nil:", err)
	}
}

func TestRegister(t *testing.T) {
	var tagger AppTagger

	err := tagger.Register()
	noErr(t, err)

	if len(tagger.apps) != 16 {
		t.Errorf("wrong summary for diagnostic \ngot: %d \nwant: 16", len(tagger.apps))
	}
}

func TestMatch(t *testing.T) {
	r := setup(t)

	job, err := r.FindByIDDirect(317)
	noErr(t, err)

	var tagger AppTagger

	err = tagger.Register()
	noErr(t, err)

	tagger.Match(job)

	if !r.HasTag(317, "app", "vasp") {
		t.Errorf("missing tag vasp")
	}
}
