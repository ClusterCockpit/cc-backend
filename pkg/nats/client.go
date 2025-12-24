// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package nats provides a generic NATS messaging client for publish/subscribe communication.
//
// The package wraps the nats.go library with connection management, automatic reconnection
// handling, and subscription tracking. It supports multiple authentication methods including
// username/password and credential files.
//
// # Configuration
//
// Configure the client via JSON in the application config:
//
//	{
//	  "nats": {
//	    "address": "nats://localhost:4222",
//	    "username": "user",
//	    "password": "secret"
//	  }
//	}
//
// Or using a credentials file:
//
//	{
//	  "nats": {
//	    "address": "nats://localhost:4222",
//	    "creds-file-path": "/path/to/creds.json"
//	  }
//	}
//
// # Usage
//
// The package provides a singleton client initialized once and retrieved globally:
//
//	nats.Init(rawConfig)
//	nats.Connect()
//
//	client := nats.GetClient()
//	client.Subscribe("events", func(subject string, data []byte) {
//	    fmt.Printf("Received: %s\n", data)
//	})
//
//	client.Publish("events", []byte("hello"))
//
// # Thread Safety
//
// All Client methods are safe for concurrent use.
package nats

import (
	"context"
	"fmt"
	"sync"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/nats-io/nats.go"
)

var (
	clientOnce     sync.Once
	clientInstance *Client
)

// Client wraps a NATS connection with subscription management.
type Client struct {
	conn          *nats.Conn
	subscriptions []*nats.Subscription
	mu            sync.Mutex
}

// MessageHandler is a callback function for processing received messages.
type MessageHandler func(subject string, data []byte)

// Connect initializes the singleton NATS client using the global Keys config.
func Connect() {
	clientOnce.Do(func() {
		if Keys.Address == "" {
			cclog.Warn("NATS: no address configured, skipping connection")
			return
		}

		client, err := NewClient(nil)
		if err != nil {
			cclog.Warnf("NATS connection failed: %v", err)
			return
		}

		clientInstance = client
	})
}

// GetClient returns the singleton NATS client instance.
func GetClient() *Client {
	if clientInstance == nil {
		cclog.Warn("NATS client not initialized")
	}
	return clientInstance
}

// NewClient creates a new NATS client. If cfg is nil, uses the global Keys config.
func NewClient(cfg *NatsConfig) (*Client, error) {
	if cfg == nil {
		cfg = &Keys
	}

	if cfg.Address == "" {
		return nil, fmt.Errorf("NATS address is required")
	}

	var opts []nats.Option

	if cfg.Username != "" && cfg.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	}

	if cfg.CredsFilePath != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFilePath))
	}

	opts = append(opts, nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
		if err != nil {
			cclog.Warnf("NATS disconnected: %v", err)
		}
	}))

	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		cclog.Infof("NATS reconnected to %s", nc.ConnectedUrl())
	}))

	opts = append(opts, nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
		cclog.Errorf("NATS error: %v", err)
	}))

	nc, err := nats.Connect(cfg.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("NATS connect failed: %w", err)
	}

	cclog.Infof("NATS connected to %s", cfg.Address)

	return &Client{
		conn:          nc,
		subscriptions: make([]*nats.Subscription, 0),
	}, nil
}

// Subscribe registers a handler for messages on the given subject.
func (c *Client) Subscribe(subject string, handler MessageHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sub, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
	})
	if err != nil {
		return fmt.Errorf("NATS subscribe to '%s' failed: %w", subject, err)
	}

	c.subscriptions = append(c.subscriptions, sub)
	cclog.Infof("NATS subscribed to '%s'", subject)
	return nil
}

// SubscribeQueue registers a handler with queue group for load-balanced message processing.
func (c *Client) SubscribeQueue(subject, queue string, handler MessageHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sub, err := c.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
	})
	if err != nil {
		return fmt.Errorf("NATS queue subscribe to '%s' (queue: %s) failed: %w", subject, queue, err)
	}

	c.subscriptions = append(c.subscriptions, sub)
	cclog.Infof("NATS queue subscribed to '%s' (queue: %s)", subject, queue)
	return nil
}

// SubscribeChan subscribes to a subject and delivers messages to the provided channel.
func (c *Client) SubscribeChan(subject string, ch chan *nats.Msg) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sub, err := c.conn.ChanSubscribe(subject, ch)
	if err != nil {
		return fmt.Errorf("NATS chan subscribe to '%s' failed: %w", subject, err)
	}

	c.subscriptions = append(c.subscriptions, sub)
	cclog.Infof("NATS chan subscribed to '%s'", subject)
	return nil
}

// Publish sends data to the specified subject.
func (c *Client) Publish(subject string, data []byte) error {
	if err := c.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("NATS publish to '%s' failed: %w", subject, err)
	}
	return nil
}

// Request sends a request and waits for a response with the given context timeout.
func (c *Client) Request(subject string, data []byte, timeout context.Context) ([]byte, error) {
	msg, err := c.conn.RequestWithContext(timeout, subject, data)
	if err != nil {
		return nil, fmt.Errorf("NATS request to '%s' failed: %w", subject, err)
	}
	return msg.Data, nil
}

// Flush flushes the connection buffer to ensure all published messages are sent.
func (c *Client) Flush() error {
	return c.conn.Flush()
}

// Close unsubscribes all subscriptions and closes the NATS connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sub := range c.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			cclog.Warnf("NATS unsubscribe failed: %v", err)
		}
	}
	c.subscriptions = nil

	if c.conn != nil {
		c.conn.Close()
		cclog.Info("NATS connection closed")
	}
}

// IsConnected returns true if the client has an active connection.
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

// Connection returns the underlying NATS connection for advanced usage.
func (c *Client) Connection() *nats.Conn {
	return c.conn
}
