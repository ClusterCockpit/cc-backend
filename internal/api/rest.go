// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package api provides the REST API layer for ClusterCockpit.
// It handles HTTP requests for job management, user administration,
// cluster queries, node state updates, and metrics storage operations.
// The API supports both JWT token authentication and session-based authentication.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/tagger"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/go-chi/chi/v5"
)

// @title                      ClusterCockpit REST API
// @version                    1.0.0
// @description                API for batch job control.

// @contact.name               ClusterCockpit Project
// @contact.url                https://github.com/ClusterCockpit
// @contact.email              support@clustercockpit.org

// @license.name               MIT License
// @license.url                https://opensource.org/licenses/MIT

// @host                       localhost:8080

// @securityDefinitions.apikey ApiKeyAuth
// @in                         header
// @name                       X-Auth-Token

const (
	noticeFilePath  = "./var/notice.txt"
	noticeFilePerms = 0o644
	maxNoticeLength = 10000 // Maximum allowed notice content length in characters
)

type RestAPI struct {
	JobRepository   *repository.JobRepository
	Authentication  *auth.Authentication
	MachineStateDir string
	// RepositoryMutex protects job creation operations from race conditions
	// when checking for duplicate jobs during startJob API calls.
	// It prevents concurrent job starts with the same jobId/cluster/startTime
	// from creating duplicate entries in the database.
	RepositoryMutex sync.Mutex
}

// New creates and initializes a new RestAPI instance with configured dependencies.
func New() *RestAPI {
	return &RestAPI{
		JobRepository:   repository.GetJobRepository(),
		MachineStateDir: config.Keys.MachineStateDir,
		Authentication:  auth.GetAuthInstance(),
	}
}

// MountAPIRoutes registers REST API endpoints for job and cluster management.
// These routes use JWT token authentication via the X-Auth-Token header.
func (api *RestAPI) MountAPIRoutes(r chi.Router) {
	// REST API Uses TokenAuth
	// User List
	r.Get("/users/", api.getUsers)
	// Cluster List
	r.Get("/clusters/", api.getClusters)
	// Slurm node state
	r.Post("/nodestate/", api.updateNodeStates)
	r.Put("/nodestate/", api.updateNodeStates)
	// Job Handler
	if config.Keys.APISubjects == nil {
		cclog.Info("Enabling REST start/stop job API")
		r.Post("/jobs/start_job/", api.startJob)
		r.Put("/jobs/start_job/", api.startJob)
		r.Post("/jobs/stop_job/", api.stopJobByRequest)
		r.Put("/jobs/stop_job/", api.stopJobByRequest)
	}
	r.Get("/jobs/", api.getJobs)
	r.Get("/jobs/used_nodes", api.getUsedNodes)
	r.Post("/jobs/tag_job/{id}", api.tagJob)
	r.Patch("/jobs/tag_job/{id}", api.tagJob)
	r.Delete("/jobs/tag_job/{id}", api.removeTagJob)
	r.Patch("/jobs/edit_meta/{id}", api.editMeta)
	r.Patch("/jobs/edit_meta/", api.editMetaByRequest)
	r.Get("/jobs/metrics/{id}", api.getJobMetrics)
	r.Delete("/jobs/delete_job/", api.deleteJobByRequest)
	r.Delete("/jobs/delete_job/{id}", api.deleteJobByID)
	r.Delete("/jobs/delete_job_before/{ts}", api.deleteJobBefore)
	r.Post("/jobs/{id}", api.getJobByID)
	r.Get("/jobs/{id}", api.getCompleteJobByID)

	r.Delete("/tags/", api.removeTags)

	if api.MachineStateDir != "" {
		r.Get("/machine_state/{cluster}/{host}", api.getMachineState)
		r.Put("/machine_state/{cluster}/{host}", api.putMachineState)
		r.Post("/machine_state/{cluster}/{host}", api.putMachineState)
	}
}

// MountUserAPIRoutes registers user-accessible REST API endpoints.
// These are limited endpoints for regular users with JWT token authentication.
func (api *RestAPI) MountUserAPIRoutes(r chi.Router) {
	// REST API Uses TokenAuth
	r.Get("/jobs/", api.getJobs)
	r.Post("/jobs/{id}", api.getJobByID)
	r.Get("/jobs/{id}", api.getCompleteJobByID)
	r.Get("/jobs/metrics/{id}", api.getJobMetrics)
}

// MountMetricStoreAPIRoutes registers metric storage API endpoints.
// These endpoints handle metric data ingestion and health checks with JWT token authentication.
func (api *RestAPI) MountMetricStoreAPIRoutes(r chi.Router) {
	// REST API Uses TokenAuth
	r.Post("/free", freeMetrics)
	r.Post("/write", writeMetrics)
	r.Get("/debug", debugMetrics)
	r.Post("/healthcheck", api.updateNodeStates)
	// Same endpoints but with trailing slash
	r.Post("/free/", freeMetrics)
	r.Post("/write/", writeMetrics)
	r.Get("/debug/", debugMetrics)
	r.Post("/healthcheck/", api.updateNodeStates)
}

// MountConfigAPIRoutes registers configuration and user management endpoints.
// These routes use session-based authentication and require admin privileges.
// Routes use full paths (including /config prefix) to avoid conflicting with
// the /config page route when registered via Group instead of Route.
func (api *RestAPI) MountConfigAPIRoutes(r chi.Router) {
	// Settings Frontend Uses SessionAuth
	if api.Authentication != nil {
		r.Get("/config/roles/", api.getRoles)
		r.Post("/config/users/", api.createUser)
		r.Put("/config/users/", api.createUser)
		r.Get("/config/users/", api.getUsers)
		r.Delete("/config/users/", api.deleteUser)
		r.Post("/config/user/{id}", api.updateUser)
		r.Post("/config/notice/", api.editNotice)
		r.Get("/config/taggers/", api.getTaggers)
		r.Post("/config/taggers/run/", api.runTagger)
	}
}

// MountFrontendAPIRoutes registers frontend-specific API endpoints.
// These routes support JWT generation and user configuration updates with session authentication.
func (api *RestAPI) MountFrontendAPIRoutes(r chi.Router) {
	r.Get("/logs/", api.getJournalLog)
	// Settings Frontend Uses SessionAuth
	if api.Authentication != nil {
		r.Get("/jwt/", api.getJWT)
		r.Post("/configuration/", api.updateConfiguration)
	}
}

// ErrorResponse model
type ErrorResponse struct {
	// Statustext of Errorcode
	Status string `json:"status"`
	Error  string `json:"error"` // Error Message
}

// DefaultAPIResponse model
type DefaultAPIResponse struct {
	Message string `json:"msg"`
}

// handleError writes a standardized JSON error response with the given status code.
// It logs the error at WARN level and ensures proper Content-Type headers are set.
func handleError(err error, statusCode int, rw http.ResponseWriter) {
	cclog.Warnf("REST ERROR : %s", err.Error())
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	if err := json.NewEncoder(rw).Encode(ErrorResponse{
		Status: http.StatusText(statusCode),
		Error:  err.Error(),
	}); err != nil {
		cclog.Errorf("Failed to encode error response: %v", err)
	}
}

// decode reads JSON from r into val with strict validation that rejects unknown fields.
func decode(r io.Reader, val any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(val)
}

// validatePathComponent checks if a path component contains potentially malicious patterns
// that could be used for path traversal attacks. Returns an error if validation fails.
func validatePathComponent(component, componentName string) error {
	if strings.Contains(component, "..") ||
		strings.Contains(component, "/") ||
		strings.Contains(component, "\\") {
		return fmt.Errorf("invalid %s", componentName)
	}
	return nil
}

// editNotice godoc
// @summary     Update system notice
// @tags        Config
// @description Updates the notice.txt file content. Only admins are allowed. Content is limited to 10000 characters.
// @accept      mpfd
// @produce     plain
// @param       new-content formData string true "New notice content (max 10000 characters)"
// @success     200 {string} string "Update Notice Content Success"
// @failure     400 {object} ErrorResponse "Bad Request"
// @failure     403 {object} ErrorResponse "Forbidden"
// @failure     500 {object} ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /notice/ [post]
func (api *RestAPI) editNotice(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to update the notice.txt file"), http.StatusForbidden, rw)
		return
	}

	// Get Value
	newContent := r.FormValue("new-content")

	if len(newContent) > maxNoticeLength {
		handleError(fmt.Errorf("notice content exceeds maximum length of %d characters", maxNoticeLength), http.StatusBadRequest, rw)
		return
	}

	// Check File
	noticeExists := util.CheckFileExists(noticeFilePath)
	if !noticeExists {
		ntxt, err := os.Create(noticeFilePath)
		if err != nil {
			handleError(fmt.Errorf("creating notice file failed: %w", err), http.StatusInternalServerError, rw)
			return
		}
		if err := ntxt.Close(); err != nil {
			cclog.Warnf("Failed to close notice file: %v", err)
		}
	}

	if err := os.WriteFile(noticeFilePath, []byte(newContent), noticeFilePerms); err != nil {
		handleError(fmt.Errorf("writing to notice file failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.Header().Set("Content-Type", "text/plain")
	rw.WriteHeader(http.StatusOK)
	var msg []byte
	if newContent != "" {
		msg = []byte("Update Notice Content Success")
	} else {
		msg = []byte("Empty Notice Content Success")
	}
	if _, err := rw.Write(msg); err != nil {
		cclog.Errorf("Failed to write response: %v", err)
	}
}

func (api *RestAPI) getTaggers(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to list taggers"), http.StatusForbidden, rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(tagger.ListTaggers()); err != nil {
		cclog.Errorf("Failed to encode tagger list: %v", err)
	}
}

func (api *RestAPI) runTagger(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to run taggers"), http.StatusForbidden, rw)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		handleError(fmt.Errorf("missing required parameter: name"), http.StatusBadRequest, rw)
		return
	}

	if err := tagger.RunTaggerByName(name); err != nil {
		handleError(err, http.StatusConflict, rw)
		return
	}

	rw.Header().Set("Content-Type", "text/plain")
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(fmt.Appendf(nil, "Tagger %s started", name)); err != nil {
		cclog.Errorf("Failed to write response: %v", err)
	}
}

// getJWT godoc
// @summary     Generate JWT token
// @tags        Frontend
// @description Generates a JWT token for a user. Admins can generate tokens for any user, regular users only for themselves.
// @accept      mpfd
// @produce     plain
// @param       username formData string true "Username to generate JWT for"
// @success     200 {string} string "JWT token"
// @failure     403 {object} ErrorResponse "Forbidden"
// @failure     404 {object} ErrorResponse "User Not Found"
// @failure     500 {object} ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jwt/ [get]
func (api *RestAPI) getJWT(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/plain")
	username := r.FormValue("username")
	me := repository.GetUserFromContext(r.Context())
	if !me.HasRole(schema.RoleAdmin) {
		if username != me.Username {
			handleError(fmt.Errorf("only admins are allowed to sign JWTs not for themselves"), http.StatusForbidden, rw)
			return
		}
	}

	user, err := repository.GetUserRepository().GetUser(username)
	if err != nil {
		handleError(fmt.Errorf("getting user failed: %w", err), http.StatusNotFound, rw)
		return
	}

	jwt, err := api.Authentication.JwtAuth.ProvideJWT(user)
	if err != nil {
		handleError(fmt.Errorf("providing JWT failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write([]byte(jwt)); err != nil {
		cclog.Errorf("Failed to write JWT response: %v", err)
	}
}

// getRoles godoc
// @summary     Get available roles
// @tags        Config
// @description Returns a list of valid user roles. Only admins are allowed.
// @produce     json
// @success     200 {array} string "List of role names"
// @failure     403 {object} ErrorResponse "Forbidden"
// @failure     500 {object} ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /roles/ [get]
func (api *RestAPI) getRoles(rw http.ResponseWriter, r *http.Request) {
	user := repository.GetUserFromContext(r.Context())
	if !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to fetch a list of roles"), http.StatusForbidden, rw)
		return
	}

	roles, err := schema.GetValidRoles(user)
	if err != nil {
		handleError(fmt.Errorf("getting valid roles failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(roles); err != nil {
		cclog.Errorf("Failed to encode roles response: %v", err)
	}
}

// updateConfiguration godoc
// @summary     Update user configuration
// @tags        Frontend
// @description Updates a user's configuration key-value pair.
// @accept      mpfd
// @produce     plain
// @param       key formData string true "Configuration key"
// @param       value formData string true "Configuration value"
// @success     200 {string} string "success"
// @failure     500 {object} ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /configuration/ [post]
func (api *RestAPI) updateConfiguration(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/plain")
	key, value := r.FormValue("key"), r.FormValue("value")

	if err := repository.GetUserCfgRepo().UpdateConfig(key, value, repository.GetUserFromContext(r.Context())); err != nil {
		handleError(fmt.Errorf("updating configuration failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write([]byte("success")); err != nil {
		cclog.Errorf("Failed to write response: %v", err)
	}
}

// putMachineState godoc
// @summary     Store machine state
// @tags        Machine State
// @description Stores machine state data for a specific cluster node. Validates cluster and host names to prevent path traversal.
// @accept      json
// @produce     plain
// @param       cluster path string true "Cluster name"
// @param       host path string true "Host name"
// @success     201 "Created"
// @failure     400 {object} ErrorResponse "Bad Request"
// @failure     404 {object} ErrorResponse "Machine state not enabled"
// @failure     500 {object} ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /machine_state/{cluster}/{host} [put]
func (api *RestAPI) putMachineState(rw http.ResponseWriter, r *http.Request) {
	if api.MachineStateDir == "" {
		handleError(fmt.Errorf("machine state not enabled"), http.StatusNotFound, rw)
		return
	}

	cluster := chi.URLParam(r, "cluster")
	host := chi.URLParam(r, "host")

	if err := validatePathComponent(cluster, "cluster name"); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}
	if err := validatePathComponent(host, "host name"); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}

	dir := filepath.Join(api.MachineStateDir, cluster)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		handleError(fmt.Errorf("creating directory failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", host))
	f, err := os.Create(filename)
	if err != nil {
		handleError(fmt.Errorf("creating file failed: %w", err), http.StatusInternalServerError, rw)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, r.Body); err != nil {
		handleError(fmt.Errorf("writing file failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.WriteHeader(http.StatusCreated)
}

// getMachineState godoc
// @summary     Retrieve machine state
// @tags        Machine State
// @description Retrieves stored machine state data for a specific cluster node. Validates cluster and host names to prevent path traversal.
// @produce     json
// @param       cluster path string true "Cluster name"
// @param       host path string true "Host name"
// @success     200 {object} object "Machine state JSON data"
// @failure     400 {object} ErrorResponse "Bad Request"
// @failure     404 {object} ErrorResponse "Machine state not enabled or file not found"
// @security    ApiKeyAuth
// @router      /machine_state/{cluster}/{host} [get]
func (api *RestAPI) getMachineState(rw http.ResponseWriter, r *http.Request) {
	if api.MachineStateDir == "" {
		handleError(fmt.Errorf("machine state not enabled"), http.StatusNotFound, rw)
		return
	}

	cluster := chi.URLParam(r, "cluster")
	host := chi.URLParam(r, "host")

	if err := validatePathComponent(cluster, "cluster name"); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}
	if err := validatePathComponent(host, "host name"); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}

	filename := filepath.Join(api.MachineStateDir, cluster, fmt.Sprintf("%s.json", host))

	// Sets the content-type and 'Last-Modified' Header and so on automatically
	http.ServeFile(rw, r, filename)
}
