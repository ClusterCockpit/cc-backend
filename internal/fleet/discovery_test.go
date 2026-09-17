// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-lib/v2/receivers"
	influx "github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

func mkSvc(scope, cluster, stype, host, state, metaJSON string) *repository.ServiceDB {
	meta := sql.NullString{}
	if metaJSON != "" {
		meta = sql.NullString{String: metaJSON, Valid: true}
	}
	return &repository.ServiceDB{
		Scope: scope, Cluster: cluster, ServiceType: stype, Hostname: host,
		State: state, InstanceID: "iid-" + host, MetaData: meta,
	}
}

func findRoster(rosters []roster, subject string) *roster {
	for i := range rosters {
		if rosters[i].subject == subject {
			return &rosters[i]
		}
	}
	return nil
}

func TestBuildRosters(t *testing.T) {
	active := []*repository.ServiceDB{
		mkSvc(ScopeInfra, "", "ccb", "mgmt01", "active", `{"endpoint":"https://mgmt:8080"}`),
		mkSvc(ScopeCluster, "fritz", "ccb", "f-ccb", "active", ""), // a cluster-local backend
		mkSvc(ScopeInfra, "", "ccms", "store01", "active", ""),
		mkSvc(ScopeCluster, "fritz", "ccmc", "f-node01", "active", ""),
		mkSvc(ScopeCluster, "alex", "ccmc", "a-node01", "active", ""),
	}

	rosters := buildRosters(active, DefaultDiscoveryPrefix)

	t.Run("cluster consumer sees own-cluster + infra providers", func(t *testing.T) {
		r := findRoster(rosters, "cc.fleet.discovery.fritz.ccmc")
		if r == nil {
			t.Fatal("missing fritz.ccmc roster")
		}
		// ccmc -> {ccb}: the fritz-local ccb AND the infra ccb, sorted by host.
		if len(r.providers) != 2 {
			t.Fatalf("want 2 providers, got %+v", r.providers)
		}
		if r.providers[0].Hostname != "f-ccb" || r.providers[1].Hostname != "mgmt01" {
			t.Fatalf("unexpected providers/order: %+v", r.providers)
		}
		for _, p := range r.providers {
			if p.Type != ServiceTypeBackend {
				t.Fatalf("only ccb is relevant to ccmc, got %q", p.Type)
			}
		}
	})

	t.Run("cluster filter excludes other clusters", func(t *testing.T) {
		r := findRoster(rosters, "cc.fleet.discovery.alex.ccmc")
		if r == nil {
			t.Fatal("missing alex.ccmc roster")
		}
		// alex sees only the infra ccb, never fritz's f-ccb.
		if len(r.providers) != 1 || r.providers[0].Hostname != "mgmt01" {
			t.Fatalf("cluster filter leaked: %+v", r.providers)
		}
	})

	t.Run("infra bucket exists and carries relevant providers", func(t *testing.T) {
		r := findRoster(rosters, "cc.fleet.discovery.infra.ccms")
		if r == nil {
			t.Fatal("missing infra.ccms roster")
		}
		// ccms -> {ccb}: sees both ccb instances anywhere.
		if len(r.providers) != 2 {
			t.Fatalf("want 2 ccb providers in infra bucket, got %+v", r.providers)
		}
	})

	t.Run("meta is carried through", func(t *testing.T) {
		r := findRoster(rosters, "cc.fleet.discovery.fritz.ccmc")
		var mgmt *ProviderInfo
		for i := range r.providers {
			if r.providers[i].Hostname == "mgmt01" {
				mgmt = &r.providers[i]
			}
		}
		if mgmt == nil || mgmt.Meta["endpoint"] != "https://mgmt:8080" {
			t.Fatalf("endpoint meta not carried: %+v", mgmt)
		}
	})

	t.Run("consumer with no relevant providers is not published", func(t *testing.T) {
		if r := findRoster(rosters, "cc.fleet.discovery.fritz.ccb"); r != nil {
			t.Fatalf("ccb has no relevant providers; should emit no roster, got %+v", r.providers)
		}
	})
}

// capturePub records published messages instead of sending them to a broker.
type capturePub struct{ msgs map[string][]byte }

func (c *capturePub) publish(subject string, data []byte) error {
	c.msgs[subject] = data
	return nil
}

func TestPublishRostersRoundTripNoLeak(t *testing.T) {
	setupDB(t)

	infraReg := NewInfraRegistry()
	reg := NewRegistry(time.Hour)

	// Register + activate a global backend, a metric store, and a cluster collector.
	rb, err := infraReg.Register(InfraRegistrationRequest{
		Hostname: "mgmt01", ServiceType: ServiceTypeBackend,
		MetaData: map[string]string{"endpoint": "https://mgmt:8080"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := infraReg.Register(InfraRegistrationRequest{Hostname: "store01", ServiceType: ServiceTypeMetricStore})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := reg.Register(RegistrationRequest{Cluster: "fritz", Hostname: "f-node01", ServiceType: ServiceTypeCollector})
	if err != nil {
		t.Fatal(err)
	}
	instanceIDs := []string{rb.InstanceID, rs.InstanceID, rc.InstanceID}

	// Only active services are advertised.
	for _, id := range instanceIDs {
		if err := infraReg.Heartbeat(id, time.Unix(1000, 0)); err != nil {
			t.Fatal(err)
		}
	}

	cap := &capturePub{msgs: make(map[string][]byte)}
	pub := NewFleetPublisher(cap.publish, "")
	if err := pub.PublishRosters(); err != nil {
		t.Fatal(err)
	}
	if len(cap.msgs) == 0 {
		t.Fatal("no rosters published")
	}

	// SECURITY: the registration credential must never appear on the wire.
	for subject, data := range cap.msgs {
		for _, id := range instanceIDs {
			if strings.Contains(string(data), id) {
				t.Fatalf("instance_id %q leaked into %q", id, subject)
			}
		}
	}

	// Round-trip: the fritz collector's roster decodes via the same line-protocol
	// path as internal/api/nats.go and contains the backend.
	data, ok := cap.msgs["cc.fleet.discovery.fritz.ccmc"]
	if !ok {
		t.Fatal("missing fritz.ccmc roster")
	}
	providers := decodeRoster(t, data)
	found := false
	for _, p := range providers {
		if p.Type == ServiceTypeBackend && p.Hostname == "mgmt01" {
			found = true
			if p.Meta["endpoint"] != "https://mgmt:8080" {
				t.Errorf("endpoint meta lost in round-trip: %+v", p)
			}
		}
	}
	if !found {
		t.Fatalf("ccb/mgmt01 not in decoded roster: %+v", providers)
	}
}

func decodeRoster(t *testing.T, data []byte) []ProviderInfo {
	t.Helper()
	d := influx.NewDecoderWithBytes(data)
	if !d.Next() {
		t.Fatal("no line-protocol message decoded")
	}
	m, err := receivers.DecodeInfluxMessage(d)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := m.GetEventValue()
	if !ok {
		t.Fatal("message has no event field")
	}
	var providers []ProviderInfo
	if err := json.Unmarshal([]byte(ev), &providers); err != nil {
		t.Fatal(err)
	}
	return providers
}

// countingPub counts publish calls; safe for concurrent use by the publisher
// goroutine and the test.
type countingPub struct {
	mu    sync.Mutex
	calls int
	bump  chan struct{}
}

func newCountingPub() *countingPub {
	return &countingPub{bump: make(chan struct{}, 64)}
}

func (c *countingPub) publish(string, []byte) error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	select {
	case c.bump <- struct{}{}:
	default:
	}
	return nil
}

func (c *countingPub) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// waitForPublish blocks until at least one publish happened since the caller
// drained bump, or the deadline expires.
func (c *countingPub) waitForPublish(t *testing.T, d time.Duration) bool {
	t.Helper()
	select {
	case <-c.bump:
		return true
	case <-time.After(d):
		return false
	}
}

func TestPublisherNotify(t *testing.T) {
	setupDB(t)

	reg := NewRegistry(time.Hour)
	if _, err := reg.Register(RegistrationRequest{
		Cluster: "fritz", Hostname: "f0101", ServiceType: ServiceTypeCollector,
	}); err != nil {
		t.Fatal(err)
	}

	cp := newCountingPub()
	pub := NewFleetPublisher(cp.publish, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A long tick interval isolates Notify: any publish beyond the initial one
	// can only come from the notification path.
	pub.Start(ctx, time.Hour)
	defer pub.Shutdown()

	// Drain the initial publish (Start publishes synchronously before the loop).
	for cp.waitForPublish(t, 100*time.Millisecond) {
	}
	initial := cp.count()
	if initial == 0 {
		t.Fatal("Start must publish once immediately")
	}

	t.Run("rapid notifications collapse into one publish", func(t *testing.T) {
		for range 20 {
			pub.Notify()
		}
		// The initial publish counts towards the rate limit, so this batch is
		// held for minRepublish and then emitted exactly once.
		if !cp.waitForPublish(t, 4*minRepublish) {
			t.Fatal("a debounced publish must eventually happen")
		}
		for cp.waitForPublish(t, minRepublish/2) {
		}
		afterOne := cp.count()

		// One roster subject per (bucket, consumer) pair, so compare in units of
		// the initial publish's message count.
		if afterOne != 2*initial {
			t.Fatalf("expected exactly one extra roster round (%d messages), got %d total", 2*initial, afterOne)
		}
	})

	t.Run("notify after shutdown is a no-op", func(t *testing.T) {
		pub.Shutdown()
		before := cp.count()
		for range 5 {
			pub.Notify()
		}
		time.Sleep(2 * minRepublish)
		if got := cp.count(); got != before {
			t.Fatalf("publisher kept running after Shutdown: %d -> %d", before, got)
		}
	})

	t.Run("notify on a nil publisher does not panic", func(t *testing.T) {
		var nilPub *FleetPublisher
		nilPub.Notify()
	})
}
