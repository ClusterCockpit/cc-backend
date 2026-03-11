// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Priority  int    `json:"priority"`
	Message   string `json:"message"`
	Unit      string `json:"unit"`
}

var safePattern = regexp.MustCompile(`^[a-zA-Z0-9 :\-\.]+$`)

func (api *RestAPI) getJournalLog(rw http.ResponseWriter, r *http.Request) {
	user := repository.GetUserFromContext(r.Context())
	if !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to view logs"), http.StatusForbidden, rw)
		return
	}

	since := r.URL.Query().Get("since")
	if since == "" {
		since = "1 hour ago"
	}
	if !safePattern.MatchString(since) {
		handleError(fmt.Errorf("invalid 'since' parameter"), http.StatusBadRequest, rw)
		return
	}

	lines := 200
	if l := r.URL.Query().Get("lines"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			handleError(fmt.Errorf("invalid 'lines' parameter"), http.StatusBadRequest, rw)
			return
		}
		if n > 1000 {
			n = 1000
		}
		lines = n
	}

	unit := config.Keys.SystemdUnit
	if unit == "" {
		unit = "clustercockpit.service"
	}

	args := []string{
		"--output=json",
		"--no-pager",
		"-n", fmt.Sprintf("%d", lines),
		"--since", since,
		"-u", unit,
	}

	if level := r.URL.Query().Get("level"); level != "" {
		n, err := strconv.Atoi(level)
		if err != nil || n < 0 || n > 7 {
			handleError(fmt.Errorf("invalid 'level' parameter (must be 0-7)"), http.StatusBadRequest, rw)
			return
		}
		args = append(args, "--priority", fmt.Sprintf("%d", n))
	}

	if search := r.URL.Query().Get("search"); search != "" {
		if !safePattern.MatchString(search) {
			handleError(fmt.Errorf("invalid 'search' parameter"), http.StatusBadRequest, rw)
			return
		}
		args = append(args, "--grep", search)
	}

	cclog.Debugf("calling journalctl with %s", strings.Join(args, " "))
	cmd := exec.CommandContext(r.Context(), "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		handleError(fmt.Errorf("failed to create pipe: %w", err), http.StatusInternalServerError, rw)
		return
	}

	if err := cmd.Start(); err != nil {
		handleError(fmt.Errorf("failed to start journalctl: %w", err), http.StatusInternalServerError, rw)
		return
	}

	entries := make([]LogEntry, 0, lines)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			cclog.Debugf("error unmarshal log output: %v", err)
			continue
		}

		priority := 6 // default info
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
		if m, ok := raw["MESSAGE"]; ok {
			if s, ok := m.(string); ok {
				msg = s
			}
		}

		ts := ""
		if t, ok := raw["__REALTIME_TIMESTAMP"]; ok {
			if s, ok := t.(string); ok {
				ts = s
			}
		}

		unitName := ""
		if u, ok := raw["_SYSTEMD_UNIT"]; ok {
			if s, ok := u.(string); ok {
				unitName = s
			}
		}

		entries = append(entries, LogEntry{
			Timestamp: ts,
			Priority:  priority,
			Message:   msg,
			Unit:      unitName,
		})
	}

	if err := cmd.Wait(); err != nil {
		// journalctl returns exit code 1 when --grep matches nothing
		if len(entries) == 0 {
			cclog.Debugf("journalctl exited with: %v", err)
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(entries); err != nil {
		cclog.Errorf("Failed to encode log entries: %v", err)
	}
}
