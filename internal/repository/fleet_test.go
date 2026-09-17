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

func TestFleetHeartbeatBatch(t *testing.T) {
	setup(t)
	repo := GetFleetRepository()

	// One row per state the batch has to distinguish.
	rows := []*ServiceDB{
		{Cluster: "fritz", Hostname: "n01", ServiceType: "agent", InstanceID: "iid-pending", Scope: "cluster", RegisteredAt: 1000},
		{Cluster: "fritz", Hostname: "n02", ServiceType: "agent", InstanceID: "iid-stale", Scope: "cluster", RegisteredAt: 1000},
		{Cluster: "fritz", Hostname: "n03", ServiceType: "agent", InstanceID: "iid-dead", Scope: "cluster", RegisteredAt: 1000},
	}
	for _, r := range rows {
		if _, err := repo.RegisterService(r); err != nil {
			t.Fatal(err)
		}
	}

	// Age n02 into 'stale' and terminate n03.
	if _, err := repo.Heartbeat("iid-stale", 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkStale(2000); err != nil {
		t.Fatal(err)
	}
	if err := repo.Deregister("iid-dead"); err != nil {
		t.Fatal(err)
	}

	t.Run("empty batch is a no-op", func(t *testing.T) {
		applied, err := repo.HeartbeatBatch(nil)
		noErr(t, err)
		if applied != 0 {
			t.Fatalf("want 0 rows applied, got %d", applied)
		}
	})

	t.Run("applies known rows and skips deregistered and unknown ones", func(t *testing.T) {
		applied, err := repo.HeartbeatBatch(map[string]int64{
			"iid-pending": 5000,
			"iid-stale":   5000,
			"iid-dead":    5000,
			"iid-nobody":  5000,
		})
		noErr(t, err)
		if applied != 2 {
			t.Fatalf("want 2 rows applied (pending + stale), got %d", applied)
		}

		for _, id := range []string{"iid-pending", "iid-stale"} {
			svc, err := repo.GetByInstanceID(id)
			noErr(t, err)
			if svc.State != "active" {
				t.Errorf("%s: want state active, got %q", id, svc.State)
			}
			if !svc.LastHeartbeat.Valid || svc.LastHeartbeat.Int64 != 5000 {
				t.Errorf("%s: want last_heartbeat 5000, got %+v", id, svc.LastHeartbeat)
			}
		}

		dead, err := repo.GetByInstanceID("iid-dead")
		noErr(t, err)
		if dead.State != "deregistered" {
			t.Errorf("a deregistered row must never be revived, got state %q", dead.State)
		}
		if dead.LastHeartbeat.Valid {
			t.Errorf("a deregistered row must not get a heartbeat, got %+v", dead.LastHeartbeat)
		}
	})

	t.Run("unknown instance ids never insert a row", func(t *testing.T) {
		before, err := repo.ListByCluster("fritz")
		noErr(t, err)

		applied, err := repo.HeartbeatBatch(map[string]int64{"iid-ghost": 6000})
		noErr(t, err)
		if applied != 0 {
			t.Fatalf("want 0 rows applied, got %d", applied)
		}

		after, err := repo.ListByCluster("fritz")
		noErr(t, err)
		if len(after) != len(before) {
			t.Fatalf("row count changed: %d -> %d", len(before), len(after))
		}
	})
}
