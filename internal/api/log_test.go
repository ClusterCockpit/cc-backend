// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/logviewer"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-chi/chi/v5"
)

const logContextUserKey repository.ContextKey = "user"

// logRouter mounts the frontend routes without the rest of the application:
// the log endpoint only needs the resolved log source and the user in the
// request context.
func logRouter(enabled bool) *chi.Mux {
	r := chi.NewRouter()
	(&api.RestAPI{LogsEnabled: enabled}).MountFrontendAPIRoutes(r)
	return r
}

func logRequest(t *testing.T, r *chi.Mux, target string, roles []string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := context.WithValue(req.Context(), logContextUserKey, &schema.User{
		Username: "testadmin",
		Projects: make([]string, 0),
		Roles:    roles,
	})

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req.WithContext(ctx))
	return recorder.Result()
}

func TestGetLogFromMemory(t *testing.T) {
	t.Cleanup(func() { logviewer.Init(logviewer.Options{Mode: logviewer.ModeDisabled}) })

	logviewer.InstallSink(100)
	logviewer.Init(logviewer.Options{Mode: logviewer.ModeMemory, BufferSize: 100})

	cclog.Warn("a captured warning")
	cclog.Error("a captured error")

	r := logRouter(true)

	t.Run("AllEntries", func(t *testing.T) {
		response := logRequest(t, r, "/logs/?since=1+hour+ago&lines=100", []string{"admin"})
		if response.StatusCode != http.StatusOK {
			t.Fatalf("got %s", response.Status)
		}

		var entries []logviewer.Entry
		if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
			t.Fatal(err)
		}
		if len(entries) < 2 {
			t.Fatalf("got %d entries, want at least the two logged ones", len(entries))
		}

		// The message keeps whatever cclog's flags add (date, file:line);
		// only the level prefix is stripped.
		last := entries[len(entries)-1]
		if !strings.HasSuffix(last.Message, "a captured error") {
			t.Errorf("last message = %q, want it to end in 'a captured error'", last.Message)
		}
		if last.Priority != 3 {
			t.Errorf("last priority = %d, want 3", last.Priority)
		}
		if last.Timestamp == "" {
			t.Error("timestamp is empty")
		}
	})

	t.Run("Search", func(t *testing.T) {
		response := logRequest(t, r, "/logs/?search=captured+warning", []string{"admin"})
		if response.StatusCode != http.StatusOK {
			t.Fatalf("got %s", response.Status)
		}

		var entries []logviewer.Entry
		if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || !strings.HasSuffix(entries[0].Message, "a captured warning") {
			t.Errorf("got %+v, want only the warning", entries)
		}
	})

	t.Run("InvalidParameters", func(t *testing.T) {
		for _, target := range []string{
			"/logs/?since=" + url.QueryEscape("$(rm -rf /)"),
			"/logs/?search=" + url.QueryEscape(";reboot"),
			"/logs/?lines=abc",
			"/logs/?lines=0",
			"/logs/?level=9",
			"/logs/?since=whenever",
		} {
			response := logRequest(t, r, target, []string{"admin"})
			if response.StatusCode == http.StatusOK {
				t.Errorf("%s: got %s, want an error", target, response.Status)
			}
		}
	})

	t.Run("NonAdminForbidden", func(t *testing.T) {
		response := logRequest(t, r, "/logs/", []string{"user"})
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("got %s, want 403", response.Status)
		}
	})
}

func TestGetLogDisabled(t *testing.T) {
	logviewer.Init(logviewer.Options{Mode: logviewer.ModeDisabled})

	response := logRequest(t, logRouter(false), "/logs/", []string{"admin"})
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %s, want 503", response.Status)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error == "" {
		t.Error("expected an explanatory error message")
	}
}
