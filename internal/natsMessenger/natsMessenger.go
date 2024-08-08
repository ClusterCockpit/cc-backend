// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package natsMessenger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// JobRepository  *repository.JobRepository
// Authentication *auth.Authentication
type NatsMessenger struct {
	Server        *server.Server
	Connection    *nats.Conn
	Subscriptions []*nats.Subscription
}

func New(config *schema.NatsConfig) (nm *NatsMessenger, err error) {
	return SetupNatsMessenger(config)
}

type DevNatsMessage struct {
	Content string `json:"content"`
}

// StartJobNatsMessage model
type StartJobNatsMessage struct {
	schema.BaseJob
	ID         *int64                          `json:"id,omitempty"`
	Statistics map[string]schema.JobStatistics `json:"statistics"`
	StartTime  int64                           `json:"startTime" db:"start_time" example:"1649723812" minimum:"1"`
}

// StopJobNatsMessage model
type StopJobNatsMessage struct {
	JobId     *int64          `json:"jobId" example:"123000"`
	Cluster   *string         `json:"cluster" example:"fritz"`
	StartTime *int64          `json:"startTime" example:"1649723812"`
	State     schema.JobState `json:"jobState" validate:"required" example:"completed"`
	StopTime  int64           `json:"stopTime" validate:"required" example:"1649763839"`
}

// DeleteJobNatsMessage model
type DeleteJobNatsMessage struct {
	JobId     *int64  `json:"jobId" validate:"required" example:"123000"` // Cluster Job ID of job
	Cluster   *string `json:"cluster" example:"fritz"`                    // Cluster of job
	StartTime *int64  `json:"startTime" example:"1649723812"`             // Start Time of job as epoch
}

// jobEventNatsMessage model
type ReceiveEventNatsMessage struct {
}

// Check auth and setup listeners to channels

// ns *server.Server, nc *nats.Conn, subs []*nats.Subscription, err error
func SetupNatsMessenger(config *schema.NatsConfig) (nm *NatsMessenger, err error) {
	// Check if Config present
	if config == nil {
		log.Info("No NATS config found: Skip NATS init.")
		return nil, nil
	}

	// Init Raw
	nmr := NatsMessenger{
		Server:        nil,
		Connection:    nil,
		Subscriptions: []*nats.Subscription{},
	}

	// Start Nats Server
	// Note: You can configure things like Host, Port, Authorization, and much more using server.Options.
	opts := &server.Options{Port: config.Port}
	nmr.Server, err = server.NewServer(opts)

	if err != nil {
		log.Error("nats server error on creation")
		return nil, err
	}

	go nmr.Server.Start()

	if !nmr.Server.ReadyForConnections(3 * time.Second) {
		log.Error("nats server not ready for connection")
		return nil, fmt.Errorf("nats server not ready for connection")
	}

	// Connect
	var copts []nats.Option
	nmr.Connection, err = nats.Connect(nmr.Server.ClientURL(), copts...)
	if nmr.Connection == nil {
		nmr.Server.Shutdown()
		log.Error("nats connection could not be established: nats shut down")
		return nil, err
	}

	// Subscribe
	nmr.Subscriptions, err = setupSubscriptions(nmr.Connection)
	if err != nil {
		log.Error("error when subscribing to channels")
		return nil, err
	}

	log.Infof("NATS server and subscriptions on port '%d' established\n", config.Port)
	return &nmr, nil
}

func (nm *NatsMessenger) StopNatsMessenger() {
	for _, sub := range nm.Subscriptions {
		err := sub.Unsubscribe()
		if err != nil {
			log.Errorf("NATS unsubscribe failed: %s", err.Error())
		}
	}

	nm.Connection.Close()
	nm.Server.Shutdown()
	log.Info("NATS connections closed and server shut down")
}

func setupSubscriptions(conn *nats.Conn) (subs []*nats.Subscription, err error) {

	if startSub, err := startJobListener(conn); err != nil {
		log.Infof("Subscription to 'start-job' failed: %s", err)
	} else {
		log.Info("Subscribed to 'start-job'")
		subs = append(subs, startSub)
	}

	if stopSub, err := stopJobListener(conn); err != nil {
		log.Infof("Subscription to 'stop-job' failed: %s", err)
	} else {
		log.Info("Subscribed to 'stop-job'")
		subs = append(subs, stopSub)
	}

	if deleteSub, err := deleteJobListener(conn); err == nil {
		log.Infof("Subscription to 'delete-job' failed: %s", err)
	} else {
		log.Info("Subscribed to 'delete-job'")
		subs = append(subs, deleteSub)
	}

	if eventSub, err := jobEventListener(conn); err != nil {
		log.Infof("Subscription to 'job-event' failed: %s", err)
	} else {
		log.Info("Subscribed to 'job-event'")
		subs = append(subs, eventSub)
	}

	return subs, err
}

// Listeners: Subscribe to specified channels and handle with specific handler functions

func startJobListener(conn *nats.Conn) (sub *nats.Subscription, err error) {
	return conn.Subscribe("start-job", func(m *nats.Msg) {
		var req DevNatsMessage
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Error("Error while unmarshaling raw json nats message content: startJob")
		}

		if err := startJobHandler(req); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})
}

func stopJobListener(conn *nats.Conn) (sub *nats.Subscription, err error) {
	return conn.Subscribe("stop-job", func(m *nats.Msg) {
		var req DevNatsMessage
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Error("Error while unmarshaling raw json nats message content: stopJob")
		}

		if err := stopJobHandler(req); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})
}

func deleteJobListener(conn *nats.Conn) (sub *nats.Subscription, err error) {
	return conn.Subscribe("delete-job", func(m *nats.Msg) {
		var req DevNatsMessage
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Error("Error while unmarshaling raw json nats message content: deleteJob")
		}

		if err := deleteJobHandler(req); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})
}

func jobEventListener(conn *nats.Conn) (sub *nats.Subscription, err error) {
	return conn.Subscribe("job-event", func(m *nats.Msg) {
		var req DevNatsMessage
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Error("Error while unmarshaling raw json nats message content: jobEvent")
		}

		if err := jobEventHandler(req); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})
}

// Handlers: Take content of message and perform action, e.g. adding job in db

func startJobHandler(req DevNatsMessage) (err error) {
	log.Debugf("CALLED HANDLER FOR startJob: %s", req.Content)
	return nil
}

func stopJobHandler(req DevNatsMessage) (err error) {
	log.Debugf("CALLED HANDLER FOR stopJob: %s", req.Content)
	return nil
}

func deleteJobHandler(req DevNatsMessage) (err error) {
	log.Debugf("CALLED HANDLER FOR deleteJob: %s", req.Content)
	return nil
}

func jobEventHandler(req DevNatsMessage) (err error) {
	log.Debugf("CALLED HANDLER FOR jobEvent: %s", req.Content)
	return nil
}
