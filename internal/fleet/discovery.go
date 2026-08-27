// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	lp "github.com/ClusterCockpit/cc-lib/v2/ccMessage"
)

// infraBucket is the reserved subject token for the cluster-independent bucket;
// cluster-scope buckets use the cluster name.
const infraBucket = "infra"

// DefaultDiscoveryPrefix is the NATS subject prefix under which discovery
// rosters are published. A subscriber listens on
// "<prefix>.<cluster-or-infra>.<own-service-type>".
const DefaultDiscoveryPrefix = "cc.fleet.discovery"

// ProviderInfo is one entry in a discovery roster: enough for a consumer to
// locate a peer, and nothing more.
//
// SECURITY: this is the entire on-wire shape, published over NATS which has no
// application-layer auth (see the package doc and internal/api/nats.go). It
// deliberately carries NO instance_id (the registration credential), NO config
// content, and NO config_revision. Meta is whatever the producer put in its
// registration MetaData — operators MUST NOT register secrets there, since it
// is broadcast to every subscriber.
type ProviderInfo struct {
	Type     ServiceType       `json:"type"`
	Hostname string            `json:"hostname"`
	State    string            `json:"state"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// roster is one published message: the set of providers relevant to a given
// consumer type within a given bucket (a cluster name, or "infra").
type roster struct {
	subject   string
	bucket    string
	consumer  ServiceType
	providers []ProviderInfo
}

// FleetPublisher builds per-consumer discovery rosters from the active-service
// roster and publishes them over NATS. The publish function is injected so the
// component has no hard NATS dependency and is unit-testable without a broker;
// the wiring layer supplies one backed by nats.GetClient().Publish.
//
// Delivery model: server-tailored per-consumer subject. For each bucket and
// each consumer service type, the server publishes the pre-filtered set of
// providers that type should discover (see RelevantProviders). A subscriber
// reads exactly one subject and gets a ready-to-use list.
type FleetPublisher struct {
	repo    *repository.FleetRepository
	publish func(subject string, data []byte) error
	prefix  string

	stop     chan struct{}
	stopOnce sync.Once
}

// NewFleetPublisher returns a publisher that sources the roster from the
// singleton FleetRepository and emits via publish. An empty prefix defaults to
// DefaultDiscoveryPrefix.
func NewFleetPublisher(publish func(subject string, data []byte) error, prefix string) *FleetPublisher {
	if prefix == "" {
		prefix = DefaultDiscoveryPrefix
	}
	return &FleetPublisher{
		repo:    repository.GetFleetRepository(),
		publish: publish,
		prefix:  prefix,
		stop:    make(chan struct{}),
	}
}

// PublishRosters builds and publishes every discovery roster once. Publishing
// is best-effort: an encode/publish failure for one subject is logged and the
// rest still go out; the first error is returned.
func (p *FleetPublisher) PublishRosters() error {
	active, err := p.repo.ListActive()
	if err != nil {
		return err
	}

	var firstErr error
	for _, r := range buildRosters(active, p.prefix) {
		data, encErr := encodeRoster(r)
		if encErr != nil {
			cclog.Errorf("fleet: encoding discovery roster for %q failed: %v", r.subject, encErr)
			if firstErr == nil {
				firstErr = encErr
			}
			continue
		}
		if pubErr := p.publish(r.subject, data); pubErr != nil {
			cclog.Errorf("fleet: publishing discovery roster to %q failed: %v", r.subject, pubErr)
			if firstErr == nil {
				firstErr = pubErr
			}
		}
	}
	return firstErr
}

// Start publishes rosters once immediately, then re-publishes every interval
// until ctx is cancelled or Shutdown is called. Periodic re-publish lets a
// service that subscribes late converge to the current roster (NATS core
// pub/sub is fire-and-forget). Mirrors Registry.StartSweep's lifecycle.
func (p *FleetPublisher) Start(ctx context.Context, interval time.Duration) {
	if err := p.PublishRosters(); err != nil {
		cclog.Errorf("fleet: initial discovery publish failed: %v", err)
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-p.stop:
				return
			case <-ticker.C:
				if err := p.PublishRosters(); err != nil {
					cclog.Errorf("fleet: periodic discovery publish failed: %v", err)
				}
			}
		}
	}()
}

// Shutdown stops the periodic publisher. Safe to call multiple times.
func (p *FleetPublisher) Shutdown() {
	p.stopOnce.Do(func() { close(p.stop) })
}

// buildRosters is the pure core: given the active services, produce one roster
// per (bucket, consumer-type) where the consumer type has any relevant
// providers. Buckets are every cluster present plus the reserved "infra"
// bucket. A cluster-scope consumer on cluster C sees relevant providers in C
// plus all infra-scope providers; the infra bucket sees relevant providers
// everywhere.
func buildRosters(active []*repository.ServiceDB, prefix string) []roster {
	byCluster := make(map[string][]*repository.ServiceDB)
	var infra []*repository.ServiceDB
	for _, s := range active {
		if s.Scope == ScopeInfra {
			infra = append(infra, s)
		} else {
			byCluster[s.Cluster] = append(byCluster[s.Cluster], s)
		}
	}

	buckets := make([]string, 0, len(byCluster)+1)
	for c := range byCluster {
		buckets = append(buckets, c)
	}
	sort.Strings(buckets)
	buckets = append(buckets, infraBucket)

	rosters := make([]roster, 0)
	for _, bucket := range buckets {
		var candidates []*repository.ServiceDB
		if bucket == infraBucket {
			candidates = active // relevant providers everywhere
		} else {
			candidates = append(candidates, byCluster[bucket]...)
			candidates = append(candidates, infra...)
		}

		for _, consumer := range AllServiceTypes {
			rel := RelevantProviders(consumer)
			if len(rel) == 0 {
				continue
			}
			relSet := make(map[ServiceType]struct{}, len(rel))
			for _, t := range rel {
				relSet[t] = struct{}{}
			}

			providers := make([]ProviderInfo, 0)
			for _, s := range candidates {
				if _, ok := relSet[ServiceType(s.ServiceType)]; ok {
					providers = append(providers, toProviderInfo(s))
				}
			}
			sort.Slice(providers, func(i, j int) bool {
				if providers[i].Type != providers[j].Type {
					return providers[i].Type < providers[j].Type
				}
				return providers[i].Hostname < providers[j].Hostname
			})

			rosters = append(rosters, roster{
				subject:   prefix + "." + bucket + "." + string(consumer),
				bucket:    bucket,
				consumer:  consumer,
				providers: providers,
			})
		}
	}
	return rosters
}

func toProviderInfo(s *repository.ServiceDB) ProviderInfo {
	meta, err := unmarshalMeta(s.MetaData)
	if err != nil {
		cclog.Warnf("fleet: ignoring unparseable meta_data for %s/%s: %v", s.Hostname, s.ServiceType, err)
		meta = nil
	}
	return ProviderInfo{
		Type:     ServiceType(s.ServiceType),
		Hostname: s.Hostname,
		State:    s.State,
		Meta:     meta,
	}
}

// encodeRoster serializes a roster as an InfluxDB line-protocol event, matching
// the decode path in internal/api/nats.go: measurement "fleetdiscovery", tags
// cluster/type, and a JSON array of providers in the "event" field.
func encodeRoster(r roster) ([]byte, error) {
	payload, err := json.Marshal(r.providers)
	if err != nil {
		return nil, err
	}
	tags := map[string]string{
		"cluster": r.bucket,
		"type":    string(r.consumer),
	}
	msg, err := lp.NewEvent("fleetdiscovery", tags, nil, string(payload), time.Now())
	if err != nil {
		return nil, err
	}
	return []byte(msg.ToLineProtocol(nil)), nil
}
