// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// journalScannerMax bounds a single journalctl JSON record. The scanner
// default of 64 KiB is exceeded by long messages, which would otherwise abort
// the scan silently.
const journalScannerMax = 1024 * 1024

// journalSource reads the systemd journal of a unit via journalctl.
type journalSource struct {
	unit string
}

func (s *journalSource) Mode() Mode { return ModeJournal }

// journalArgs builds the journalctl command line for q.
func journalArgs(unit string, q Query) []string {
	args := []string{
		"--output=json",
		"--no-pager",
		"-n", strconv.Itoa(q.Lines),
		"--since", q.Since,
		"-u", unit,
	}

	if q.Level >= 0 {
		args = append(args, "--priority", strconv.Itoa(q.Level))
	}

	if q.Search != "" {
		args = append(args, "--grep", q.Search)
	}

	return args
}

func (s *journalSource) Query(ctx context.Context, q Query) ([]Entry, error) {
	args := journalArgs(s.unit, q)
	cclog.Debugf("calling journalctl with %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start journalctl: %w", err)
	}

	entries := make([]Entry, 0, q.Lines)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), journalScannerMax)
	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			cclog.Debugf("error unmarshal log output: %v", err)
			continue
		}

		entries = append(entries, entryFromJournal(raw))
	}
	if err := scanner.Err(); err != nil {
		cclog.Warnf("error reading journalctl output: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		// journalctl returns exit code 1 when --grep matches nothing
		if len(entries) == 0 {
			cclog.Debugf("journalctl exited with: %v", err)
		}
	}

	return entries, nil
}

// entryFromJournal maps one journald JSON record to an Entry.
func entryFromJournal(raw map[string]any) Entry {
	priority := int(prioInfo)
	if p, ok := raw["PRIORITY"]; ok {
		switch v := p.(type) {
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				priority = n
			}
		case float64:
			priority = int(v)
		}
	}

	msg := ""
	if m, ok := raw["MESSAGE"].(string); ok {
		msg = m
	}

	ts := ""
	if t, ok := raw["__REALTIME_TIMESTAMP"].(string); ok {
		ts = t
	}

	unit := ""
	if u, ok := raw["_SYSTEMD_UNIT"].(string); ok {
		unit = u
	}

	return Entry{
		Timestamp: ts,
		Priority:  priority,
		Message:   msg,
		Unit:      unit,
	}
}
