// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var jobs []*schema.Job

func setup(t *testing.T) archive.ArchiveBackend {
	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	util.CopyDir("./testdata/archive/", jobarchive)
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
