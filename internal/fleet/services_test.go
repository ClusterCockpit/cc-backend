// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	_ "github.com/mattn/go-sqlite3"
)

// setupDB migrates a fresh temp database to the current schema and wires the
// repository singletons to it, isolated per test.
func setupDB(t *testing.T) {
	t.Helper()
	cclog.Init("warn", true)

	dbfile := filepath.Join(t.TempDir(), "fleet.db")
	if err := repository.ResetConnection(); err != nil {
		t.Fatal(err)
	}
	if err := repository.MigrateDB(dbfile); err != nil {
		t.Fatal(err)
	}
	repository.Connect(dbfile)
	t.Cleanup(func() { repository.ResetConnection() })
}

func TestInfraRegistry(t *testing.T) {
	setupDB(t)
	reg := NewInfraRegistry()

	t.Run("register requires hostname and a valid service_type", func(t *testing.T) {
		if _, err := reg.Register(InfraRegistrationRequest{ServiceType: ServiceTypeMetricStore}); err == nil {
			t.Fatal("expected error for missing hostname")
		}
		if _, err := reg.Register(InfraRegistrationRequest{Hostname: "ms01"}); err == nil {
			t.Fatal("expected error for missing service_type")
		}
		if _, err := reg.Register(InfraRegistrationRequest{Hostname: "ms01", ServiceType: "bogus"}); err == nil {
			t.Fatal("expected error for unknown service_type")
		}
	})

	var instanceID string
	t.Run("register issues identity with revision 0", func(t *testing.T) {
		r, err := reg.Register(InfraRegistrationRequest{
			Hostname: "ms01", ServiceType: ServiceTypeMetricStore,
			MetaData: map[string]string{"version": "1.2.3"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if r.InstanceID == "" || r.ConfigRevision != 0 {
			t.Fatalf("unexpected registration: %+v", r)
		}
		instanceID = r.InstanceID
	})

	t.Run("get returns infra scope and empty cluster", func(t *testing.T) {
		svc, err := reg.Get(instanceID)
		if err != nil {
			t.Fatal(err)
		}
		if svc.Cluster != "" || svc.ServiceType != ServiceTypeMetricStore || svc.State != "pending" {
			t.Fatalf("unexpected service: %+v", svc)
		}
		if svc.MetaData["version"] != "1.2.3" {
			t.Fatalf("metadata not round-tripped: %+v", svc.MetaData)
		}
	})

	t.Run("heartbeat activates known, rejects unknown", func(t *testing.T) {
		if err := reg.Heartbeat(instanceID, time.Unix(5000, 0)); err != nil {
			t.Fatal(err)
		}
		svc, _ := reg.Get(instanceID)
		if svc.State != "active" || svc.LastHeartbeat == nil {
			t.Fatalf("heartbeat did not activate: %+v", svc)
		}
		if err := reg.Heartbeat("unknown", time.Unix(5000, 0)); !errors.Is(err, ErrUnknownInstance) {
			t.Fatalf("want ErrUnknownInstance, got %v", err)
		}
	})

	t.Run("list returns only infra services", func(t *testing.T) {
		// A cluster-scope agent must not appear in the infra listing.
		cReg := NewRegistry(time.Hour)
		if _, err := cReg.Register(RegistrationRequest{
			Cluster: "fritz", Hostname: "node01", ServiceType: ServiceTypeCollector,
		}); err != nil {
			t.Fatal(err)
		}

		list, err := reg.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ServiceType != ServiceTypeMetricStore {
			t.Fatalf("unexpected infra list: %+v", list)
		}
	})

	t.Run("ack config records revision, deregister is terminal", func(t *testing.T) {
		if err := reg.AckConfig(instanceID, 99); err != nil {
			t.Fatal(err)
		}
		svc, _ := reg.Get(instanceID)
		if svc.ConfigRevision != 99 {
			t.Fatalf("want revision 99, got %d", svc.ConfigRevision)
		}

		if err := reg.Deregister(instanceID); err != nil {
			t.Fatal(err)
		}
		if err := reg.Heartbeat(instanceID, time.Unix(6000, 0)); !errors.Is(err, ErrUnknownInstance) {
			t.Fatalf("heartbeat after deregister should fail: %v", err)
		}
	})
}
