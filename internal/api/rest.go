// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
	"github.com/gorilla/mux"
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

type RestApi struct {
	JobRepository   *repository.JobRepository
	Authentication  *auth.Authentication
	MachineStateDir string
	RepositoryMutex sync.Mutex
}

func New() *RestApi {
	return &RestApi{
		JobRepository:   repository.GetJobRepository(),
		MachineStateDir: config.Keys.MachineStateDir,
		Authentication:  auth.GetAuthInstance(),
	}
}

func (api *RestApi) MountApiRoutes(r *mux.Router) {
	r.StrictSlash(true)
	// REST API Uses TokenAuth
	// User List
	r.HandleFunc("/users/", api.getUsers).Methods(http.MethodGet)
	// Cluster List
	r.HandleFunc("/clusters/", api.getClusters).Methods(http.MethodGet)
	// Slurm node state
	r.HandleFunc("/nodestate/", api.updateNodeStates).Methods(http.MethodPost, http.MethodPut)
	// Job Handler
	r.HandleFunc("/jobs/start_job/", api.startJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/jobs/stop_job/", api.stopJobByRequest).Methods(http.MethodPost, http.MethodPut)
	// r.HandleFunc("/jobs/import/", api.importJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/jobs/", api.getJobs).Methods(http.MethodGet)
	r.HandleFunc("/jobs/{id}", api.getJobById).Methods(http.MethodPost)
	r.HandleFunc("/jobs/{id}", api.getCompleteJobById).Methods(http.MethodGet)
	r.HandleFunc("/jobs/tag_job/{id}", api.tagJob).Methods(http.MethodPost, http.MethodPatch)
	r.HandleFunc("/jobs/tag_job/{id}", api.removeTagJob).Methods(http.MethodDelete)
	r.HandleFunc("/jobs/edit_meta/{id}", api.editMeta).Methods(http.MethodPost, http.MethodPatch)
	r.HandleFunc("/jobs/metrics/{id}", api.getJobMetrics).Methods(http.MethodGet)
	r.HandleFunc("/jobs/delete_job/", api.deleteJobByRequest).Methods(http.MethodDelete)
	r.HandleFunc("/jobs/delete_job/{id}", api.deleteJobById).Methods(http.MethodDelete)
	r.HandleFunc("/jobs/delete_job_before/{ts}", api.deleteJobBefore).Methods(http.MethodDelete)

	r.HandleFunc("/tags/", api.removeTags).Methods(http.MethodDelete)

	if api.MachineStateDir != "" {
		r.HandleFunc("/machine_state/{cluster}/{host}", api.getMachineState).Methods(http.MethodGet)
		r.HandleFunc("/machine_state/{cluster}/{host}", api.putMachineState).Methods(http.MethodPut, http.MethodPost)
	}
}

func (api *RestApi) MountUserApiRoutes(r *mux.Router) {
	r.StrictSlash(true)
	// REST API Uses TokenAuth
	r.HandleFunc("/jobs/", api.getJobs).Methods(http.MethodGet)
	r.HandleFunc("/jobs/{id}", api.getJobById).Methods(http.MethodPost)
	r.HandleFunc("/jobs/{id}", api.getCompleteJobById).Methods(http.MethodGet)
	r.HandleFunc("/jobs/metrics/{id}", api.getJobMetrics).Methods(http.MethodGet)
}

func (api *RestApi) MountMetricStoreApiRoutes(r *mux.Router) {
	// REST API Uses TokenAuth
	// Refactor ??
	r.HandleFunc("/api/free", freeMetrics).Methods(http.MethodPost)
	r.HandleFunc("/api/write", writeMetrics).Methods(http.MethodPost)
	r.HandleFunc("/api/debug", debugMetrics).Methods(http.MethodGet)
	r.HandleFunc("/api/healthcheck", metricsHealth).Methods(http.MethodGet)
}

func (api *RestApi) MountConfigApiRoutes(r *mux.Router) {
	r.StrictSlash(true)
	// Settings Frontend Uses SessionAuth
	if api.Authentication != nil {
		r.HandleFunc("/roles/", api.getRoles).Methods(http.MethodGet)
		r.HandleFunc("/users/", api.createUser).Methods(http.MethodPost, http.MethodPut)
		r.HandleFunc("/users/", api.getUsers).Methods(http.MethodGet)
		r.HandleFunc("/users/", api.deleteUser).Methods(http.MethodDelete)
		r.HandleFunc("/user/{id}", api.updateUser).Methods(http.MethodPost)
		r.HandleFunc("/notice/", api.editNotice).Methods(http.MethodPost)
	}
}

func (api *RestApi) MountFrontendApiRoutes(r *mux.Router) {
	r.StrictSlash(true)
	// Settings Frontend Uses SessionAuth
	if api.Authentication != nil {
		r.HandleFunc("/jwt/", api.getJWT).Methods(http.MethodGet)
		r.HandleFunc("/configuration/", api.updateConfiguration).Methods(http.MethodPost)
	}
}

// ErrorResponse model
type ErrorResponse struct {
	// Statustext of Errorcode
	Status string `json:"status"`
	Error  string `json:"error"` // Error Message
}

// DefaultApiResponse model
type DefaultApiResponse struct {
	Message string `json:"msg"`
}

func handleError(err error, statusCode int, rw http.ResponseWriter) {
	cclog.Warnf("REST ERROR : %s", err.Error())
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	json.NewEncoder(rw).Encode(ErrorResponse{
		Status: http.StatusText(statusCode),
		Error:  err.Error(),
	})
}

func decode(r io.Reader, val any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(val)
}

func (api *RestApi) editNotice(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to update the notice.txt file", http.StatusForbidden)
		return
	}

	// Get Value
	newContent := r.FormValue("new-content")

	// Check FIle
	noticeExists := util.CheckFileExists("./var/notice.txt")
	if !noticeExists {
		ntxt, err := os.Create("./var/notice.txt")
		if err != nil {
			cclog.Errorf("Creating ./var/notice.txt failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		ntxt.Close()
	}

	if newContent != "" {
		if err := os.WriteFile("./var/notice.txt", []byte(newContent), 0o666); err != nil {
			cclog.Errorf("Writing to ./var/notice.txt failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		} else {
			rw.Write([]byte("Update Notice Content Success"))
		}
	} else {
		if err := os.WriteFile("./var/notice.txt", []byte(""), 0o666); err != nil {
			cclog.Errorf("Writing to ./var/notice.txt failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		} else {
			rw.Write([]byte("Empty Notice Content Success"))
		}
	}
}

func (api *RestApi) getJWT(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/plain")
	username := r.FormValue("username")
	me := repository.GetUserFromContext(r.Context())
	if !me.HasRole(schema.RoleAdmin) {
		if username != me.Username {
			http.Error(rw, "Only admins are allowed to sign JWTs not for themselves",
				http.StatusForbidden)
			return
		}
	}

	user, err := repository.GetUserRepository().GetUser(username)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	jwt, err := api.Authentication.JwtAuth.ProvideJWT(user)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte(jwt))
}

func (api *RestApi) getRoles(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	user := repository.GetUserFromContext(r.Context())
	if !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "only admins are allowed to fetch a list of roles", http.StatusForbidden)
		return
	}

	roles, err := schema.GetValidRoles(user)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(rw).Encode(roles)
}

func (api *RestApi) updateConfiguration(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/plain")
	key, value := r.FormValue("key"), r.FormValue("value")

	if err := repository.GetUserCfgRepo().UpdateConfig(key, value, repository.GetUserFromContext(r.Context())); err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	rw.Write([]byte("success"))
}

func (api *RestApi) putMachineState(rw http.ResponseWriter, r *http.Request) {
	if api.MachineStateDir == "" {
		http.Error(rw, "REST > machine state not enabled", http.StatusNotFound)
		return
	}

	vars := mux.Vars(r)
	cluster := vars["cluster"]
	host := vars["host"]
	dir := filepath.Join(api.MachineStateDir, cluster)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", host))
	f, err := os.Create(filename)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, r.Body); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusCreated)
}

func (api *RestApi) getMachineState(rw http.ResponseWriter, r *http.Request) {
	if api.MachineStateDir == "" {
		http.Error(rw, "REST > machine state not enabled", http.StatusNotFound)
		return
	}

	vars := mux.Vars(r)
	filename := filepath.Join(api.MachineStateDir, vars["cluster"], fmt.Sprintf("%s.json", vars["host"]))

	// Sets the content-type and 'Last-Modified' Header and so on automatically
	http.ServeFile(rw, r, filename)
}
