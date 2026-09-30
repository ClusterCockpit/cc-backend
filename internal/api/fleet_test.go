// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/fleet"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

const fleetContextUserKey repository.ContextKey = "user"

// setupFleetAPI wires a fresh database and an initialized fleet subsystem to a
// router carrying only the API routes. The fleet endpoints need neither the
// archive nor auth, so the lightweight struct-literal setup is enough.
func setupFleetAPI(t *testing.T, configTree map[string]string) *chi.Mux {
	t.Helper()
	cclog.Init("warn", true)

	dbfile := filepath.Join(t.TempDir(), "fleet.db")
	if err := repository.ResetConnection(); err != nil {
		t.Fatal(err)
	}
	if err := repository.MigrateDB(dbfile); err != nil {
		t.Fatal(err)
	}
	repository.Connect(dbfile)
	t.Cleanup(func() { repository.ResetConnection() })

	configDir := ""
	if configTree != nil {
		configDir = t.TempDir()
		for rel, content := range configTree {
			full := filepath.Join(configDir, rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := fleet.Init(context.Background(), fleet.Options{
		ConfigDir:     configDir,
		StaleAfter:    time.Hour,
		SweepInterval: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fleet.Reset)

	r := chi.NewRouter()
	(&api.RestAPI{FleetEnabled: true}).MountAPIRoutes(r)
	return r
}

// fleetRequest issues a request with the given roles in the request context.
func fleetRequest(t *testing.T, r *chi.Mux, method, target, body string, roles []string, headers map[string]string) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	ctx := context.WithValue(req.Context(), fleetContextUserKey, &schema.User{
		Username: "fleetagent",
		Projects: make([]string, 0),
		Roles:    roles,
	})

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req.WithContext(ctx))
	return recorder.Result()
}

func apiRoles() []string { return []string{schema.GetRoleString(schema.RoleAPI)} }

// registerFleetService registers a service and returns the decoded response.
func registerFleetService(t *testing.T, r *chi.Mux, path, body string) api.FleetRegisterResponse {
	t.Helper()

	resp := fleetRequest(t, r, http.MethodPost, path, body, apiRoles(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("register %s: got %s, want 201 (%s)", path, resp.Status, payload)
	}

	var reg api.FleetRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		t.Fatal(err)
	}
	if len(reg.InstanceID) != 32 {
		t.Fatalf("expected a 32 character instance id, got %q", reg.InstanceID)
	}
	return reg
}

func TestFleetLifecycle(t *testing.T) {
	r := setupFleetAPI(t, map[string]string{
		"defaults.json":         `{"interval":"10s"}`,
		"ccms/defaults.json":    `{"port":8081}`,
		"ccms/fritz/f0101.json": `{"port":9091}`,
	})

	reg := registerFleetService(t, r, "/fleet/register/cluster/",
		`{"cluster":"fritz","hostname":"f0101","serviceType":"ccms"}`)
	if reg.ConfigRevision != 0 {
		t.Fatalf("a fresh registration must report revision 0, got %d", reg.ConfigRevision)
	}

	var revision string

	t.Run("config pull returns the merged tree with an ETag", func(t *testing.T) {
		resp := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+reg.InstanceID, "", apiRoles(), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %s, want 200", resp.Status)
		}

		revision = resp.Header.Get("X-CC-Config-Revision")
		if revision == "" || resp.Header.Get("ETag") != `"`+revision+`"` {
			t.Fatalf("missing or inconsistent revision headers: ETag=%q revision=%q",
				resp.Header.Get("ETag"), revision)
		}

		var got map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["interval"] != "10s" || got["port"] != float64(9091) {
			t.Fatalf("unexpected merged config: %v", got)
		}
	})

	t.Run("pull acknowledges the revision", func(t *testing.T) {
		svc, err := repository.GetFleetRepository().GetByInstanceID(reg.InstanceID)
		if err != nil {
			t.Fatal(err)
		}
		if got := int64ToString(svc.ConfigRevision); got != revision {
			t.Fatalf("stored revision %s, served %s", got, revision)
		}
	})

	t.Run("If-None-Match yields 304 with an empty body", func(t *testing.T) {
		resp := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+reg.InstanceID, "", apiRoles(),
			map[string]string{"If-None-Match": `"` + revision + `"`})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotModified {
			t.Fatalf("got %s, want 304", resp.Status)
		}
		body, _ := io.ReadAll(resp.Body)
		if len(body) != 0 {
			t.Fatalf("304 must carry no body, got %q", body)
		}
	})

	t.Run("a stale If-None-Match still returns the payload", func(t *testing.T) {
		resp := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+reg.InstanceID, "", apiRoles(),
			map[string]string{"If-None-Match": `"12345"`})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %s, want 200", resp.Status)
		}
	})

	t.Run("heartbeat activates the service", func(t *testing.T) {
		resp := fleetRequest(t, r, http.MethodPost, "/fleet/heartbeat/"+reg.InstanceID, "", apiRoles(), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("got %s, want 204", resp.Status)
		}

		svc, err := repository.GetFleetRepository().GetByInstanceID(reg.InstanceID)
		if err != nil {
			t.Fatal(err)
		}
		if svc.State != "active" {
			t.Fatalf("state %q, want active", svc.State)
		}
	})

	t.Run("deregister is idempotent and terminal", func(t *testing.T) {
		for range 2 {
			resp := fleetRequest(t, r, http.MethodDelete, "/fleet/deregister/"+reg.InstanceID, "", apiRoles(), nil)
			resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("got %s, want 204", resp.Status)
			}
		}

		resp := fleetRequest(t, r, http.MethodPost, "/fleet/heartbeat/"+reg.InstanceID, "", apiRoles(), nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("heartbeat after deregistration: got %s, want 404", resp.Status)
		}

		cfg := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+reg.InstanceID, "", apiRoles(), nil)
		cfg.Body.Close()
		if cfg.StatusCode != http.StatusNotFound {
			t.Fatalf("config pull after deregistration: got %s, want 404", cfg.Status)
		}
	})
}

func TestFleetRegisterInfra(t *testing.T) {
	r := setupFleetAPI(t, map[string]string{
		"defaults.json":   `{"shared":true}`,
		"ccb/mgmt01.json": `{"port":8080}`,
	})

	reg := registerFleetService(t, r, "/fleet/register/infra/",
		`{"hostname":"mgmt01","serviceType":"ccb","metaData":{"endpoint":"https://mgmt:8080"}}`)

	svc, err := repository.GetFleetRepository().GetByInstanceID(reg.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Scope != "infra" || svc.Cluster != "" {
		t.Fatalf("expected an infra row with an empty cluster, got scope=%q cluster=%q", svc.Scope, svc.Cluster)
	}

	resp := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+reg.InstanceID, "", apiRoles(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %s, want 200", resp.Status)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["port"] != float64(8080) || got["shared"] != true {
		t.Fatalf("unexpected merged infra config: %v", got)
	}
}

func TestFleetRegisterInvalid(t *testing.T) {
	r := setupFleetAPI(t, nil)

	tests := []struct {
		name string
		path string
		body string
	}{
		{"missing cluster", "/fleet/register/cluster/", `{"hostname":"f0101","serviceType":"ccms"}`},
		{"missing hostname", "/fleet/register/cluster/", `{"cluster":"fritz","serviceType":"ccms"}`},
		{"unknown service type", "/fleet/register/cluster/", `{"cluster":"fritz","hostname":"f0101","serviceType":"zzz"}`},
		{"path traversal in hostname", "/fleet/register/cluster/", `{"cluster":"fritz","hostname":"../etc","serviceType":"ccms"}`},
		{"path traversal in cluster", "/fleet/register/cluster/", `{"cluster":"../etc","hostname":"f0101","serviceType":"ccms"}`},
		{"unknown field", "/fleet/register/cluster/", `{"cluster":"fritz","hostname":"f0101","serviceType":"ccms","bogus":1}`},
		{"malformed json", "/fleet/register/cluster/", `{`},
		{"infra without hostname", "/fleet/register/infra/", `{"serviceType":"ccb"}`},
		{"infra with unknown service type", "/fleet/register/infra/", `{"hostname":"mgmt01","serviceType":"zzz"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := fleetRequest(t, r, http.MethodPost, tt.path, tt.body, apiRoles(), nil)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("got %s, want 400", resp.Status)
			}
			var errResp api.ErrorResponse
			if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
				t.Fatal(err)
			}
			if errResp.Error == "" {
				t.Fatal("expected an error message")
			}
		})
	}
}

func TestFleetUnknownInstance(t *testing.T) {
	r := setupFleetAPI(t, nil)

	const unknown = "0123456789abcdef0123456789abcdef"
	const malformed = "abc"

	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{"heartbeat unknown", http.MethodPost, "/fleet/heartbeat/" + unknown, http.StatusNotFound},
		{"config unknown", http.MethodGet, "/fleet/config/" + unknown, http.StatusNotFound},
		{"heartbeat malformed", http.MethodPost, "/fleet/heartbeat/" + malformed, http.StatusBadRequest},
		{"config malformed", http.MethodGet, "/fleet/config/" + malformed, http.StatusBadRequest},
		{"deregister malformed", http.MethodDelete, "/fleet/deregister/" + malformed, http.StatusBadRequest},
		{"uppercase hex is malformed", http.MethodGet, "/fleet/config/0123456789ABCDEF0123456789ABCDEF", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := fleetRequest(t, r, tt.method, tt.target, "", apiRoles(), nil)
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Fatalf("got %s, want %d", resp.Status, tt.want)
			}
		})
	}

	// Deregistering an unknown id stays idempotent.
	resp := fleetRequest(t, r, http.MethodDelete, "/fleet/deregister/"+unknown, "", apiRoles(), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("deregister unknown: got %s, want 204", resp.Status)
	}
}

func TestFleetNoConfig(t *testing.T) {
	r := setupFleetAPI(t, nil)

	reg := registerFleetService(t, r, "/fleet/register/cluster/",
		`{"cluster":"fritz","hostname":"f0101","serviceType":"ccms"}`)

	resp := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+reg.InstanceID, "", apiRoles(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %s, want 204", resp.Status)
	}
	if resp.Header.Get("ETag") != "" {
		t.Fatal("204 must not carry an ETag")
	}
}

func TestFleetForbiddenWithoutAPIRole(t *testing.T) {
	r := setupFleetAPI(t, nil)
	userRoles := []string{schema.GetRoleString(schema.RoleUser)}
	const id = "0123456789abcdef0123456789abcdef"

	tests := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodPost, "/fleet/register/cluster/", `{"cluster":"fritz","hostname":"f0101","serviceType":"ccms"}`},
		{http.MethodPost, "/fleet/register/infra/", `{"hostname":"mgmt01","serviceType":"ccb"}`},
		{http.MethodPost, "/fleet/heartbeat/" + id, ""},
		{http.MethodGet, "/fleet/config/" + id, ""},
		{http.MethodDelete, "/fleet/deregister/" + id, ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			resp := fleetRequest(t, r, tt.method, tt.target, tt.body, userRoles, nil)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("got %s, want 403", resp.Status)
			}
		})
	}
}

func TestFleetRoutesNotMountedWhenDisabled(t *testing.T) {
	r := chi.NewRouter()
	(&api.RestAPI{FleetEnabled: false}).MountAPIRoutes(r)

	resp := fleetRequest(t, r, http.MethodPost, "/fleet/register/cluster/",
		`{"cluster":"fritz","hostname":"f0101","serviceType":"ccms"}`, apiRoles(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %s, want 404", resp.Status)
	}
}

func TestFleetReRegisterIssuesNewInstanceID(t *testing.T) {
	r := setupFleetAPI(t, map[string]string{"ccms/defaults.json": `{"port":8081}`})

	const body = `{"cluster":"fritz","hostname":"f0101","serviceType":"ccms"}`
	first := registerFleetService(t, r, "/fleet/register/cluster/", body)

	// Pull once so a revision is on record, then register again.
	pull := fleetRequest(t, r, http.MethodGet, "/fleet/config/"+first.InstanceID, "", apiRoles(), nil)
	pull.Body.Close()
	if pull.StatusCode != http.StatusOK {
		t.Fatalf("config pull: got %s, want 200", pull.Status)
	}
	servedRevision := pull.Header.Get("X-CC-Config-Revision")

	second := registerFleetService(t, r, "/fleet/register/cluster/", body)
	if second.InstanceID == first.InstanceID {
		t.Fatal("re-registration must issue a new instance id")
	}
	if int64ToString(second.ConfigRevision) != servedRevision {
		t.Fatalf("re-registration must preserve the config revision: got %d, want %s",
			second.ConfigRevision, servedRevision)
	}

	resp := fleetRequest(t, r, http.MethodPost, "/fleet/heartbeat/"+first.InstanceID, "", apiRoles(), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("the superseded instance id must no longer heartbeat: got %s, want 404", resp.Status)
	}
}

func int64ToString(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
