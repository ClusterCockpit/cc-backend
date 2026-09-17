// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// Service states, matching the CHECK constraint on the service table. A
// registration starts 'pending', becomes 'active' on its first heartbeat, ages
// to 'stale' without one, and 'deregistered' is terminal until re-registration.
const (
	StatePending      = "pending"
	StateActive       = "active"
	StateStale        = "stale"
	StateDeregistered = "deregistered"
)

// Default lifecycle timings, applied for any zero value in Options.
const (
	DefaultStaleAfter        = 90 * time.Second
	DefaultSweepInterval     = 30 * time.Second
	DefaultReloadInterval    = 60 * time.Second
	DefaultDiscoveryInterval = 60 * time.Second
)

// Options configures the fleet subsystem. The cmd layer maps the `main.fleet`
// config block into this, which keeps internal/fleet free of a dependency on
// internal/config (same arrangement as logviewer.Options).
type Options struct {
	// ConfigDir is the root of the hierarchical configuration tree. An empty
	// value disables config deployment: every pull reports ErrNoConfig.
	ConfigDir string

	// StaleAfter is how long an instance may go without a heartbeat before the
	// sweep flips it from 'active' to 'stale'.
	StaleAfter time.Duration

	// SweepInterval is how often that sweep runs.
	SweepInterval time.Duration

	// ReloadInterval is how often the configuration tree is re-scanned.
	ReloadInterval time.Duration

	// DiscoveryPrefix is the NATS subject prefix for discovery rosters.
	DiscoveryPrefix string

	// DiscoveryInterval is how often all rosters are re-published.
	DiscoveryInterval time.Duration

	// Publish is the NATS seam for the discovery publisher, supplied by the
	// wiring layer (nats.GetClient().Publish). A nil value starts no publisher,
	// which is the correct behaviour when NATS is not configured.
	Publish func(subject string, data []byte) error
}

// Manager owns the process-wide fleet components and their goroutines. It is
// created once by Init and reached through Get.
type Manager struct {
	repo      *repository.FleetRepository
	registry  *Registry
	infra     *InfraRegistry
	configs   *ConfigStore    // nil when no config tree is configured
	publisher *FleetPublisher // nil when no publish func was supplied
	cancel    context.CancelFunc
}

var manager atomic.Pointer[Manager]

// ConfigResult is what a config pull yields: the merged blob, its content-hash
// revision, and the revision the instance is already on record for. The latter
// lets the REST handler skip the acknowledgement write when nothing changed.
type ConfigResult struct {
	Blob          json.RawMessage
	Revision      int64
	AckedRevision int64
}

// Init builds the fleet components, starts the stale sweep, the configuration
// reloader and — when a publish function was supplied — the discovery
// publisher. It requires repository.Connect to have run. All goroutines stop
// when ctx is cancelled or Shutdown is called.
//
// Init is called once per process; a second call is an error rather than a
// silent second set of goroutines.
func Init(ctx context.Context, opts Options) error {
	if manager.Load() != nil {
		return errors.New("fleet: already initialized")
	}

	if opts.StaleAfter <= 0 {
		opts.StaleAfter = DefaultStaleAfter
	}
	if opts.SweepInterval <= 0 {
		opts.SweepInterval = DefaultSweepInterval
	}
	if opts.ReloadInterval <= 0 {
		opts.ReloadInterval = DefaultReloadInterval
	}
	if opts.DiscoveryInterval <= 0 {
		opts.DiscoveryInterval = DefaultDiscoveryInterval
	}
	if opts.DiscoveryPrefix == "" {
		opts.DiscoveryPrefix = DefaultDiscoveryPrefix
	}

	ctx, cancel := context.WithCancel(ctx)

	m := &Manager{
		repo:     repository.GetFleetRepository(),
		registry: NewRegistry(opts.StaleAfter),
		infra:    NewInfraRegistry(),
		cancel:   cancel,
	}

	// Exactly one sweep for the whole process: MarkStale is scope-agnostic, so
	// InfraRegistry deliberately has no sweep of its own.
	m.registry.StartSweep(ctx, opts.SweepInterval)

	if opts.ConfigDir != "" {
		m.configs = NewConfigStore(opts.ConfigDir)
		// Load once synchronously so a malformed tree fails startup instead of
		// silently serving nothing to every member. The periodic reload started
		// below is best-effort by design and keeps the last good generation.
		if err := m.configs.Reload(); err != nil {
			cancel()
			return fmt.Errorf("fleet: loading config tree %q: %w", opts.ConfigDir, err)
		}
		m.configs.Start(ctx, opts.ReloadInterval)
	}

	if opts.Publish != nil {
		m.publisher = NewFleetPublisher(opts.Publish, opts.DiscoveryPrefix)
		m.publisher.Start(ctx, opts.DiscoveryInterval)
		cclog.Warnf("fleet: publishing discovery rosters under %q — these are broadcast UNAUTHENTICATED "+
			"and contain hostnames, service types and registration meta data. Do not put secrets in a "+
			"service's meta_data.", opts.DiscoveryPrefix)
	}

	manager.Store(m)
	cclog.Infof("fleet: initialized (config-dir '%s', stale-after %s, sweep %s, reload %s, discovery %s)",
		opts.ConfigDir, opts.StaleAfter, opts.SweepInterval, opts.ReloadInterval, opts.DiscoveryInterval)
	return nil
}

// Get returns the fleet manager, or nil when Init was never called.
func Get() *Manager { return manager.Load() }

// Enabled reports whether the fleet subsystem was initialized.
func Enabled() bool { return manager.Load() != nil }

// Shutdown stops the sweep, the configuration reloader and the discovery
// publisher. It deliberately leaves Get non-nil so in-flight REST requests keep
// working until the HTTP server has drained. Safe to call multiple times and
// when Init was never called.
func Shutdown() {
	m := manager.Load()
	if m == nil {
		return
	}
	if m.publisher != nil {
		m.publisher.Shutdown()
	}
	m.cancel()
}

// Reset stops the subsystem and clears the singleton. Intended for tests, which
// need to Init again against a fresh database.
func Reset() {
	Shutdown()
	manager.Store(nil)
}

// Registry returns the cluster-scope registry.
func (m *Manager) Registry() *Registry { return m.registry }

// InfraRegistry returns the infra-scope registry.
func (m *Manager) InfraRegistry() *InfraRegistry { return m.infra }

// Publisher returns the discovery publisher, or nil when none is running.
// FleetPublisher.Notify tolerates a nil receiver, so callers need no guard.
func (m *Manager) Publisher() *FleetPublisher { return m.publisher }

// ConfigStore returns the configuration store, or nil when none is configured.
func (m *Manager) ConfigStore() *ConfigStore { return m.configs }

// ResolveConfig merges the configuration layers that apply to instanceID.
//
// It exists here rather than in the REST layer because Resolve needs the
// registration's scope, which Service deliberately does not carry: the manager
// is in-package and can read the stored row directly, so handlers never reach
// past the domain package. Callers only ever see ErrUnknownInstance and
// ErrNoConfig.
func (m *Manager) ResolveConfig(instanceID string) (ConfigResult, error) {
	row, err := m.repo.GetByInstanceID(instanceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ConfigResult{}, ErrUnknownInstance
		}
		return ConfigResult{}, err
	}
	// A deregistered identity is gone for good; only a fresh REST registration
	// brings it back (see the package doc).
	if row.State == StateDeregistered {
		return ConfigResult{}, ErrUnknownInstance
	}

	if m.configs == nil {
		return ConfigResult{}, ErrNoConfig
	}

	blob, revision, err := m.configs.Resolve(row.Scope, row.Cluster, row.ServiceType, row.Hostname)
	if err != nil {
		return ConfigResult{}, err
	}

	return ConfigResult{Blob: blob, Revision: revision, AckedRevision: row.ConfigRevision}, nil
}

// AckConfig records that instanceID has received revision.
func (m *Manager) AckConfig(instanceID string, revision int64) error {
	return m.registry.AckConfig(instanceID, revision)
}
