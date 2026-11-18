// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ccnats "github.com/ClusterCockpit/cc-lib/v2/nats"
	natserver "github.com/nats-io/nats-server/v2/server"
)

// delayedNatsClient pauses a real broker callback immediately before the
// receiver handler begins, modelling a callback descheduled during Close.
type delayedNatsClient struct {
	client   *ccnats.Client
	entered  chan struct{}
	release  chan struct{}
	finished chan any
}

func (client delayedNatsClient) Subscribe(subject string, handler ccnats.MessageHandler) error {
	return client.client.Subscribe(subject, func(subject string, data []byte) {
		defer func() { client.finished <- recover() }()
		close(client.entered)
		<-client.release
		handler(subject, data)
	})
}

func TestNatsIngestion(t *testing.T) {
	broker, err := natserver.NewServer(&natserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go broker.Start()
	if !broker.ReadyForConnections(2 * time.Second) {
		t.Fatal("owned loopback NATS broker failed to start")
	}
	previousKeys := Keys
	t.Cleanup(func() { Keys = previousKeys; broker.Shutdown() })
	client, err := ccnats.NewClient(&ccnats.NatsConfig{Address: broker.ClientURL()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	Keys.Checkpoints.FileFormat = "json"
	newStore := func() *MemoryStore {
		return &MemoryStore{Metrics: map[string]MetricConfig{"load": {Frequency: 1, offset: 0}}, root: Level{metrics: make([]*buffer, 1)}}
	}
	waitSubscriptions := func(t *testing.T, minimum int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for client.Connection().NumSubscriptions() < minimum {
			if time.Now().After(deadline) {
				t.Fatal("receiver did not establish its owned subscriptions")
			}
			time.Sleep(time.Millisecond)
		}
		if err := client.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	waitHosts := func(t *testing.T, store *MemoryStore, minimum int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for len(ingestionHostPaths(store)) < minimum {
			if time.Now().After(deadline) {
				t.Fatalf("received %d hosts, want at least %d", len(ingestionHostPaths(store)), minimum)
			}
			time.Sleep(time.Millisecond)
		}
	}
	start := func(t *testing.T, store *MemoryStore, workers int) (context.CancelFunc, <-chan error) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		finished := make(chan error, 1)
		go func() { finished <- receiveNats(store, workers, ctx, client) }()
		t.Cleanup(cancel)
		return cancel, finished
	}

	t.Run("RoutesSubscriptionDefaults", func(t *testing.T) {
		Keys.Subscriptions = &Subscriptions{{SubscribeTo: "audit.routing.alpha", ClusterTag: "alpha"}, {SubscribeTo: "audit.routing.beta", ClusterTag: "beta"}}
		store := newStore()
		minimum := client.Connection().NumSubscriptions() + 2
		cancel, finished := start(t, store, 2)
		defer func() { cancel(); <-finished }()
		waitSubscriptions(t, minimum)
		for i := 0; i < 64; i++ {
			for _, cluster := range []string{"alpha", "beta"} {
				payload := fmt.Appendf(nil, "load,host=%s-%02d value=1 1800000000", cluster, i)
				if err := client.Publish("audit.routing."+cluster, payload); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := client.Publish("audit.routing.alpha", []byte("load,cluster=explicit,host=override value=2 1800000000")); err != nil {
			t.Fatal(err)
		}
		if err := client.Flush(); err != nil {
			t.Fatal(err)
		}
		waitHosts(t, store, 129)
		for _, path := range ingestionHostPaths(store) {
			if len(path) != 2 {
				t.Fatalf("unexpected host selector %v", path)
			}
			if path[1] == "override" {
				if path[0] != "explicit" {
					t.Fatalf("explicit cluster tag lost: %v", path)
				}
				continue
			}
			if path[1][:len(path[0])] != path[0] {
				t.Fatalf("subscription default routed %s into cluster %s", path[1], path[0])
			}
		}
	})

	t.Run("SingleWorkerRemainsUntilCancellation", func(t *testing.T) {
		Keys.Subscriptions = &Subscriptions{{SubscribeTo: "audit.single", ClusterTag: "single"}}
		store := newStore()
		minimum := client.Connection().NumSubscriptions() + 1
		cancel, finished := start(t, store, 1)
		waitSubscriptions(t, minimum)
		select {
		case err := <-finished:
			t.Fatalf("ReceiveNats returned before cancellation: %v", err)
		case <-time.After(25 * time.Millisecond):
		}
		if err := client.Publish("audit.single", []byte("load,host=one value=3 1800000000")); err != nil {
			t.Fatal(err)
		}
		waitHosts(t, store, 1)
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("receiver did not finish after cancellation")
		}
		if err := client.Publish("audit.single", []byte("load,host=after-cancel value=4 1800000000")); err != nil {
			t.Fatal(err)
		}
		if err := client.Flush(); err != nil {
			t.Fatal(err)
		}
		delivered := make(chan struct{})
		if err := client.Connection().Barrier(func() { close(delivered) }); err != nil {
			t.Fatal(err)
		}
		select {
		case <-delivered:
		case <-time.After(time.Second):
			t.Fatal("broker callback did not finish after cancellation")
		}
		if got := len(ingestionHostPaths(store)); got != 1 {
			t.Fatalf("cancelled receiver continued to mutate the store: %d hosts", got)
		}
	})

	t.Run("SubscriptionErrorIsReturned", func(t *testing.T) {
		Keys.Subscriptions = &Subscriptions{{SubscribeTo: "audit.valid", ClusterTag: "valid"}, {SubscribeTo: "", ClusterTag: "invalid"}}
		cancel, finished := start(t, newStore(), 2)
		select {
		case err := <-finished:
			if err == nil {
				t.Fatal("invalid subscription subject reported success")
			}
		case <-time.After(250 * time.Millisecond):
			cancel()
			if err := <-finished; err == nil {
				t.Fatal("invalid subscription error ignored until cancellation")
			}
		}
	})

	t.Run("CloseThenCancelDoesNotRaceAnInFlightCallback", func(t *testing.T) {
		Keys.Subscriptions = &Subscriptions{{SubscribeTo: "audit.lifecycle", ClusterTag: "lifecycle"}}
		var panics atomic.Int32
		// Repetition is needed only for Go's selection between two ready
		// cases. The callback-entry/Close/cancel/late-dispatch ordering itself
		// is deterministic and follows the backend's shutdown order.
		for range 32 {
			connection, err := ccnats.NewClient(&ccnats.NatsConfig{Address: broker.ClientURL()})
			if err != nil {
				t.Fatal(err)
			}
			controlled := delayedNatsClient{connection, make(chan struct{}), make(chan struct{}), make(chan any, 1)}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- receiveNats(newStore(), 2, ctx, controlled) }()
			deadline := time.Now().Add(time.Second)
			for connection.Connection().NumSubscriptions() == 0 {
				if time.Now().After(deadline) {
					t.Fatal("controlled lifecycle subscription not ready")
				}
				time.Sleep(time.Millisecond)
			}
			if err := connection.Flush(); err != nil {
				t.Fatal(err)
			}
			if err := client.Publish("audit.lifecycle", []byte("load,host=late value=1 1800000000")); err != nil {
				t.Fatal(err)
			}
			select {
			case <-controlled.entered:
			case <-time.After(time.Second):
				t.Fatal("real broker did not enter controlled callback")
			}
			connection.Close()
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled receiver did not return")
			}
			close(controlled.release)
			select {
			case recovered := <-controlled.finished:
				if recovered != nil {
					panics.Add(1)
				}
			case <-time.After(time.Second):
				t.Fatal("late callback did not finish")
			}
		}
		if got := panics.Load(); got != 0 {
			t.Fatalf("%d in-flight broker callbacks panicked after Close then cancellation", got)
		}
	})
}

func TestNatsShutdownDrainsAcceptedMetrics(t *testing.T) {
	const isolatedFixture = "CC_AUDIT_NATS_SHUTDOWN_FIXTURE"
	format := os.Getenv(isolatedFixture)
	if format == "" {
		// Production Init uses singleton client/store lifecycles. Each child
		// tests that real lifecycle once without resetting global sync.Once or
		// leaving a closed singleton for other tests and repeated package runs.
		for _, candidate := range []string{"json", "wal"} {
			t.Run(candidate, func(t *testing.T) {
				command := exec.Command(os.Args[0], "-test.run=^TestNatsShutdownDrainsAcceptedMetrics$", "-test.timeout=10s")
				command.Env = append(os.Environ(), isolatedFixture+"="+candidate)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("isolated production %s lifecycle: %v\n%s", candidate, err, output)
				}
			})
		}
		return
	}
	broker, err := natserver.NewServer(&natserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go broker.Start()
	if !broker.ReadyForConnections(2 * time.Second) {
		t.Fatal("owned shutdown broker failed to start")
	}
	t.Cleanup(broker.Shutdown)
	previousKeys, previousStore, previousNatsKeys := Keys, msInstance, ccnats.Keys
	lastCheckpointMu.Lock()
	previousCheckpoint := lastCheckpoint
	lastCheckpointMu.Unlock()
	t.Cleanup(func() {
		Keys, msInstance, ccnats.Keys = previousKeys, previousStore, previousNatsKeys
		lastCheckpointMu.Lock()
		lastCheckpoint = previousCheckpoint
		lastCheckpointMu.Unlock()
	})
	ccnats.Keys = ccnats.NatsConfig{Address: broker.ClientURL()}
	ccnats.Connect()
	client := ccnats.GetClient()
	if client == nil || !client.IsConnected() {
		t.Fatal("owned production-path NATS client failed to connect")
	}
	t.Cleanup(client.Close)
	Keys.NumWorkers = 2
	Keys.RetentionInMemory = "48h"
	Keys.CheckpointInterval = "24h"
	Keys.MemoryCap = 100
	Keys.Checkpoints = Checkpoints{FileFormat: format, RootDir: t.TempDir()}
	Keys.Subscriptions = &Subscriptions{{SubscribeTo: "audit.shutdown", ClusterTag: "shutdown"}}
	var workers sync.WaitGroup
	Init(nil, map[string]MetricConfig{"load": {Frequency: 1}}, nil, &workers)
	store := GetMemoryStore()
	t.Cleanup(func() { Shutdown(); workers.Wait() })
	// Model a running service whose last periodic checkpoint predates this
	// accepted data; do not make the fixture depend on wall-clock rollover.
	lastCheckpointMu.Lock()
	lastCheckpoint = time.Now().Add(-2 * time.Minute)
	lastCheckpointMu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for client.Connection().NumSubscriptions() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("production Init did not establish its owned subscription")
		}
		time.Sleep(time.Millisecond)
	}
	if err := client.Flush(); err != nil {
		t.Fatal(err)
	}
	// Prevent only store mutation while all broker callbacks admit messages
	// into the receiver's queue. This creates a valid bounded input backlog.
	store.root.lock.Lock()
	locked := true
	defer func() {
		if locked {
			store.root.lock.Unlock()
		}
	}()
	const samples = 128
	for i := 0; i < samples; i++ {
		if err := client.Publish("audit.shutdown", fmt.Appendf(nil, "load,host=queued-%03d value=%d %d", i, i+1, time.Now().Add(-time.Minute).Unix())); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Flush(); err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct{})
	if err := client.Connection().Barrier(func() { close(accepted) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("broker callbacks did not admit the bounded backlog")
	}
	// Exactly the production order: close broker client, cancel ingestion
	// through Shutdown, then wait for background workers only afterwards.
	client.Close()
	finished := make(chan struct{})
	go func() { Shutdown(); close(finished) }()
	time.Sleep(25 * time.Millisecond)
	store.root.lock.Unlock()
	locked = false
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("production Shutdown did not finish")
	}
	backgroundDone := make(chan struct{})
	go func() { workers.Wait(); close(backgroundDone) }()
	select {
	case <-backgroundDone:
	case <-time.After(2 * time.Second):
		t.Fatal("production background workers did not finish")
	}
	if got := len(ingestionHostPaths(store)); got != samples {
		t.Fatalf("ingestion fixture lost admitted samples in memory: %d hosts", got)
	}
	restored := &MemoryStore{Metrics: store.Metrics, root: Level{metrics: make([]*buffer, 1)}}
	if _, err := restored.FromCheckpoint(Keys.Checkpoints.RootDir, 0); err != nil {
		t.Fatal(err)
	}
	if got := len(ingestionHostPaths(restored)); got != samples {
		t.Fatalf("final checkpoint lost accepted NATS samples: restored %d hosts, in memory %d", got, samples)
	}
}

// Snapshot actual host levels under the current tree's read locks.
func ingestionHostPaths(store *MemoryStore) [][]string {
	store.root.lock.RLock()
	defer store.root.lock.RUnlock()
	var paths [][]string
	for cluster, level := range store.root.children {
		level.lock.RLock()
		for host := range level.children {
			paths = append(paths, []string{cluster, host})
		}
		level.lock.RUnlock()
	}
	return paths
}
