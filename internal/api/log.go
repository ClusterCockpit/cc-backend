// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/ClusterCockpit/cc-backend/internal/logviewer"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// safePattern restricts the free-form query parameters that reach journalctl.
var safePattern = regexp.MustCompile(`^[a-zA-Z0-9 :\-\.]+$`)

const (
	defaultLogSince = "1 hour ago"
	defaultLogLines = 200
	maxLogLines     = 1000
)

// getLog serves the admin log view. The backend it reads from — the systemd
// journal or the in-process buffer — is selected once at startup by
// internal/logviewer.
//
// Like the other /frontend endpoints this is not part of the documented REST
// API and therefore carries no swagger annotations.
func (api *RestAPI) getLog(rw http.ResponseWriter, r *http.Request) {
	user := repository.GetUserFromContext(r.Context())
	if !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to view logs"), http.StatusForbidden, rw)
		return
	}

	src := logviewer.Get()
	if src == nil {
		handleError(fmt.Errorf("log viewer is disabled by configuration (main.log-source)"),
			http.StatusServiceUnavailable, rw)
		return
	}

	query := logviewer.Query{
		Since: defaultLogSince,
		Lines: defaultLogLines,
		Level: -1,
	}

	if since := r.URL.Query().Get("since"); since != "" {
		if !safePattern.MatchString(since) {
			handleError(fmt.Errorf("invalid 'since' parameter"), http.StatusBadRequest, rw)
			return
		}
		query.Since = since
	}

	if l := r.URL.Query().Get("lines"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			handleError(fmt.Errorf("invalid 'lines' parameter"), http.StatusBadRequest, rw)
			return
		}
		query.Lines = min(n, maxLogLines)
	}

	if level := r.URL.Query().Get("level"); level != "" {
		n, err := strconv.Atoi(level)
		if err != nil || n < 0 || n > 7 {
			handleError(fmt.Errorf("invalid 'level' parameter (must be 0-7)"), http.StatusBadRequest, rw)
			return
		}
		query.Level = n
	}

	if search := r.URL.Query().Get("search"); search != "" {
		if !safePattern.MatchString(search) {
			handleError(fmt.Errorf("invalid 'search' parameter"), http.StatusBadRequest, rw)
			return
		}
		query.Search = search
	}

	entries, err := src.Query(r.Context(), query)
	if err != nil {
		if errors.Is(err, logviewer.ErrInvalidQuery) {
			handleError(err, http.StatusBadRequest, rw)
			return
		}
		handleError(fmt.Errorf("failed to read logs: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(entries); err != nil {
		cclog.Errorf("Failed to encode log entries: %v", err)
	}
}
