// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package logviewer provides the log source backing the admin log view.
//
// Two backends exist:
//
//   - journal: shells out to journalctl and reads the systemd journal of the
//     configured unit. Only usable when cc-backend itself runs as a systemd
//     unit.
//   - memory: an in-process ring buffer fed from cc-lib's per-level loggers.
//     Works in any environment (container, supervisord, plain shell) and is
//     therefore the fallback whenever the journal is not available.
//
// The active backend is resolved once during startup by Init and never changes
// afterwards, so the package state is read without synchronization by the API
// and web layers (same contract as config.Keys).
//
// Note that the memory backend only sees what goes through the cclog loggers.
// cclog.Print, Exit and Abort write straight to stdout and are not captured;
// this notably includes Abortf on fatal configuration errors. cclog.Fatal goes
// through CritLog and is captured.
package logviewer

import (
	"context"
	"errors"
	"io"
	"log"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// Mode selects the log backend.
type Mode string

const (
	// ModeAuto detects the backend from the runtime environment.
	ModeAuto Mode = "auto"
	// ModeJournal reads the systemd journal via journalctl.
	ModeJournal Mode = "journal"
	// ModeMemory serves the in-process ring buffer.
	ModeMemory Mode = "memory"
	// ModeDisabled turns the log view off.
	ModeDisabled Mode = "disabled"
)

// DefaultBufferSize is the number of log records the memory backend retains
// until the configuration is loaded and possibly resizes the buffer.
const DefaultBufferSize = 4096

// defaultUnit is the systemd unit queried when none is configured.
const defaultUnit = "clustercockpit.service"

// Entry is one log record as delivered to the log view. Timestamp is
// microseconds since the epoch as a decimal string, matching journald's
// __REALTIME_TIMESTAMP, which the frontend already parses.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Priority  int    `json:"priority"`
	Message   string `json:"message"`
	Unit      string `json:"unit"`
}

// Query is a validated log query. Level < 0 means "all levels"; otherwise only
// records with a priority <= Level are returned, following journalctl's
// --priority semantics.
type Query struct {
	Since  string
	Lines  int
	Level  int
	Search string
}

// ErrInvalidQuery reports a query a backend cannot interpret, for example an
// unparsable 'since' expression. Callers should map it to a client error.
var ErrInvalidQuery = errors.New("invalid log query")

// Source retrieves log records.
type Source interface {
	Query(ctx context.Context, q Query) ([]Entry, error)
	Mode() Mode
}

// Options configures Init.
type Options struct {
	// Mode is the configured backend. An empty value or ModeAuto triggers
	// detection.
	Mode Mode
	// SystemdUnit is the unit queried in journal mode.
	SystemdUnit string
	// BufferSize is the ring capacity in memory mode.
	BufferSize int
}

// attachment remembers a logger's writer so the sink can be removed again.
type attachment struct {
	logger *log.Logger
	orig   io.Writer
}

var (
	active      = ModeDisabled
	source      Source
	buffer      *ring
	attachments []attachment
)

// InstallSink attaches the in-memory capture to cclog's per-level loggers so
// that log output is recorded from the very first line, before the
// configuration that selects the backend has been read. Init drops the buffer
// again if the resolved backend is not ModeMemory.
//
// Call this immediately after cclog.Init and never call cclog.Init or
// cclog.SetOutputFile afterwards: both replace the loggers and would silently
// detach the sink. InstallSink is idempotent.
func InstallSink(capacity int) {
	if buffer != nil {
		return
	}

	buffer = newRing(capacity)

	// Levels suppressed by cclog.Init write to io.Discard. Skip those so the
	// buffer honours -loglevel and the discard fast path stays intact.
	attach(cclog.DebugLog, cclog.DebugWriter, prioDebug, cclog.DebugPrefix)
	attach(cclog.InfoLog, cclog.InfoWriter, prioInfo, cclog.InfoPrefix)
	attach(cclog.WarnLog, cclog.WarnWriter, prioWarn, cclog.WarnPrefix)
	attach(cclog.ErrLog, cclog.ErrWriter, prioErr, cclog.ErrPrefix)
	attach(cclog.CritLog, cclog.CritWriter, prioCrit, cclog.CritPrefix)
}

func attach(logger *log.Logger, orig io.Writer, prio uint8, prefix string) {
	if logger == nil || orig == io.Discard {
		return
	}

	sink := &levelSink{r: buffer, prio: prio, prefix: []byte(prefix)}
	// The sink comes first so records are kept even if the original writer
	// fails.
	logger.SetOutput(io.MultiWriter(sink, orig))
	attachments = append(attachments, attachment{logger: logger, orig: orig})
}

// detachSink restores the original writers and releases the ring buffer.
func detachSink() {
	for _, a := range attachments {
		a.logger.SetOutput(a.orig)
	}
	attachments = nil
	buffer = nil
}

// Init resolves the effective backend. It must be called once after the
// configuration has been loaded.
func Init(opts Options) {
	configured := opts.Mode
	if configured == "" {
		configured = ModeAuto
	}

	mode := configured
	if mode == ModeAuto {
		mode = detectMode(osProbes())
	}

	switch mode {
	case ModeJournal:
		unit := opts.SystemdUnit
		if unit == "" {
			unit = defaultUnit
		}
		source = &journalSource{unit: unit}
		detachSink()
	case ModeMemory:
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		if buffer == nil {
			InstallSink(size)
		} else {
			buffer.resize(size)
		}
		source = &memorySource{r: buffer}
	default:
		mode = ModeDisabled
		source = nil
		detachSink()
	}

	active = mode
	cclog.Infof("Log viewer: source %s (configured: %s)", mode, configured)
}

// Enabled reports whether the log view is available.
func Enabled() bool { return active != ModeDisabled }

// Active returns the resolved backend.
func Active() Mode { return active }

// Get returns the active log source, or nil if the log view is disabled.
func Get() Source { return source }
