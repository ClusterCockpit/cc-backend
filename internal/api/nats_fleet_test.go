// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/fleet"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	_ "github.com/mattn/go-sqlite3"
)

// setupFleetNats wires a fresh database plus fleet subsystem and returns a
// consumer whose workers are running. No broker is involved: the tests feed raw
// line protocol into the handler, exactly like the job/node NATS tests.
func setupFleetNats(t *testing.T) *FleetNatsAPI {
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

	if err := fleet.Init(context.Background(), fleet.Options{
		StaleAfter:    time.Hour,
		SweepInterval: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fleet.Reset)

	api := NewFleetNatsAPI()
	t.Cleanup(api.Shutdown)
	return api
}

// heartbeatLine builds one line protocol heartbeat.
func heartbeatLine(instanceID string) []byte {
	return heartbeatLineAt(instanceID, time.Now())
}

func heartbeatLineAt(instanceID string, tm time.Time) []byte {
	return []byte(fmt.Sprintf(`fleet,function=heartbeat event="{\"instanceId\":\"%s\"}" %d`,
		instanceID, tm.UnixNano()))
}

// registerForHeartbeat creates a cluster-scope registration and returns its id.
func registerForHeartbeat(t *testing.T, hostname string) string {
	t.Helper()
	reg, err := fleet.Get().Registry().Register(fleet.RegistrationRequest{
		Cluster: "fritz", Hostname: hostname, ServiceType: fleet.ServiceTypeMetricStore,
	})
	if err != nil {
		t.Fatal(err)
	}
	return reg.InstanceID
}

// waitForState polls until the row reaches want, or fails.
func waitForState(t *testing.T, instanceID, want string) *repository.ServiceDB {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		svc, err := repository.GetFleetRepository().GetByInstanceID(instanceID)
		if err != nil {
			t.Fatal(err)
		}
		if svc.State == want {
			return svc
		}
		last = svc.State
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("instance %s never reached state %q (last: %q)", instanceID, want, last)
	return nil
}

func TestFleetNatsHeartbeatRoundTrip(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	before := time.Now().Add(-time.Second).Unix()
	api.handleFleetEvent("cc.fleet.event", heartbeatLine(id))

	svc := waitForState(t, id, "active")
	if !svc.LastHeartbeat.Valid || svc.LastHeartbeat.Int64 < before {
		t.Fatalf("heartbeat was not recorded: %+v", svc.LastHeartbeat)
	}
}

func TestFleetNatsHeartbeatRevivesStale(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	repo := repository.GetFleetRepository()
	if _, err := repo.Heartbeat(id, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkStale(2000); err != nil {
		t.Fatal(err)
	}
	if svc, _ := repo.GetByInstanceID(id); svc.State != "stale" {
		t.Fatalf("setup failed: state is %q, want stale", svc.State)
	}

	api.handleFleetEvent("cc.fleet.event", heartbeatLine(id))
	waitForState(t, id, "active")
}

func TestFleetNatsIgnoresDeregistered(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	if err := fleet.Get().Registry().Deregister(id); err != nil {
		t.Fatal(err)
	}

	api.handleFleetEvent("cc.fleet.event", heartbeatLine(id))
	waitForCounter(t, &api.unknown, 1)

	svc, err := repository.GetFleetRepository().GetByInstanceID(id)
	if err != nil {
		t.Fatal(err)
	}
	if svc.State != "deregistered" {
		t.Fatalf("a deregistered instance must never be revived over NATS, got state %q", svc.State)
	}
}

func TestFleetNatsUnknownInstanceIsNoOp(t *testing.T) {
	api := setupFleetNats(t)

	api.handleFleetEvent("cc.fleet.event", heartbeatLine("0123456789abcdef0123456789abcdef"))
	waitForCounter(t, &api.unknown, 1)

	rows, err := repository.GetFleetRepository().ListByCluster("fritz")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("an unknown instance id must never create a row, got %d rows", len(rows))
	}
}

func TestFleetNatsUsesServerClock(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	// A hostile or skewed publisher must not be able to park last_heartbeat in
	// the future so the stale sweep never fires.
	future := time.Now().Add(365 * 24 * time.Hour)
	api.handleFleetEvent("cc.fleet.event", heartbeatLineAt(id, future))

	svc := waitForState(t, id, "active")
	if svc.LastHeartbeat.Int64 > time.Now().Add(time.Minute).Unix() {
		t.Fatalf("message timestamp leaked into last_heartbeat: %d", svc.LastHeartbeat.Int64)
	}
}

func TestFleetNatsRejectsNonHeartbeatFunctions(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	payload := fmt.Sprintf(`event="{\"instanceId\":\"%s\"}" %d`, id, time.Now().UnixNano())
	for _, function := range []string{"register", "deregister", "ack_config", "stop_job"} {
		t.Run(function, func(t *testing.T) {
			api.handleFleetEvent("cc.fleet.event",
				[]byte(fmt.Sprintf("fleet,function=%s %s", function, payload)))
		})
	}
	// No function tag at all.
	api.handleFleetEvent("cc.fleet.event", []byte("fleet "+payload))

	waitForCounter(t, &api.rejected, 5)

	svc, err := repository.GetFleetRepository().GetByInstanceID(id)
	if err != nil {
		t.Fatal(err)
	}
	if svc.State != "pending" || svc.LastHeartbeat.Valid {
		t.Fatalf("a rejected message must not mutate the row: %+v", svc)
	}
}

func TestFleetNatsIgnoresWrongMeasurement(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	api.handleFleetEvent("cc.fleet.event",
		[]byte(fmt.Sprintf(`job,function=start_job event="{\"instanceId\":\"%s\"}" %d`, id, time.Now().UnixNano())))

	time.Sleep(2 * fleetFlushInterval)
	svc, err := repository.GetFleetRepository().GetByInstanceID(id)
	if err != nil {
		t.Fatal(err)
	}
	if svc.State != "pending" {
		t.Fatalf("a foreign measurement must be ignored, got state %q", svc.State)
	}
}

func TestFleetNatsMalformedPayloads(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")
	ts := time.Now().UnixNano()

	tests := []struct {
		name string
		data []byte
	}{
		{"empty message", []byte("")},
		{"not line protocol", []byte("this is not line protocol at all")},
		{"non-event message", []byte(fmt.Sprintf("fleet,function=heartbeat value=1 %d", ts))},
		{"empty event", []byte(fmt.Sprintf(`fleet,function=heartbeat event="" %d`, ts))},
		{"empty instance id", []byte(fmt.Sprintf(`fleet,function=heartbeat event="{}" %d`, ts))},
		{"snake_case field", []byte(fmt.Sprintf(`fleet,function=heartbeat event="{\"instance_id\":\"%s\"}" %d`, id, ts))},
		{"extra field", []byte(fmt.Sprintf(`fleet,function=heartbeat event="{\"instanceId\":\"%s\",\"x\":1}" %d`, id, ts))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api.handleFleetEvent("cc.fleet.event", tt.data)
		})
	}

	time.Sleep(2 * fleetFlushInterval)
	svc, err := repository.GetFleetRepository().GetByInstanceID(id)
	if err != nil {
		t.Fatal(err)
	}
	if svc.State != "pending" || svc.LastHeartbeat.Valid {
		t.Fatalf("malformed payloads must not mutate the row: %+v", svc)
	}
}

func TestFleetNatsMultiLinePayload(t *testing.T) {
	api := setupFleetNats(t)
	first := registerForHeartbeat(t, "f0101")
	second := registerForHeartbeat(t, "f0102")

	batch := append(heartbeatLine(first), '\n')
	batch = append(batch, heartbeatLine(second)...)
	api.handleFleetEvent("cc.fleet.event", batch)

	waitForState(t, first, "active")
	waitForState(t, second, "active")
}

func TestFleetNatsCoalescesRepeatedHeartbeats(t *testing.T) {
	api := setupFleetNats(t)
	id := registerForHeartbeat(t, "f0101")

	// Several heartbeats for the same instance inside one flush window collapse
	// into a single row update.
	for range 5 {
		api.handleFleetEvent("cc.fleet.event", heartbeatLine(id))
	}

	waitForState(t, id, "active")
	if got := api.unknown.Load(); got != 0 {
		t.Fatalf("no heartbeat referenced an unknown instance, counter is %d", got)
	}
}

func TestFleetNatsShutdownIsIdempotent(t *testing.T) {
	api := setupFleetNats(t)

	api.Shutdown()
	api.Shutdown()

	// Enqueuing after shutdown must not block.
	done := make(chan struct{})
	go func() {
		api.handleFleetEvent("cc.fleet.event", heartbeatLine("0123456789abcdef0123456789abcdef"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleFleetEvent blocked after Shutdown")
	}
}

func TestFleetNatsStartSubscriptionsWithoutClient(t *testing.T) {
	api := setupFleetNats(t)

	// No NATS client is configured in tests: subscribing must be a no-op rather
	// than an error, so a deployment without NATS still starts.
	if err := api.StartSubscriptions(); err != nil {
		t.Fatalf("StartSubscriptions without a client: %v", err)
	}
}

// waitForCounter blocks until the counter reaches want, or fails.
func waitForCounter(t *testing.T, counter *atomic.Uint64, want uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if counter.Load() >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("counter reached %d, want %d", counter.Load(), want)
}
