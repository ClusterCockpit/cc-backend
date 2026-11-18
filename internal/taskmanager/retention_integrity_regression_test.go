// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"errors"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	pqa "github.com/ClusterCockpit/cc-backend/pkg/archive/parquet"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

var retentionFixtureError = errors.New("synthetic retention failure")

type retentionSourceFixture struct {
	archive.ArchiveBackend
	failAt string
}

type failFirstParquetTarget struct{ writes int }

func (target *failFirstParquetTarget) WriteFile(string, []byte) error {
	target.writes++
	if target.writes == 1 {
		return retentionFixtureError
	}
	return nil
}

func TestNodeStateTransferDoesNotSkipFailedRows(t *testing.T) {
	rows := []repository.NodeStateWithNode{{Hostname: "first"}, {Hostname: "second"}}
	if err := transferNodeStatesParquet(rows, &failFirstParquetTarget{}, 0); !errors.Is(err, retentionFixtureError) {
		t.Fatalf("failed node-state AddRow must block deletion: %v", err)
	}
}

func (source retentionSourceFixture) LoadClusterCfg(string) (*schema.Cluster, error) {
	if source.failAt == "cluster" {
		return nil, retentionFixtureError
	}
	return &schema.Cluster{Name: "testcluster"}, nil
}

func (source retentionSourceFixture) LoadJobMeta(job *schema.Job) (*schema.Job, error) {
	if source.failAt == "meta" {
		return nil, retentionFixtureError
	}
	return job, nil
}

func (source retentionSourceFixture) LoadJobData(*schema.Job) (schema.JobData, error) {
	if source.failAt == "data" {
		return schema.JobData{}, retentionFixtureError
	}
	return schema.JobData{}, nil
}

type retentionTargetFixture struct {
	archive.ArchiveBackend
	failAt string
}

func (target retentionTargetFixture) StoreClusterCfg(string, *schema.Cluster) error {
	if target.failAt == "cluster-write" {
		return retentionFixtureError
	}
	return nil
}

func (target retentionTargetFixture) ImportJob(*schema.Job, *schema.JobData) error {
	if target.failAt == "import" {
		return retentionFixtureError
	}
	return nil
}

func TestRetentionTransfersDoNotReportSuccessForSkippedJobs(t *testing.T) {
	jobs := []*schema.Job{{Cluster: "testcluster", JobID: 42}}
	for _, stage := range []string{"cluster", "meta", "data"} {
		t.Run("parquet-"+stage, func(t *testing.T) {
			target, err := pqa.NewFileTarget(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := transferJobsParquet(jobs, retentionSourceFixture{failAt: stage}, target, 16); !errors.Is(err, retentionFixtureError) {
				t.Fatalf("transfer must block destructive cleanup after %s failure: %v", stage, err)
			}
		})
		t.Run("json-"+stage, func(t *testing.T) {
			if err := transferJobsJSON(jobs, retentionSourceFixture{failAt: stage}, retentionTargetFixture{}); !errors.Is(err, retentionFixtureError) {
				t.Fatalf("transfer must block destructive cleanup after %s failure: %v", stage, err)
			}
		})
	}
	for _, stage := range []string{"cluster-write", "import"} {
		t.Run("json-"+stage, func(t *testing.T) {
			if err := transferJobsJSON(jobs, retentionSourceFixture{}, retentionTargetFixture{failAt: stage}); !errors.Is(err, retentionFixtureError) {
				t.Fatalf("transfer must block destructive cleanup after %s failure: %v", stage, err)
			}
		})
	}
}

func TestParquetTransferDoesNotSkipFailedFlush(t *testing.T) {
	jobs := []*schema.Job{{Cluster: "testcluster", JobID: 41}, {Cluster: "testcluster", JobID: 42}}
	if err := transferJobsParquet(jobs, retentionSourceFixture{}, &failFirstParquetTarget{}, 0); !errors.Is(err, retentionFixtureError) {
		t.Fatalf("failed Parquet AddJob flush must block cleanup: %v", err)
	}
}
