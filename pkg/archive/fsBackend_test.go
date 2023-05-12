// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func TestInitEmptyPath(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"kind\":\"testdata/archive\"}"))
	if err == nil {
		t.Fatal(err)
	}
}

func TestInitNoJson(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("\"path\":\"testdata/archive\"}"))
	if err == nil {
		t.Fatal(err)
	}
}
func TestInitNotExists(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/job-archive\"}"))
	if err == nil {
		t.Fatal(err)
	}
}

func TestInit(t *testing.T) {
	var fsa FsArchive
	version, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}
	if fsa.path != "testdata/archive" {
		t.Fail()
	}
	if version != 1 {
		t.Fail()
	}
	if len(fsa.clusters) != 1 || fsa.clusters[0] != "emmy" {
		t.Fail()
	}
}

func TestLoadJobMetaInternal(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}

	job, err := loadJobMeta("testdata/archive/emmy/1404/397/1609300556/meta.json")
	if err != nil {
		t.Fatal(err)
	}

	if job.JobID != 1404397 {
		t.Fail()
	}
	if int(job.NumNodes) != len(job.Resources) {
		t.Fail()
	}
	if job.StartTime != 1609300556 {
		t.Fail()
	}
}

func TestLoadJobMeta(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}

	jobIn := schema.Job{BaseJob: schema.JobDefaults}
	jobIn.StartTime = time.Unix(1608923076, 0)
	jobIn.JobID = 1403244
	jobIn.Cluster = "emmy"

	job, err := fsa.LoadJobMeta(&jobIn)
	if err != nil {
		t.Fatal(err)
	}

	if job.JobID != 1403244 {
		t.Fail()
	}
	if int(job.NumNodes) != len(job.Resources) {
		t.Fail()
	}
	if job.StartTime != 1608923076 {
		t.Fail()
	}
}

func TestLoadJobData(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}

	jobIn := schema.Job{BaseJob: schema.JobDefaults}
	jobIn.StartTime = time.Unix(1608923076, 0)
	jobIn.JobID = 1403244
	jobIn.Cluster = "emmy"

	data, err := fsa.LoadJobData(&jobIn)
	if err != nil {
		t.Fatal(err)
	}

	for name, scopes := range data {
		fmt.Printf("Metric name: %s\n", name)

		if _, exists := scopes[schema.MetricScopeNode]; !exists {
			t.Fail()
		}
	}
}

func TestLoadCluster(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := fsa.LoadClusterCfg("emmy")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.SubClusters[0].CoresPerSocket != 4 {
		t.Fail()
	}
}

func TestIter(t *testing.T) {
	var fsa FsArchive
	_, err := fsa.Init(json.RawMessage("{\"path\":\"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}

	for job := range fsa.Iter(false) {
		fmt.Printf("Job %d\n", job.Meta.JobID)

		if job.Meta.Cluster != "emmy" {
			t.Fail()
		}
	}
}
