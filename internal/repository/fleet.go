// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"database/sql"
	"sync"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/lrucache"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

var (
	fleetRepoOnce     sync.Once
	fleetRepoInstance *FleetRepository
)

type FleetRepository struct {
	DB     *sqlx.DB
	cache  *lrucache.Cache
	driver string
}

func GetFleetRepository() *FleetRepository {
	fleetRepoOnce.Do(func() {
		db := GetConnection()

		fleetRepoInstance = &FleetRepository{
			DB:     db.DB,
			driver: db.Driver,
			cache:  lrucache.New(repoConfig.CacheSize),
		}
	})
	return fleetRepoInstance
}

// ServiceDB is the row shape of the "service" table: one row per registered
// (cluster, hostname, service_type) fleet member.
type ServiceDB struct {
	ID             int64          `db:"id"`
	Cluster        string         `db:"cluster"`
	Hostname       string         `db:"hostname"`
	ServiceType    string         `db:"service_type"`
	InstanceID     string         `db:"instance_id"`
	State          string         `db:"state"`
	RegisteredAt   int64          `db:"registered_at"`
	LastHeartbeat  sql.NullInt64  `db:"last_heartbeat"`
	ConfigRevision int64          `db:"config_revision"`
	MetaData       sql.NullString `db:"meta_data"`
}

const namedServiceInsert string = `
INSERT INTO service (cluster, hostname, service_type, instance_id, state, registered_at, config_revision, meta_data)
	VALUES (:cluster, :hostname, :service_type, :instance_id, 'pending', :registered_at, 0, :meta_data);`

// RegisterService upserts a service by (cluster, hostname, service_type): a
// service registering for the first time is inserted with config_revision 0;
// one re-registering (e.g. after a restart) keeps its existing last_heartbeat
// and config_revision but gets a fresh instance_id and is reset to 'pending'
// until its next heartbeat. Returns the row id.
func (r *FleetRepository) RegisterService(svc *ServiceDB) (int64, error) {
	var id int64
	err := sq.Select("id").From("service").
		Where("cluster = ?", svc.Cluster).
		Where("hostname = ?", svc.Hostname).
		Where("service_type = ?", svc.ServiceType).
		RunWith(r.DB).QueryRow().Scan(&id)

	switch err {
	case nil:
		if _, uerr := sq.Update("service").
			Set("instance_id", svc.InstanceID).
			Set("state", "pending").
			Set("registered_at", svc.RegisteredAt).
			Set("meta_data", svc.MetaData).
			Where("id = ?", id).
			RunWith(r.DB).Exec(); uerr != nil {
			cclog.Errorf("Error while re-registering service '%s/%s/%s': %v", svc.Cluster, svc.Hostname, svc.ServiceType, uerr)
			return 0, uerr
		}
		return id, nil
	case sql.ErrNoRows:
		res, ierr := r.DB.NamedExec(namedServiceInsert, svc)
		if ierr != nil {
			cclog.Errorf("Error while registering service '%s/%s/%s': %v", svc.Cluster, svc.Hostname, svc.ServiceType, ierr)
			return 0, ierr
		}
		return res.LastInsertId()
	default:
		cclog.Errorf("Error while looking up service '%s/%s/%s': %v", svc.Cluster, svc.Hostname, svc.ServiceType, err)
		return 0, err
	}
}

// Heartbeat marks the instance active as of timestamp. It never creates a row:
// the returned row count is 0 for an unknown or deregistered instance_id, so
// callers (e.g. an unauthenticated NATS consumer) can tell a spoofed/expired
// heartbeat from a real one without the repository silently upserting it.
func (r *FleetRepository) Heartbeat(instanceID string, timestamp int64) (int64, error) {
	res, err := sq.Update("service").
		Set("last_heartbeat", timestamp).
		Set("state", "active").
		Where("instance_id = ?", instanceID).
		Where("state <> ?", "deregistered").
		RunWith(r.DB).Exec()
	if err != nil {
		cclog.Errorf("Error while recording heartbeat for instance '%s': %v", instanceID, err)
		return 0, err
	}
	return res.RowsAffected()
}

// MarkStale flips 'active' services with last_heartbeat older than cutoff to
// 'stale'. Intended to be called periodically by a sweep goroutine.
func (r *FleetRepository) MarkStale(cutoff int64) (int64, error) {
	res, err := sq.Update("service").
		Set("state", "stale").
		Where("state = ?", "active").
		Where("last_heartbeat < ?", cutoff).
		RunWith(r.DB).Exec()
	if err != nil {
		cclog.Errorf("Error while marking stale services (cutoff %d): %v", cutoff, err)
		return 0, err
	}
	return res.RowsAffected()
}

func (r *FleetRepository) Deregister(instanceID string) error {
	if _, err := sq.Update("service").
		Set("state", "deregistered").
		Where("instance_id = ?", instanceID).
		RunWith(r.DB).Exec(); err != nil {
		cclog.Errorf("Error while deregistering instance '%s': %v", instanceID, err)
		return err
	}
	return nil
}

// SetConfigRevision records which config revision a service last received.
// Called after the service pulls its config over REST.
func (r *FleetRepository) SetConfigRevision(instanceID string, revision int64) error {
	if _, err := sq.Update("service").
		Set("config_revision", revision).
		Where("instance_id = ?", instanceID).
		RunWith(r.DB).Exec(); err != nil {
		cclog.Errorf("Error while setting config revision for instance '%s': %v", instanceID, err)
		return err
	}
	return nil
}

var serviceColumns = []string{
	"id", "cluster", "hostname", "service_type", "instance_id",
	"state", "registered_at", "last_heartbeat", "config_revision", "meta_data",
}

func scanService(row interface{ Scan(...any) error }) (*ServiceDB, error) {
	svc := &ServiceDB{}
	if err := row.Scan(&svc.ID, &svc.Cluster, &svc.Hostname, &svc.ServiceType, &svc.InstanceID,
		&svc.State, &svc.RegisteredAt, &svc.LastHeartbeat, &svc.ConfigRevision, &svc.MetaData); err != nil {
		return nil, err
	}
	return svc, nil
}

func (r *FleetRepository) GetByID(id int64) (*ServiceDB, error) {
	row := sq.Select(serviceColumns...).From("service").Where("id = ?", id).RunWith(r.DB).QueryRow()
	svc, err := scanService(row)
	if err != nil {
		cclog.Errorf("Error while querying service id '%d': %v", id, err)
		return nil, err
	}
	return svc, nil
}

func (r *FleetRepository) GetByInstanceID(instanceID string) (*ServiceDB, error) {
	row := sq.Select(serviceColumns...).From("service").Where("instance_id = ?", instanceID).RunWith(r.DB).QueryRow()
	svc, err := scanService(row)
	if err != nil {
		cclog.Errorf("Error while querying service instance '%s': %v", instanceID, err)
		return nil, err
	}
	return svc, nil
}

func (r *FleetRepository) ListByCluster(cluster string) ([]*ServiceDB, error) {
	rows, err := sq.Select(serviceColumns...).From("service").
		Where("cluster = ?", cluster).
		OrderBy("hostname ASC", "service_type ASC").
		RunWith(r.DB).Query()
	if err != nil {
		cclog.Errorf("Error while listing services for cluster '%s': %v", cluster, err)
		return nil, err
	}
	defer rows.Close()

	services := make([]*ServiceDB, 0)
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			cclog.Warn("Error while scanning rows (ListByCluster)")
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}
