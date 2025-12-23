// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/nats"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
)

// NatsAPI provides NATS subscription-based handlers for Job and Node operations.
// It mirrors the functionality of the REST API but uses NATS messaging.
type NatsAPI struct {
	JobRepository *repository.JobRepository
	// RepositoryMutex protects job creation operations from race conditions
	// when checking for duplicate jobs during startJob calls.
	RepositoryMutex sync.Mutex
}

// NewNatsAPI creates a new NatsAPI instance with default dependencies.
func NewNatsAPI() *NatsAPI {
	return &NatsAPI{
		JobRepository: repository.GetJobRepository(),
	}
}

// StartSubscriptions registers all NATS subscriptions for Job and Node APIs.
// Returns an error if the NATS client is not available or subscription fails.
func (api *NatsAPI) StartSubscriptions() error {
	client := nats.GetClient()
	if client == nil {
		cclog.Warn("NATS client not available, skipping API subscriptions")
		return nil
	}

	if config.Keys.APISubjects != nil {

		s := config.Keys.APISubjects

		if err := client.Subscribe(s.SubjectJobStart, api.handleStartJob); err != nil {
			return err
		}

		if err := client.Subscribe(s.SubjectJobStop, api.handleStopJob); err != nil {
			return err
		}

		if err := client.Subscribe(s.SubjectNodeState, api.handleNodeState); err != nil {
			return err
		}

		cclog.Info("NATS API subscriptions started")
	}
	return nil
}

// handleStartJob processes job start messages received via NATS.
// Expected JSON payload follows the schema.Job structure.
func (api *NatsAPI) handleStartJob(subject string, data []byte) {
	req := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		cclog.Errorf("NATS %s: parsing request failed: %v", subject, err)
		return
	}

	cclog.Debugf("NATS %s: %s", subject, req.GoString())
	req.State = schema.JobStateRunning

	if err := importer.SanityChecks(&req); err != nil {
		cclog.Errorf("NATS %s: sanity check failed: %v", subject, err)
		return
	}

	var unlockOnce sync.Once
	api.RepositoryMutex.Lock()
	defer unlockOnce.Do(api.RepositoryMutex.Unlock)

	jobs, err := api.JobRepository.FindAll(&req.JobID, &req.Cluster, nil)
	if err != nil && err != sql.ErrNoRows {
		cclog.Errorf("NATS %s: checking for duplicate failed: %v", subject, err)
		return
	}
	if err == nil {
		for _, job := range jobs {
			if (req.StartTime - job.StartTime) < secondsPerDay {
				cclog.Errorf("NATS %s: job with jobId %d, cluster %s already exists (dbid: %d)",
					subject, req.JobID, req.Cluster, job.ID)
				return
			}
		}
	}

	id, err := api.JobRepository.Start(&req)
	if err != nil {
		cclog.Errorf("NATS %s: insert into database failed: %v", subject, err)
		return
	}
	unlockOnce.Do(api.RepositoryMutex.Unlock)

	for _, tag := range req.Tags {
		if _, err := api.JobRepository.AddTagOrCreate(nil, id, tag.Type, tag.Name, tag.Scope); err != nil {
			cclog.Errorf("NATS %s: adding tag to new job %d failed: %v", subject, id, err)
			return
		}
	}

	cclog.Infof("NATS: new job (id: %d): cluster=%s, jobId=%d, user=%s, startTime=%d",
		id, req.Cluster, req.JobID, req.User, req.StartTime)
}

// handleStopJob processes job stop messages received via NATS.
// Expected JSON payload follows the StopJobAPIRequest structure.
func (api *NatsAPI) handleStopJob(subject string, data []byte) {
	var req StopJobAPIRequest

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		cclog.Errorf("NATS %s: parsing request failed: %v", subject, err)
		return
	}

	if req.JobID == nil {
		cclog.Errorf("NATS %s: the field 'jobId' is required", subject)
		return
	}

	job, err := api.JobRepository.Find(req.JobID, req.Cluster, req.StartTime)
	if err != nil {
		cachedJob, cachedErr := api.JobRepository.FindCached(req.JobID, req.Cluster, req.StartTime)
		if cachedErr != nil {
			cclog.Errorf("NATS %s: finding job failed: %v (cached lookup also failed: %v)",
				subject, err, cachedErr)
			return
		}
		job = cachedJob
	}

	if job.State != schema.JobStateRunning {
		cclog.Errorf("NATS %s: jobId %d (id %d) on %s: job has already been stopped (state is: %s)",
			subject, job.JobID, job.ID, job.Cluster, job.State)
		return
	}

	if job.StartTime > req.StopTime {
		cclog.Errorf("NATS %s: jobId %d (id %d) on %s: stopTime %d must be >= startTime %d",
			subject, job.JobID, job.ID, job.Cluster, req.StopTime, job.StartTime)
		return
	}

	if req.State != "" && !req.State.Valid() {
		cclog.Errorf("NATS %s: jobId %d (id %d) on %s: invalid job state: %#v",
			subject, job.JobID, job.ID, job.Cluster, req.State)
		return
	} else if req.State == "" {
		req.State = schema.JobStateCompleted
	}

	job.Duration = int32(req.StopTime - job.StartTime)
	job.State = req.State
	api.JobRepository.Mutex.Lock()
	defer api.JobRepository.Mutex.Unlock()

	if err := api.JobRepository.Stop(*job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
		if err := api.JobRepository.StopCached(*job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
			cclog.Errorf("NATS %s: jobId %d (id %d) on %s: marking job as '%s' failed: %v",
				subject, job.JobID, job.ID, job.Cluster, job.State, err)
			return
		}
	}

	cclog.Infof("NATS: archiving job (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%d, duration=%d, state=%s",
		job.ID, job.Cluster, job.JobID, job.User, job.StartTime, job.Duration, job.State)

	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return
	}

	archiver.TriggerArchiving(job)
}

// handleNodeState processes node state update messages received via NATS.
// Expected JSON payload follows the UpdateNodeStatesRequest structure.
func (api *NatsAPI) handleNodeState(subject string, data []byte) {
	var req UpdateNodeStatesRequest

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		cclog.Errorf("NATS %s: parsing request failed: %v", subject, err)
		return
	}

	repo := repository.GetNodeRepository()

	for _, node := range req.Nodes {
		state := determineState(node.States)
		nodeState := schema.NodeStateDB{
			TimeStamp:       time.Now().Unix(),
			NodeState:       state,
			CpusAllocated:   node.CpusAllocated,
			MemoryAllocated: node.MemoryAllocated,
			GpusAllocated:   node.GpusAllocated,
			HealthState:     schema.MonitoringStateFull,
			JobsRunning:     node.JobsRunning,
		}

		if err := repo.UpdateNodeState(node.Hostname, req.Cluster, &nodeState); err != nil {
			cclog.Errorf("NATS %s: updating node state for %s on %s failed: %v",
				subject, node.Hostname, req.Cluster, err)
		}
	}

	cclog.Debugf("NATS %s: updated %d node states for cluster %s", subject, len(req.Nodes), req.Cluster)
}
