// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/fleet"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/nats"
	"github.com/ClusterCockpit/cc-lib/v2/receivers"
	influx "github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

const (
	// fleetMeasurement is the only line protocol measurement accepted on the
	// fleet subject.
	fleetMeasurement = "fleet"

	// fleetFunctionHeartbeat is the only accepted value of the "function" tag.
	// Registration, deregistration and configuration deliberately require an
	// authenticated REST call: the NATS subject has no application-layer auth,
	// so a publisher must not be able to create, resurrect or terminate a
	// service identity.
	fleetFunctionHeartbeat = "heartbeat"

	// fleetQueueGroup makes several cc-backend instances share the heartbeat
	// stream instead of each writing the same row.
	fleetQueueGroup = "cc-backend-fleet"

	// fleetFlushInterval and fleetMaxBatch bound how long a heartbeat waits and
	// how many are coalesced into one write transaction.
	fleetFlushInterval = time.Second
	fleetMaxBatch      = 512

	// fleetReportInterval is how often dropped and unknown heartbeats are
	// summarised. Logging them per message would let anyone with publish rights
	// flood the log.
	fleetReportInterval = time.Minute
)

// FleetHeartbeatRequest is the on-wire heartbeat payload. The instance id is the
// credential the REST registration endpoint issued; nothing else is accepted,
// and in particular no field that could create or modify a registration.
type FleetHeartbeatRequest struct {
	InstanceID string `json:"instanceId" example:"3f1c9a2b7d4e6f8a0b1c2d3e4f5a6b7c"`
}

// fleetHeartbeatMsg is a decoded heartbeat on its way to the flusher.
type fleetHeartbeatMsg struct {
	instanceID string
	at         time.Time
}

// FleetNatsAPI consumes fleet heartbeats from NATS.
//
// It is a separate type from NatsAPI because it has its own enable condition
// (the `fleet` config block), its own shutdown point, and none of the job
// repository state.
//
// # Message format
//
//	fleet,function=heartbeat event="{\"instanceId\":\"3f1c9a2b…\"}" 1734000000000000000
//
// Several heartbeat lines may share one message; that is the supported way for
// an edge aggregator to batch.
//
// The message timestamp is ignored in favour of the server clock: a producer
// supplied timestamp would let a skewed or hostile publisher park last_heartbeat
// in the future so the stale sweep never fires.
//
// Writes are coalesced — decoder workers hand heartbeats to a single flusher
// goroutine which applies at most one transaction per fleetFlushInterval,
// because every individual update is one fsync on a single-writer database.
type FleetNatsAPI struct {
	registry *fleet.Registry

	fleetCh chan natsMessage
	beatCh  chan fleetHeartbeatMsg

	// stop is closed on shutdown; the channels themselves are never closed so
	// an in-flight subscription callback can never send on a closed channel.
	stop     chan struct{}
	stopOnce sync.Once

	dropped  atomic.Uint64
	unknown  atomic.Uint64
	rejected atomic.Uint64
}

// NewFleetNatsAPI creates the heartbeat consumer and starts its worker
// goroutines. It requires fleet.Init to have run.
func NewFleetNatsAPI() *FleetNatsAPI {
	workers := config.DefaultFleetHeartbeatConcurrency
	if config.Keys.Fleet != nil {
		workers = config.Keys.Fleet.HeartbeatWorkers()
	}

	api := &FleetNatsAPI{
		registry: fleet.Get().Registry(),
		fleetCh:  make(chan natsMessage, max(256, 4*workers)),
		beatCh:   make(chan fleetHeartbeatMsg, 1024),
		stop:     make(chan struct{}),
	}

	for range workers {
		go api.decodeWorker()
	}
	go api.flushWorker()

	return api
}

// Shutdown stops the workers and tells subscription callbacks to stop
// enqueueing. Safe to call multiple times. Callers must close the NATS client
// first so no new callbacks are invoked.
func (api *FleetNatsAPI) Shutdown() {
	if api == nil {
		return
	}
	api.stopOnce.Do(func() {
		close(api.stop)
	})
}

// StartSubscriptions subscribes to the configured heartbeat subject. It is a
// no-op when NATS or the subject is not configured.
func (api *FleetNatsAPI) StartSubscriptions() error {
	client := nats.GetClient()
	if client == nil {
		cclog.Warn("NATS client not available, skipping fleet heartbeat subscription")
		return nil
	}

	cfg := config.Keys.Fleet
	if cfg == nil || cfg.HeartbeatSubject == "" {
		return nil
	}

	if err := client.SubscribeQueue(cfg.HeartbeatSubject, fleetQueueGroup, func(subject string, data []byte) {
		select {
		case api.fleetCh <- natsMessage{subject: subject, data: data}:
		case <-api.stop:
		default:
			// Never block the NATS callback: a lost heartbeat is harmless
			// because the next one arrives within seconds, while a blocked
			// callback grows the pending queue and risks slow-consumer drops on
			// every subject sharing the connection.
			api.dropped.Add(1)
		}
	}); err != nil {
		return err
	}

	cclog.Warnf("NATS fleet heartbeat subscription started on subject %q (queue group %q) — this subject is "+
		"UNAUTHENTICATED: anyone with publish rights on the broker who learns a service's instance id can keep "+
		"that instance marked 'active' and thus present in discovery rosters. Registration, deregistration and "+
		"configuration are NOT reachable over NATS — they require an authenticated REST call. Restrict publish "+
		"ACLs on the NATS broker to trusted producers only.",
		cfg.HeartbeatSubject, fleetQueueGroup)

	return nil
}

// decodeWorker parses line protocol messages into heartbeats.
func (api *FleetNatsAPI) decodeWorker() {
	for {
		select {
		case <-api.stop:
			return
		case msg := <-api.fleetCh:
			api.handleFleetEvent(msg.subject, msg.data)
		}
	}
}

// handleFleetEvent decodes one NATS message, which may carry several heartbeat
// lines, and forwards every accepted heartbeat to the flusher.
func (api *FleetNatsAPI) handleFleetEvent(subject string, data []byte) {
	if len(data) == 0 {
		cclog.Warnf("NATS %s: received empty message", subject)
		return
	}

	d := influx.NewDecoderWithBytes(data)
	for d.Next() {
		m, err := receivers.DecodeInfluxMessage(d)
		if err != nil {
			cclog.Errorf("NATS %s: failed to decode message: %v", subject, err)
			return
		}

		if !m.IsEvent() {
			cclog.Debugf("NATS %s: ignoring non-event message: measurement=%s", subject, m.Name())
			continue
		}

		if m.Name() != fleetMeasurement {
			cclog.Debugf("NATS %s: ignoring unexpected measurement %q", subject, m.Name())
			continue
		}

		function, ok := m.GetTag("function")
		if !ok {
			cclog.Warnf("NATS fleet: message is missing required tag 'function'")
			api.rejected.Add(1)
			continue
		}
		if function != fleetFunctionHeartbeat {
			cclog.Warnf("NATS fleet: rejected function %q — only %q is permitted on this subject; "+
				"registration, deregistration and configuration require an authenticated REST call",
				function, fleetFunctionHeartbeat)
			api.rejected.Add(1)
			continue
		}

		payload, ok := m.GetEventValue()
		if !ok {
			cclog.Warnf("NATS fleet: heartbeat is missing the 'event' field")
			api.rejected.Add(1)
			continue
		}

		var req FleetHeartbeatRequest
		dec := json.NewDecoder(strings.NewReader(payload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			cclog.Warnf("NATS fleet: decoding heartbeat payload failed: %v", err)
			api.rejected.Add(1)
			continue
		}
		if req.InstanceID == "" {
			cclog.Warnf("NATS fleet: heartbeat without an instance id")
			api.rejected.Add(1)
			continue
		}

		select {
		case api.beatCh <- fleetHeartbeatMsg{instanceID: req.InstanceID, at: time.Now()}:
		case <-api.stop:
			return
		default:
			api.dropped.Add(1)
		}
	}
}

// flushWorker owns the pending heartbeat map and is the only writer, so the hot
// path needs no locking. It applies a batch every fleetFlushInterval, or as soon
// as fleetMaxBatch distinct instances have accumulated.
func (api *FleetNatsAPI) flushWorker() {
	pending := make(map[string]time.Time, fleetMaxBatch)

	ticker := time.NewTicker(fleetFlushInterval)
	defer ticker.Stop()
	reporter := time.NewTicker(fleetReportInterval)
	defer reporter.Stop()

	for {
		select {
		case <-api.stop:
			api.flush(pending)
			return
		case beat := <-api.beatCh:
			// Last timestamp wins; repeated heartbeats from one instance inside
			// a flush window collapse into a single update.
			pending[beat.instanceID] = beat.at
			if len(pending) >= fleetMaxBatch {
				api.flush(pending)
				pending = make(map[string]time.Time, fleetMaxBatch)
			}
		case <-ticker.C:
			if len(pending) > 0 {
				api.flush(pending)
				pending = make(map[string]time.Time, fleetMaxBatch)
			}
		case <-reporter.C:
			api.report()
		}
	}
}

// flush writes one batch and counts the heartbeats that referenced an unknown or
// deregistered instance.
func (api *FleetNatsAPI) flush(pending map[string]time.Time) {
	if len(pending) == 0 {
		return
	}

	applied, err := api.registry.HeartbeatBatch(pending)
	if err != nil {
		cclog.Errorf("NATS fleet: applying %d heartbeat(s) failed: %v", len(pending), err)
		return
	}

	if missing := int64(len(pending)) - applied; missing > 0 {
		// A no-op by design: an unknown or deregistered instance id is never
		// upserted. Counted rather than logged per message.
		api.unknown.Add(uint64(missing))
	}
}

// report summarises the counters that are deliberately not logged per message.
func (api *FleetNatsAPI) report() {
	if n := api.unknown.Swap(0); n > 0 {
		cclog.Warnf("NATS fleet: %d heartbeat(s) for unknown or deregistered instance ids in the last %s",
			n, fleetReportInterval)
	}
	if n := api.rejected.Swap(0); n > 0 {
		cclog.Warnf("NATS fleet: %d rejected message(s) in the last %s", n, fleetReportInterval)
	}
	if n := api.dropped.Swap(0); n > 0 {
		cclog.Warnf("NATS fleet: dropped %d heartbeat(s) in the last %s because the queue was full",
			n, fleetReportInterval)
	}
}
