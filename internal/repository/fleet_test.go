// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"testing"
)

func TestFleetRepository(t *testing.T) {
	setup(t) // migrates a fresh temp DB to the current schema version
	repo := GetFleetRepository()

	// A per-node (cluster) agent and a cluster-independent infra service.
	clusterSvc := &ServiceDB{
		Cluster: "fritz", Hostname: "node01", ServiceType: "agent",
		InstanceID: "iid-cluster", Scope: "cluster", RegisteredAt: 1000,
	}
	infraSvc := &ServiceDB{
		Cluster: "", Hostname: "ms01", ServiceType: "metric-store",
		InstanceID: "iid-infra", Scope: "infra", RegisteredAt: 2000,
	}

	if _, err := repo.RegisterService(clusterSvc); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterService(infraSvc); err != nil {
		t.Fatal(err)
	}

	t.Run("ListByScope isolates infra from cluster", func(t *testing.T) {
		infra, err := repo.ListByScope("infra")
		noErr(t, err)
		if len(infra) != 1 {
			t.Fatalf("want 1 infra service, got %d", len(infra))
		}
		if infra[0].InstanceID != "iid-infra" || infra[0].Cluster != "" || infra[0].Scope != "infra" {
			t.Fatalf("unexpected infra row: %+v", infra[0])
		}

		cluster, err := repo.ListByScope("cluster")
		noErr(t, err)
		if len(cluster) != 1 || cluster[0].InstanceID != "iid-cluster" {
			t.Fatalf("unexpected cluster scope rows: %+v", cluster)
		}
	})

	t.Run("ListByCluster sees only its cluster", func(t *testing.T) {
		rows, err := repo.ListByCluster("fritz")
		noErr(t, err)
		if len(rows) != 1 || rows[0].InstanceID != "iid-cluster" {
			t.Fatalf("unexpected ListByCluster(fritz): %+v", rows)
		}
	})

	t.Run("scope round-trips through GetByInstanceID", func(t *testing.T) {
		got, err := repo.GetByInstanceID("iid-infra")
		noErr(t, err)
		if got.Scope != "infra" || got.State != "pending" {
			t.Fatalf("unexpected: scope=%q state=%q", got.Scope, got.State)
		}
	})

	t.Run("heartbeat is instance-id keyed and scope-agnostic", func(t *testing.T) {
		affected, err := repo.Heartbeat("iid-infra", 3000)
		noErr(t, err)
		if affected != 1 {
			t.Fatalf("want 1 row affected, got %d", affected)
		}
		affected, err = repo.Heartbeat("does-not-exist", 3000)
		noErr(t, err)
		if affected != 0 {
			t.Fatalf("want 0 rows affected for unknown instance, got %d", affected)
		}
	})

	t.Run("ListActive returns only active services", func(t *testing.T) {
		// iid-infra was activated by the heartbeat above; iid-cluster is still pending.
		active, err := repo.ListActive()
		noErr(t, err)
		if len(active) != 1 || active[0].InstanceID != "iid-infra" || active[0].State != "active" {
			t.Fatalf("unexpected ListActive result: %+v", active)
		}
	})

	t.Run("config revision persists", func(t *testing.T) {
		noErr(t, repo.SetConfigRevision("iid-infra", 42))
		got, err := repo.GetByInstanceID("iid-infra")
		noErr(t, err)
		if got.ConfigRevision != 42 {
			t.Fatalf("want config_revision 42, got %d", got.ConfigRevision)
		}
	})

	t.Run("re-register keeps revision, resets scope and state", func(t *testing.T) {
		// Same identity triple, but arriving as an infra registration again.
		reReg := &ServiceDB{
			Cluster: "", Hostname: "ms01", ServiceType: "metric-store",
			InstanceID: "iid-infra-2", Scope: "infra", RegisteredAt: 4000,
		}
		if _, err := repo.RegisterService(reReg); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetByInstanceID("iid-infra-2")
		noErr(t, err)
		if got.State != "pending" || got.Scope != "infra" || got.ConfigRevision != 42 {
			t.Fatalf("unexpected after re-register: %+v", got)
		}
	})
}
