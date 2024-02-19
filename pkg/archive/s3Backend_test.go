// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

func TestS3Init(t *testing.T) {
	var s3a S3Archive
	version, err := s3a.Init(json.RawMessage("{\"endpoint\":\"192.168.1.10:9100\",\"accessKeyID\":\"uACSaCN2Chiotpnr4bBS\",\"secretAccessKey\":\"MkEbBsFvMii1K5GreUriTJZxH359B1n28Au9Kaml\",\"bucket\":\"cc-archive\",\"useSSL\":false}"))
	if err != nil {
		t.Fatal(err)
	}
	if s3a.bucket != "cc-archive" {
		t.Errorf("S3 bucket \ngot: %s \nwant: cc-archive", s3a.bucket)
	}
	if version != 1 {
		t.Errorf("S3 archive version \ngot: %d \nwant: 1", version)
		t.Fail()
	}
	if len(s3a.clusters) != 2 || s3a.clusters[0] != "alex" {
		t.Fail()
	}
}

func TestS3LoadJobMeta(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage("{\"endpoint\":\"192.168.1.10:9100\",\"accessKeyID\":\"uACSaCN2Chiotpnr4bBS\",\"secretAccessKey\":\"MkEbBsFvMii1K5GreUriTJZxH359B1n28Au9Kaml\",\"bucket\":\"cc-archive\",\"useSSL\":false}"))
	if err != nil {
		t.Fatal(err)
	}

	jobIn := schema.Job{BaseJob: schema.JobDefaults}
	jobIn.StartTime = time.Unix(1675954353, 0)
	jobIn.JobID = 398764
	jobIn.Cluster = "fritz"

	job, err := s3a.LoadJobMeta(&jobIn)
	if err != nil {
		t.Fatal(err)
	}

	if job.JobID != 398764 {
		t.Fail()
	}
	if int(job.NumNodes) != len(job.Resources) {
		t.Fail()
	}
	if job.StartTime != 1675954353 {
		t.Fail()
	}
}

func TestS3LoadJobData(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage("{\"endpoint\":\"192.168.1.10:9100\",\"accessKeyID\":\"uACSaCN2Chiotpnr4bBS\",\"secretAccessKey\":\"MkEbBsFvMii1K5GreUriTJZxH359B1n28Au9Kaml\",\"bucket\":\"cc-archive\",\"useSSL\":false}"))
	if err != nil {
		t.Fatal(err)
	}

	jobIn := schema.Job{BaseJob: schema.JobDefaults}
	jobIn.StartTime = time.Unix(1675954353, 0)
	jobIn.JobID = 398764
	jobIn.Cluster = "fritz"

	data, err := s3a.LoadJobData(&jobIn)
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

func TestS3LoadCluster(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage("{\"endpoint\":\"192.168.1.10:9100\",\"accessKeyID\":\"uACSaCN2Chiotpnr4bBS\",\"secretAccessKey\":\"MkEbBsFvMii1K5GreUriTJZxH359B1n28Au9Kaml\",\"bucket\":\"cc-archive\",\"useSSL\":false}"))
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := s3a.LoadClusterCfg("fritz")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.SubClusters[0].CoresPerSocket != 36 {
		t.Fail()
	}
}

func TestS3JobImport(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage("{\"endpoint\":\"192.168.1.10:9100\",\"accessKeyID\":\"uACSaCN2Chiotpnr4bBS\",\"secretAccessKey\":\"MkEbBsFvMii1K5GreUriTJZxH359B1n28Au9Kaml\",\"bucket\":\"cc-archive\",\"useSSL\":false}"))
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile("./testdata/archive/fritz/398/759/1675954289/meta.json")
	if err != nil {
		t.Fatal("Error while reading metadata file for import")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	jobMeta := schema.JobMeta{BaseJob: schema.JobDefaults}
	if err = dec.Decode(&jobMeta); err != nil {
		t.Fatal("Error while decoding raw json metadata for import")
	}

	raw, err = os.ReadFile("./testdata/archive/fritz/398/759/1675954289/data.json")
	if err != nil {
		t.Fatal("Error while reading jobdata file for import")
	}

	dec = json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	jobData := schema.JobData{}
	if err = dec.Decode(&jobData); err != nil {
		t.Fatal("Error while decoding raw json jobdata for import")
	}

	s3a.ImportJob(&jobMeta, &jobData)
}

func TestS3Iter(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage("{\"endpoint\":\"192.168.1.10:9100\",\"accessKeyID\":\"uACSaCN2Chiotpnr4bBS\",\"secretAccessKey\":\"MkEbBsFvMii1K5GreUriTJZxH359B1n28Au9Kaml\",\"bucket\":\"cc-archive\",\"useSSL\":false}"))
	if err != nil {
		t.Fatal(err)
	}

	for jobContainer := range s3a.Iter(false) {
		if jobContainer.Meta == nil {
			fmt.Println("Is nil")
		}
	}
}
