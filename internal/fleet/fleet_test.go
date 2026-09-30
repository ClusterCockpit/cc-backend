// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initFleet initializes the subsystem for one test and tears it down again, so
// the sweep goroutine of one test never outlives its database.
func initFleet(t *testing.T, opts Options) {
	t.Helper()
	if err := Init(context.Background(), opts); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Reset)
}

// writeConfigTree materializes a config tree under a fresh temp dir.
func writeConfigTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestInitLifecycle(t *testing.T) {
	setupDB(t)

	if Enabled() {
		t.Fatal("fleet must not be enabled before Init")
	}
	if Get() != nil {
		t.Fatal("Get must be nil before Init")
	}

	initFleet(t, Options{StaleAfter: time.Minute, SweepInterval: time.Minute})

	if !Enabled() {
		t.Fatal("fleet must be enabled after Init")
	}
	m := Get()
	if m == nil || m.Registry() == nil || m.InfraRegistry() == nil {
		t.Fatal("manager must expose both registries")
	}
	if m.Publisher() != nil {
		t.Fatal("no publish func was supplied, so no publisher must run")
	}
	if m.ConfigStore() != nil {
		t.Fatal("no config dir was supplied, so no config store must exist")
	}

	if err := Init(context.Background(), Options{}); err == nil {
		t.Fatal("a second Init must be refused")
	}

	// Shutdown keeps the manager reachable so in-flight requests still work.
	Shutdown()
	if Get() == nil {
		t.Fatal("Shutdown must not clear the manager")
	}
	Shutdown() // idempotent
}

func TestInitStartsPublisherWhenPublishSupplied(t *testing.T) {
	setupDB(t)

	published := make(chan string, 16)
	initFleet(t, Options{
		DiscoveryInterval: time.Hour,
		Publish: func(subject string, _ []byte) error {
			select {
			case published <- subject:
			default:
			}
			return nil
		},
	})

	if Get().Publisher() == nil {
		t.Fatal("publisher must run when a publish func is supplied")
	}
	// Start publishes once immediately, even with an empty roster.
	select {
	case subject := <-published:
		if !strings.HasPrefix(subject, DefaultDiscoveryPrefix+".") {
			t.Fatalf("unexpected subject %q", subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no initial roster was published")
	}
}

func TestInitRejectsMalformedConfigTree(t *testing.T) {
	setupDB(t)

	root := writeConfigTree(t, map[string]string{
		"defaults.json": "{ this is not json",
	})

	err := Init(context.Background(), Options{ConfigDir: root})
	t.Cleanup(Reset)
	if err == nil {
		t.Fatal("Init must fail on a malformed config tree")
	}
	if Enabled() {
		t.Fatal("a failed Init must not leave the subsystem enabled")
	}
}

func TestResolveConfig(t *testing.T) {
	setupDB(t)

	root := writeConfigTree(t, map[string]string{
		"defaults.json":            `{"interval":"10s","shared":true}`,
		"ccms/defaults.json":       `{"port":8081}`,
		"ccms/fritz/defaults.json": `{"interval":"30s"}`,
		"ccms/fritz/f0101.json":    `{"port":9091}`,
		"ccmc/i0101.json":          `{"port":7070}`,
	})
	initFleet(t, Options{ConfigDir: root, StaleAfter: time.Minute, SweepInterval: time.Minute})
	m := Get()

	reg, err := m.Registry().Register(RegistrationRequest{
		Cluster: "fritz", Hostname: "f0101", ServiceType: ServiceTypeMetricStore,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("merges all layers broad to specific", func(t *testing.T) {
		res, err := m.ResolveConfig(reg.InstanceID)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(res.Blob, &got); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"interval": "30s", "shared": true, "port": float64(9091)}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("key %q: got %v, want %v", k, got[k], v)
			}
		}
		if res.Revision == 0 {
			t.Error("revision must be a non-zero content hash")
		}
		if res.AckedRevision != 0 {
			t.Errorf("a fresh registration must have acked revision 0, got %d", res.AckedRevision)
		}
	})

	t.Run("acked revision is reported back", func(t *testing.T) {
		res, err := m.ResolveConfig(reg.InstanceID)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.AckConfig(reg.InstanceID, res.Revision); err != nil {
			t.Fatal(err)
		}
		again, err := m.ResolveConfig(reg.InstanceID)
		if err != nil {
			t.Fatal(err)
		}
		if again.AckedRevision != res.Revision {
			t.Errorf("acked revision: got %d, want %d", again.AckedRevision, res.Revision)
		}
	})

	t.Run("infra scope resolves without a cluster layer", func(t *testing.T) {
		infra, err := m.InfraRegistry().Register(InfraRegistrationRequest{
			Hostname: "i0101", ServiceType: ServiceTypeCollector,
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := m.ResolveConfig(infra.InstanceID)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(res.Blob, &got); err != nil {
			t.Fatal(err)
		}
		if got["port"] != float64(7070) || got["shared"] != true {
			t.Errorf("unexpected merged infra config: %v", got)
		}
	})

	t.Run("unknown instance", func(t *testing.T) {
		if _, err := m.ResolveConfig("0123456789abcdef0123456789abcdef"); !errors.Is(err, ErrUnknownInstance) {
			t.Fatalf("got %v, want ErrUnknownInstance", err)
		}
	})

	t.Run("deregistered instance is unknown", func(t *testing.T) {
		dead, err := m.Registry().Register(RegistrationRequest{
			Cluster: "fritz", Hostname: "f0199", ServiceType: ServiceTypeMetricStore,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Registry().Deregister(dead.InstanceID); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ResolveConfig(dead.InstanceID); !errors.Is(err, ErrUnknownInstance) {
			t.Fatalf("got %v, want ErrUnknownInstance", err)
		}
	})
}

func TestResolveConfigWithoutConfigDir(t *testing.T) {
	setupDB(t)
	initFleet(t, Options{StaleAfter: time.Minute, SweepInterval: time.Minute})
	m := Get()

	reg, err := m.Registry().Register(RegistrationRequest{
		Cluster: "fritz", Hostname: "f0101", ServiceType: ServiceTypeMetricStore,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.ResolveConfig(reg.InstanceID); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("got %v, want ErrNoConfig", err)
	}
}

func TestResolveConfigEmptyTree(t *testing.T) {
	setupDB(t)
	initFleet(t, Options{ConfigDir: t.TempDir(), StaleAfter: time.Minute, SweepInterval: time.Minute})
	m := Get()

	reg, err := m.Registry().Register(RegistrationRequest{
		Cluster: "fritz", Hostname: "f0101", ServiceType: ServiceTypeMetricStore,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.ResolveConfig(reg.InstanceID); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("got %v, want ErrNoConfig", err)
	}
}

func TestInitAfterResetWorks(t *testing.T) {
	setupDB(t)
	initFleet(t, Options{StaleAfter: time.Minute, SweepInterval: time.Minute})
	Reset()
	if Enabled() {
		t.Fatal("Reset must clear the singleton")
	}
	initFleet(t, Options{StaleAfter: time.Minute, SweepInterval: time.Minute})
	if !Enabled() {
		t.Fatal("Init after Reset must work")
	}
}
