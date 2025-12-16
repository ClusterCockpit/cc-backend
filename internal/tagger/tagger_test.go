// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-lib/schema"
)

func TestInit(t *testing.T) {
	Init()
}

func TestJobStartCallback(t *testing.T) {
	Init()
	r := setup(t)
	job, err := r.FindByIDDirect(525)
	noErr(t, err)

	jobs := make([]*schema.Job, 0, 1)
	jobs = append(jobs, job)

	repository.CallJobStartHooks(jobs)
	if !r.HasTag(525, "app", "python") {
		t.Errorf("missing tag python")
	}
}
