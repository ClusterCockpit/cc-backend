// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package fleet provides central service discovery and configuration
// deployment for auxiliary cc-* services (e.g. metric collectors, metric
// stores) running across a cluster.
//
// Registration and config pull happen over authenticated REST — that is the
// only path that can create or resurrect a service identity. NATS, which has
// no application-layer auth (see CLAUDE.md), may only be used to carry
// heartbeats for an instance_id that REST already issued; an unknown or
// deregistered instance_id is a no-op, never an upsert.
package fleet

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// ErrUnknownInstance is returned by Heartbeat for an instance_id that was
// never registered (or has been deregistered) — it is never auto-created.
var ErrUnknownInstance = errors.New("fleet: unknown or deregistered instance id")

// Service is the domain-facing view of a fleet registration, as returned to
// REST handlers/GraphQL resolvers.
type Service struct {
	Cluster        string
	Hostname       string
	ServiceType    string
	InstanceID     string
	State          string
	RegisteredAt   time.Time
	LastHeartbeat  *time.Time
	ConfigRevision int64
	MetaData       map[string]string
}

// RegistrationRequest is what a service posts to the REST registration
// endpoint.
type RegistrationRequest struct {
	Cluster     string
	Hostname    string
	ServiceType string
	MetaData    map[string]string
}

// Registration is returned from Register: the instance_id must be presented
// on every subsequent heartbeat and config pull.
type Registration struct {
	InstanceID     string
	ConfigRevision int64
}

// Registry is the business-logic layer on top of FleetRepository: it owns
// instance-identity issuance and the pending/active/stale/deregistered state
// machine. REST handlers call Register/Deregister/List/Get; a NATS heartbeat
// consumer (mirroring the worker-pool shape in internal/api/nats.go) should
// call only Heartbeat.
type Registry struct {
	repo       *repository.FleetRepository
	staleAfter time.Duration
}

// NewRegistry returns a Registry backed by the singleton FleetRepository.
// staleAfter is how long a service may go without a heartbeat before StartSweep
// flips it from 'active' to 'stale'.
func NewRegistry(staleAfter time.Duration) *Registry {
	return &Registry{
		repo:       repository.GetFleetRepository(),
		staleAfter: staleAfter,
	}
}

// Register upserts a service's identity and returns a freshly issued
// instance_id plus the config_revision it currently has on record (0 for a
// never-before-seen service, so the caller knows to pull its initial config).
func (r *Registry) Register(req RegistrationRequest) (*Registration, error) {
	if req.Cluster == "" || req.Hostname == "" || req.ServiceType == "" {
		return nil, errors.New("fleet: cluster, hostname and service_type are required")
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
		Cluster:      req.Cluster,
		Hostname:     req.Hostname,
		ServiceType:  req.ServiceType,
		InstanceID:   instanceID,
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

	cclog.Infof("fleet: registered %s/%s/%s as instance '%s'", req.Cluster, req.Hostname, req.ServiceType, instanceID)
	return &Registration{InstanceID: instanceID, ConfigRevision: stored.ConfigRevision}, nil
}

// Heartbeat refreshes liveness for an already-registered instance. It is a
// no-op for unknown or deregistered instance IDs — see the package doc for
// why that must hold when this is reachable over NATS.
func (r *Registry) Heartbeat(instanceID string, at time.Time) error {
	affected, err := r.repo.Heartbeat(instanceID, at.Unix())
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUnknownInstance
	}
	return nil
}

// Deregister marks an instance as deregistered. Idempotent.
func (r *Registry) Deregister(instanceID string) error {
	return r.repo.Deregister(instanceID)
}

// AckConfig records that instanceID has pulled configRevision. Called by the
// REST config-pull handler after it serves the config payload.
func (r *Registry) AckConfig(instanceID string, configRevision int64) error {
	return r.repo.SetConfigRevision(instanceID, configRevision)
}

// Get returns a single service by instance_id.
func (r *Registry) Get(instanceID string) (*Service, error) {
	svc, err := r.repo.GetByInstanceID(instanceID)
	if err != nil {
		return nil, err
	}
	return toService(svc)
}

// List returns all services registered for a cluster.
func (r *Registry) List(cluster string) ([]*Service, error) {
	rows, err := r.repo.ListByCluster(cluster)
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

// StartSweep runs until ctx is cancelled, periodically marking services whose
// last heartbeat is older than staleAfter as 'stale'. Mirrors the worker
// goroutine lifecycle already used for NATS job/node consumers.
func (r *Registry) StartSweep(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-r.staleAfter).Unix()
				n, err := r.repo.MarkStale(cutoff)
				if err != nil {
					cclog.Errorf("fleet: stale sweep failed: %v", err)
					continue
				}
				if n > 0 {
					cclog.Infof("fleet: marked %d service(s) stale", n)
				}
			}
		}
	}()
}

func generateInstanceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func marshalMeta(meta map[string]string) (sql.NullString, error) {
	if len(meta) == 0 {
		return sql.NullString{}, nil
	}
	buf, err := json.Marshal(meta)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(buf), Valid: true}, nil
}

func unmarshalMeta(raw sql.NullString) (map[string]string, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	meta := make(map[string]string)
	if err := json.Unmarshal([]byte(raw.String), &meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func toService(row *repository.ServiceDB) (*Service, error) {
	meta, err := unmarshalMeta(row.MetaData)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		Cluster:        row.Cluster,
		Hostname:       row.Hostname,
		ServiceType:    row.ServiceType,
		InstanceID:     row.InstanceID,
		State:          row.State,
		RegisteredAt:   time.Unix(row.RegisteredAt, 0),
		ConfigRevision: row.ConfigRevision,
		MetaData:       meta,
	}
	if row.LastHeartbeat.Valid {
		t := time.Unix(row.LastHeartbeat.Int64, 0)
		svc.LastHeartbeat = &t
	}
	return svc, nil
}
