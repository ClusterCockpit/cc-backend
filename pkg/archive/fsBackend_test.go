// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
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
	if version != 3 {
		t.Fail()
	}
	if len(fsa.clusters) != 3 || fsa.clusters[1] != "emmy" {
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

	jobIn := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}
	jobIn.StartTime = 1608923076
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
	_, err := fsa.Init(json.RawMessage("{\"path\": \"testdata/archive\"}"))
	if err != nil {
		t.Fatal(err)
	}

	jobIn := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}
	jobIn.StartTime = 1608923076
	jobIn.JobID = 1403244
	jobIn.Cluster = "emmy"

	data, err := fsa.LoadJobData(&jobIn)
	if err != nil {
		t.Fatal(err)
	}

	for _, scopes := range data {
		// fmt.Printf("Metric name: %s\n", name)

		if _, exists := scopes[schema.MetricScopeNode]; !exists {
			t.Fail()
		}
	}
}

func BenchmarkLoadJobData(b *testing.B) {
	tmpdir := b.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	util.CopyDir("./testdata/archive/", jobarchive)
	archiveCfg := fmt.Sprintf("{\"path\": \"%s\"}", jobarchive)

	var fsa FsArchive
	fsa.Init(json.RawMessage(archiveCfg))

	jobIn := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}
	jobIn.StartTime = 1608923076
	jobIn.JobID = 1403244
	jobIn.Cluster = "emmy"

	util.UncompressFile(filepath.Join(jobarchive, "emmy/1403/244/1608923076/data.json.gz"),
		filepath.Join(jobarchive, "emmy/1403/244/1608923076/data.json"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsa.LoadJobData(&jobIn)
	}
}

func BenchmarkLoadJobDataCompressed(b *testing.B) {
	tmpdir := b.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	util.CopyDir("./testdata/archive/", jobarchive)
	archiveCfg := fmt.Sprintf("{\"path\": \"%s\"}", jobarchive)

	var fsa FsArchive
	fsa.Init(json.RawMessage(archiveCfg))

	jobIn := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}
	jobIn.StartTime = 1608923076
	jobIn.JobID = 1403244
	jobIn.Cluster = "emmy"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsa.LoadJobData(&jobIn)
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
