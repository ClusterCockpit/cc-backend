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

// StartJobNatsMessage model
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

	if !nmr.Server.ReadyForConnections(4 * time.Second) {
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
	sub, err := startJobListener(nmr.Connection)
	if err != nil {
		log.Error("startJobListener subscription error")
		return nil, err
	} else {
		log.Infof("NATS subscription to 'start-job' on port '%d' established\n", config.Port)
		nmr.Subscriptions = append(nmr.Subscriptions, sub)
	}

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

// Listeners: Subscribe to specified channels for actions

func startJobListener(conn *nats.Conn) (sub *nats.Subscription, err error) {

	sub, err = conn.Subscribe("start-job", func(m *nats.Msg) {
		var job DevNatsMessage
		if err := json.Unmarshal(m.Data, &job); err != nil {
			log.Error("Error while unmarshaling raw json nats message content")
		}

		if err := startJobHandler(job); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})

	if err != nil {
		return nil, err
	} else {
		return sub, nil
	}
}

func (nm *NatsMessenger) stopJobListener(conn *nats.Conn) {
}

func (nm *NatsMessenger) deleteJobListener(conn *nats.Conn) {
}

func (nm *NatsMessenger) jobEventListener(conn *nats.Conn) {
}

// Handlers: Take content of message and perform action, e.g. adding job in db

func startJobHandler(job DevNatsMessage) (err error) {
	log.Debugf("CALLED HANDLER FOR startJob: %s", job.Content)
	return nil
}

func (nm *NatsMessenger) stopJobHandler() {
}

func (nm *NatsMessenger) deleteJobHandler() {
}

func (nm *NatsMessenger) jobEventHandler() {
}
