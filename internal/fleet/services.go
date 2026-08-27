// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"errors"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// Scope values distinguish the two kinds of fleet member persisted in the
// shared "service" table:
//
//   - ScopeCluster: per-node agents, identified by (cluster, hostname,
//     service_type). This is the default and what Registry issues.
//   - ScopeInfra: cluster-independent monitoring infrastructure services
//     (metric stores, collectors, gateways). They are not tied to a single
//     cluster, so their cluster column is empty and identity is
//     (hostname, service_type). InfraRegistry issues these.
const (
	ScopeCluster = "cluster"
	ScopeInfra   = "infra"
)

// InfraRegistrationRequest is what a cluster-independent monitoring
// infrastructure service posts to the REST registration endpoint. Unlike
// RegistrationRequest it has no Cluster field: these services span clusters.
type InfraRegistrationRequest struct {
	Hostname    string
	ServiceType ServiceType
	MetaData    map[string]string
}

// InfraRegistry is the business-logic layer for cluster-independent monitoring
// infrastructure services. It is the ScopeInfra sibling of Registry and shares
// the same FleetRepository and "service" table; identity issuance, the
// pending/active/stale/deregistered state machine and the config-revision
// handshake are identical. The only differences are that registration takes no
// cluster and discovery is by scope rather than by cluster.
//
// Staleness: MarkStale (driven by StartSweep) ages any service by its
// last_heartbeat regardless of scope, so infra rows are already covered by the
// sweep. Do NOT start a second sweep goroutine here — exactly one StartSweep
// across the whole process is sufficient. InfraRegistry therefore deliberately
// exposes no StartSweep.
type InfraRegistry struct {
	repo *repository.FleetRepository
}

// NewInfraRegistry returns an InfraRegistry backed by the singleton
// FleetRepository.
func NewInfraRegistry() *InfraRegistry {
	return &InfraRegistry{
		repo: repository.GetFleetRepository(),
	}
}

// Register upserts a cluster-independent service's identity by
// (hostname, service_type) and returns a freshly issued instance_id plus the
// config_revision currently on record (0 for a never-before-seen service, so
// the caller knows to pull its initial config).
func (r *InfraRegistry) Register(req InfraRegistrationRequest) (*Registration, error) {
	if req.Hostname == "" {
		return nil, errors.New("fleet: hostname is required")
	}
	if !req.ServiceType.Valid() {
		return nil, fmt.Errorf("fleet: unknown service_type %q", req.ServiceType)
	}

	instanceID, err := generateInstanceID()
	if err != nil {
		return nil, err
	}

	metaJSON, err := marshalMeta(req.MetaData)
	if err != nil {
		return nil, err
	}

	svc := &repository.ServiceDB{
		Cluster:      "",
		Hostname:     req.Hostname,
		ServiceType:  string(req.ServiceType),
		InstanceID:   instanceID,
		Scope:        ScopeInfra,
		RegisteredAt: time.Now().Unix(),
		MetaData:     metaJSON,
	}

	id, err := r.repo.RegisterService(svc)
	if err != nil {
		return nil, err
	}

	stored, err := r.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	cclog.Infof("fleet: registered infra %s/%s as instance '%s'", req.Hostname, req.ServiceType, instanceID)
	return &Registration{InstanceID: instanceID, ConfigRevision: stored.ConfigRevision}, nil
}

// Heartbeat refreshes liveness for an already-registered infra instance. It is
// a no-op for unknown or deregistered instance IDs — see the package doc for
// why that must hold when this is reachable over NATS.
func (r *InfraRegistry) Heartbeat(instanceID string, at time.Time) error {
	affected, err := r.repo.Heartbeat(instanceID, at.Unix())
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUnknownInstance
	}
	return nil
}

// Deregister marks an infra instance as deregistered. Idempotent.
func (r *InfraRegistry) Deregister(instanceID string) error {
	return r.repo.Deregister(instanceID)
}

// AckConfig records that instanceID has pulled configRevision. Called by the
// REST config-pull handler after it serves the config payload.
func (r *InfraRegistry) AckConfig(instanceID string, configRevision int64) error {
	return r.repo.SetConfigRevision(instanceID, configRevision)
}

// Get returns a single infra service by instance_id.
func (r *InfraRegistry) Get(instanceID string) (*Service, error) {
	svc, err := r.repo.GetByInstanceID(instanceID)
	if err != nil {
		return nil, err
	}
	return toService(svc)
}

// List returns all registered cluster-independent infrastructure services.
func (r *InfraRegistry) List() ([]*Service, error) {
	rows, err := r.repo.ListByScope(ScopeInfra)
	if err != nil {
		return nil, err
	}

	services := make([]*Service, 0, len(rows))
	for _, row := range rows {
		svc, err := toService(row)
		if err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, nil
}
