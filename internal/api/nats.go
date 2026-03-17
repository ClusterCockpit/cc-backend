// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"database/sql"
	"encoding/json"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	lp "github.com/ClusterCockpit/cc-lib/v2/ccMessage"
	"github.com/ClusterCockpit/cc-lib/v2/nats"
	"github.com/ClusterCockpit/cc-lib/v2/receivers"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	influx "github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

// natsMessage wraps a raw NATS message with its subject for channel-based processing.
type natsMessage struct {
	subject string
	data    []byte
}

// NatsAPI provides NATS subscription-based handlers for Job and Node operations.
// It mirrors the functionality of the REST API but uses NATS messaging with
// InfluxDB line protocol as the message format.
//
// # Message Format
//
// All NATS messages use InfluxDB line protocol format (https://docs.influxdata.com/influxdb/v2.0/reference/syntax/line-protocol/)
// with the following structure:
//
//	measurement,tag1=value1,tag2=value2 field1=value1,field2=value2 timestamp
//
// # Job Events
//
// Job start/stop events use the "job" measurement with a "function" tag to distinguish operations:
//
//	job,function=start_job event="{...JSON payload...}" <timestamp>
//	job,function=stop_job event="{...JSON payload...}" <timestamp>
//
// The JSON payload in the "event" field follows the schema.Job or StopJobAPIRequest structure.
//
// Example job start message:
//
//	job,function=start_job event="{\"jobId\":1001,\"user\":\"testuser\",\"cluster\":\"testcluster\",...}" 1234567890000000000
//
// # Node State Events
//
// Node state updates use the "nodestate" measurement with cluster information:
//
//	nodestate event="{...JSON payload...}" <timestamp>
//
// The JSON payload follows the UpdateNodeStatesRequest structure.
//
// Example node state message:
//
//	nodestate event="{\"cluster\":\"testcluster\",\"nodes\":[{\"hostname\":\"node01\",\"states\":[\"idle\"]}]}" 1234567890000000000
type NatsAPI struct {
	JobRepository *repository.JobRepository
	// RepositoryMutex protects job creation operations from race conditions
	// when checking for duplicate jobs during startJob calls.
	RepositoryMutex sync.Mutex
	// jobCh receives job event messages for processing by worker goroutines.
	jobCh chan natsMessage
	// nodeCh receives node state messages for processing by worker goroutines.
	nodeCh chan natsMessage
}

// NewNatsAPI creates a new NatsAPI instance with channel-based worker pools.
// Concurrency is configured via NATSConfig (defaults: JobConcurrency=8, NodeConcurrency=2).
func NewNatsAPI() *NatsAPI {
	jobConc := 8
	nodeConc := 2

	if s := config.Keys.APISubjects; s != nil {
		if s.JobConcurrency > 0 {
			jobConc = s.JobConcurrency
		}
		if s.NodeConcurrency > 0 {
			nodeConc = s.NodeConcurrency
		}
	}

	api := &NatsAPI{
		JobRepository: repository.GetJobRepository(),
		jobCh:         make(chan natsMessage, jobConc),
		nodeCh:        make(chan natsMessage, nodeConc),
	}

	// Start worker goroutines
	for range jobConc {
		go api.jobWorker()
	}
	for range nodeConc {
		go api.nodeWorker()
	}

	return api
}

// jobWorker processes job event messages from the job channel.
func (api *NatsAPI) jobWorker() {
	for msg := range api.jobCh {
		api.handleJobEvent(msg.subject, msg.data)
	}
}

// nodeWorker processes node state messages from the node channel.
func (api *NatsAPI) nodeWorker() {
	for msg := range api.nodeCh {
		api.handleNodeState(msg.subject, msg.data)
	}
}

// StartSubscriptions registers all NATS subscriptions for Job and Node APIs.
// Messages are delivered to buffered channels and processed by worker goroutines.
// Returns an error if the NATS client is not available or subscription fails.
func (api *NatsAPI) StartSubscriptions() error {
	client := nats.GetClient()
	if client == nil {
		cclog.Warn("NATS client not available, skipping API subscriptions")
		return nil
	}

	if config.Keys.APISubjects != nil {
		s := config.Keys.APISubjects

		if err := client.Subscribe(s.SubjectJobEvent, func(subject string, data []byte) {
			api.jobCh <- natsMessage{subject: subject, data: data}
		}); err != nil {
			return err
		}

		if err := client.Subscribe(s.SubjectNodeState, func(subject string, data []byte) {
			api.nodeCh <- natsMessage{subject: subject, data: data}
		}); err != nil {
			return err
		}

		cclog.Info("NATS API subscriptions started")
	}
	return nil
}

// processJobEvent routes job event messages to the appropriate handler based on the "function" tag.
// Validates that required tags and fields are present before processing.
func (api *NatsAPI) processJobEvent(msg lp.CCMessage) {
	function, ok := msg.GetTag("function")
	if !ok {
		cclog.Errorf("Job event is missing required tag 'function': measurement=%s", msg.Name())
		return
	}

	switch function {
	case "start_job":
		v, ok := msg.GetEventValue()
		if !ok {
			cclog.Errorf("Job start event is missing event field with JSON payload")
			return
		}
		api.handleStartJob(v)

	case "stop_job":
		v, ok := msg.GetEventValue()
		if !ok {
			cclog.Errorf("Job stop event is missing event field with JSON payload")
			return
		}
		api.handleStopJob(v)

	default:
		cclog.Warnf("Unknown job event function '%s', expected 'start_job' or 'stop_job'", function)
	}
}

// handleJobEvent processes job-related messages received via NATS using InfluxDB line protocol.
// The message must be in line protocol format with measurement="job" and include:
//   - tag "function" with value "start_job" or "stop_job"
//   - field "event" containing JSON payload (schema.Job or StopJobAPIRequest)
//
// Example: job,function=start_job event="{\"jobId\":1001,...}" 1234567890000000000
func (api *NatsAPI) handleJobEvent(subject string, data []byte) {
	if len(data) == 0 {
		cclog.Warnf("NATS %s: received empty message", subject)
		return
	}

	d := influx.NewDecoderWithBytes(data)

	for d.Next() {
		m, err := receivers.DecodeInfluxMessage(d)
		if err != nil {
			cclog.Errorf("NATS %s: failed to decode InfluxDB line protocol message: %v", subject, err)
			return
		}

		if !m.IsEvent() {
			cclog.Debugf("NATS %s: received non-event message, skipping", subject)
			continue
		}

		if m.Name() == "job" {
			api.processJobEvent(m)
		} else {
			cclog.Debugf("NATS %s: unexpected measurement name '%s', expected 'job'", subject, m.Name())
		}
	}
}

// handleStartJob processes job start messages received via NATS.
// The payload parameter contains JSON following the schema.Job structure.
// Jobs are validated, checked for duplicates, and inserted into the database.
func (api *NatsAPI) handleStartJob(payload string) {
	if payload == "" {
		cclog.Error("NATS start job: payload is empty")
		return
	}
	req := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}

	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		cclog.Errorf("NATS start job: parsing request failed: %v", err)
		return
	}

	cclog.Debugf("NATS start job: %s", req.GoString())
	req.State = schema.JobStateRunning

	if err := importer.SanityChecks(&req); err != nil {
		cclog.Errorf("NATS start job: sanity check failed: %v", err)
		return
	}

	var unlockOnce sync.Once
	api.RepositoryMutex.Lock()
	defer unlockOnce.Do(api.RepositoryMutex.Unlock)

	jobs, err := api.JobRepository.FindAll(&req.JobID, &req.Cluster, nil)
	if err != nil && err != sql.ErrNoRows {
		cclog.Errorf("NATS start job: checking for duplicate failed: %v", err)
		return
	}
	if err == nil {
		for _, job := range jobs {
			if (req.StartTime - job.StartTime) < secondsPerDay {
				cclog.Errorf("NATS start job: job with jobId %d, cluster %s already exists (dbid: %d)",
					req.JobID, req.Cluster, job.ID)
				return
			}
		}
	}

	// When tags are present, insert directly into the job table so that the
	// returned ID can be used with AddTagOrCreate (which queries the job table).
	var id int64
	if len(req.Tags) > 0 {
		id, err = api.JobRepository.StartDirect(&req)
	} else {
		id, err = api.JobRepository.Start(&req)
	}
	if err != nil {
		cclog.Errorf("NATS start job: insert into database failed: %v", err)
		return
	}
	unlockOnce.Do(api.RepositoryMutex.Unlock)

	for _, tag := range req.Tags {
		if _, err := api.JobRepository.AddTagOrCreate(nil, id, tag.Type, tag.Name, tag.Scope); err != nil {
			cclog.Errorf("NATS start job: adding tag to new job %d failed: %v", id, err)
			return
		}
	}

	cclog.Infof("NATS: new job (id: %d): cluster=%s, jobId=%d, user=%s, startTime=%d",
		id, req.Cluster, req.JobID, req.User, req.StartTime)
}

// handleStopJob processes job stop messages received via NATS.
// The payload parameter contains JSON following the StopJobAPIRequest structure.
// The job is marked as stopped in the database and archiving is triggered if monitoring is enabled.
func (api *NatsAPI) handleStopJob(payload string) {
	if payload == "" {
		cclog.Error("NATS stop job: payload is empty")
		return
	}
	var req StopJobAPIRequest

	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		cclog.Errorf("NATS job stop: parsing request failed: %v", err)
		return
	}

	if req.JobID == nil {
		cclog.Errorf("NATS job stop: the field 'jobId' is required")
		return
	}

	isCached := false
	job, err := api.JobRepository.FindCached(req.JobID, req.Cluster, req.StartTime)
	if err != nil {
		// Not in cache, try main job table
		job, err = api.JobRepository.Find(req.JobID, req.Cluster, req.StartTime)
		if err != nil {
			cclog.Errorf("NATS job stop: finding job failed: %v", err)
			return
		}
	} else {
		isCached = true
	}

	if job.State != schema.JobStateRunning {
		cclog.Errorf("NATS job stop: jobId %d (id %d) on %s: job has already been stopped (state is: %s)",
			job.JobID, job.ID, job.Cluster, job.State)
		return
	}

	if job.StartTime > req.StopTime {
		cclog.Errorf("NATS job stop: jobId %d (id %d) on %s: stopTime %d must be >= startTime %d",
			job.JobID, job.ID, job.Cluster, req.StopTime, job.StartTime)
		return
	}

	if req.State != "" && !req.State.Valid() {
		cclog.Errorf("NATS job stop: jobId %d (id %d) on %s: invalid job state: %#v",
			job.JobID, job.ID, job.Cluster, req.State)
		return
	} else if req.State == "" {
		req.State = schema.JobStateCompleted
	}

	job.Duration = int32(req.StopTime - job.StartTime)
	job.State = req.State
	api.JobRepository.Mutex.Lock()
	defer api.JobRepository.Mutex.Unlock()

	// If the job is still in job_cache, transfer it to the job table first
	if isCached {
		newID, err := api.JobRepository.TransferCachedJobToMain(*job.ID)
		if err != nil {
			cclog.Errorf("NATS job stop: jobId %d (id %d) on %s: transferring cached job failed: %v",
				job.JobID, *job.ID, job.Cluster, err)
			return
		}
		cclog.Infof("NATS: transferred cached job to main table: old id %d -> new id %d (jobId=%d)", *job.ID, newID, job.JobID)
		job.ID = &newID
	}

	if err := api.JobRepository.Stop(*job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
		cclog.Errorf("NATS job stop: jobId %d (id %d) on %s: marking job as '%s' failed: %v",
			job.JobID, *job.ID, job.Cluster, job.State, err)
		return
	}

	cclog.Infof("NATS: archiving job (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%d, duration=%d, state=%s",
		*job.ID, job.Cluster, job.JobID, job.User, job.StartTime, job.Duration, job.State)

	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return
	}

	archiver.TriggerArchiving(job)
}

// processNodestateEvent extracts and processes node state data from the InfluxDB message.
// Updates node states in the repository for all nodes in the payload.
func (api *NatsAPI) processNodestateEvent(msg lp.CCMessage) {
	v, ok := msg.GetEventValue()
	if !ok {
		cclog.Errorf("Nodestate event is missing event field with JSON payload")
		return
	}

	var req UpdateNodeStatesRequest

	dec := json.NewDecoder(strings.NewReader(v))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		cclog.Errorf("NATS nodestate: parsing request failed: %v", err)
		return
	}

	repo := repository.GetNodeRepository()
	requestReceived := time.Now().Unix()

	// Build nodeList per subcluster for health check
	m := make(map[string][]string)
	metricNames := make(map[string][]string)
	healthResults := make(map[string]metricstore.HealthCheckResult)

	for _, node := range req.Nodes {
		if sc, err := archive.GetSubClusterByNode(req.Cluster, node.Hostname); err == nil {
			m[sc] = append(m[sc], node.Hostname)
		}
	}

	for sc := range m {
		if sc != "" {
			metricList := archive.GetMetricConfigSubCluster(req.Cluster, sc)
			metricNames[sc] = metricListToNames(metricList)
		}
	}

	// Perform health check against metric store
	healthRepo, err := metricdispatch.GetHealthCheckRepo(req.Cluster)
	if err != nil {
		cclog.Warnf("NATS nodestate: no metric store for cluster %s, skipping health check: %v", req.Cluster, err)
	} else {
		for sc, nl := range m {
			if sc != "" {
				if results, err := healthRepo.HealthCheck(req.Cluster, nl, metricNames[sc]); err == nil {
					maps.Copy(healthResults, results)
				}
			}
		}
	}

	updates := make([]repository.NodeStateUpdate, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		state := determineState(node.States)
		healthState := schema.MonitoringStateFailed
		var healthMetrics string
		if result, ok := healthResults[node.Hostname]; ok {
			healthState = result.State
			healthMetrics = result.HealthMetrics
		}
		nodeState := schema.NodeStateDB{
			TimeStamp:       requestReceived,
			NodeState:       state,
			CpusAllocated:   node.CpusAllocated,
			MemoryAllocated: node.MemoryAllocated,
			GpusAllocated:   node.GpusAllocated,
			HealthState:     healthState,
			HealthMetrics:   healthMetrics,
			JobsRunning:     node.JobsRunning,
		}
		updates = append(updates, repository.NodeStateUpdate{
			Hostname:  node.Hostname,
			Cluster:   req.Cluster,
			NodeState: &nodeState,
		})
	}

	if err := repo.BatchUpdateNodeStates(updates); err != nil {
		cclog.Errorf("NATS nodestate: batch update for cluster %s failed: %v", req.Cluster, err)
	}

	cclog.Debugf("NATS nodestate: updated %d node states for cluster %s", len(req.Nodes), req.Cluster)
}

// handleNodeState processes node state update messages received via NATS using InfluxDB line protocol.
// The message must be in line protocol format with measurement="nodestate" and include:
//   - field "event" containing JSON payload (UpdateNodeStatesRequest)
//
// Example: nodestate event="{\"cluster\":\"testcluster\",\"nodes\":[...]}" 1234567890000000000
func (api *NatsAPI) handleNodeState(subject string, data []byte) {
	if len(data) == 0 {
		cclog.Warnf("NATS %s: received empty message", subject)
		return
	}

	d := influx.NewDecoderWithBytes(data)

	for d.Next() {
		m, err := receivers.DecodeInfluxMessage(d)
		if err != nil {
			cclog.Errorf("NATS %s: failed to decode InfluxDB line protocol message: %v", subject, err)
			return
		}

		if !m.IsEvent() {
			cclog.Warnf("NATS %s: received non-event message, skipping", subject)
			continue
		}

		if m.Name() == "nodestate" {
			api.processNodestateEvent(m)
		} else {
			cclog.Warnf("NATS %s: unexpected measurement name '%s', expected 'nodestate'", subject, m.Name())
		}
	}
}
