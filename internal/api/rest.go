// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/gorilla/mux"
)

// @title                      ClusterCockpit REST API
// @version                    1.0.0
// @description                API for batch job control.

// @tag.name Job API

// @contact.name               ClusterCockpit Project
// @contact.url                https://github.com/ClusterCockpit
// @contact.email              support@clustercockpit.org

// @license.name               MIT License
// @license.url                https://opensource.org/licenses/MIT

// @host                       localhost:8080
// @basePath                   /api

// @securityDefinitions.apikey ApiKeyAuth
// @in                         header
// @name                       X-Auth-Token

type RestApi struct {
	JobRepository   *repository.JobRepository
	Resolver        *graph.Resolver
	Authentication  *auth.Authentication
	MachineStateDir string
	RepositoryMutex sync.Mutex
}

func (api *RestApi) MountRoutes(r *mux.Router) {
	r = r.PathPrefix("/api").Subrouter()
	r.StrictSlash(true)

	r.HandleFunc("/jobs/start_job/", api.startJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/jobs/stop_job/", api.stopJobByRequest).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/jobs/stop_job/{id}", api.stopJobById).Methods(http.MethodPost, http.MethodPut)
	// r.HandleFunc("/jobs/import/", api.importJob).Methods(http.MethodPost, http.MethodPut)

	r.HandleFunc("/jobs/", api.getJobs).Methods(http.MethodGet)
	r.HandleFunc("/jobs/{id}", api.getJobById).Methods(http.MethodPost)
	r.HandleFunc("/jobs/tag_job/{id}", api.tagJob).Methods(http.MethodPost, http.MethodPatch)
	r.HandleFunc("/jobs/metrics/{id}", api.getJobMetrics).Methods(http.MethodGet)
	r.HandleFunc("/jobs/delete_job/", api.deleteJobByRequest).Methods(http.MethodDelete)
	r.HandleFunc("/jobs/delete_job/{id}", api.deleteJobById).Methods(http.MethodDelete)
	r.HandleFunc("/jobs/delete_job_before/{ts}", api.deleteJobBefore).Methods(http.MethodDelete)

	if api.MachineStateDir != "" {
		r.HandleFunc("/machine_state/{cluster}/{host}", api.getMachineState).Methods(http.MethodGet)
		r.HandleFunc("/machine_state/{cluster}/{host}", api.putMachineState).Methods(http.MethodPut, http.MethodPost)
	}

	if api.Authentication != nil {
		r.HandleFunc("/jwt/", api.getJWT).Methods(http.MethodGet)
		r.HandleFunc("/roles/", api.getRoles).Methods(http.MethodGet)
		r.HandleFunc("/users/", api.createUser).Methods(http.MethodPost, http.MethodPut)
		r.HandleFunc("/users/", api.getUsers).Methods(http.MethodGet)
		r.HandleFunc("/users/", api.deleteUser).Methods(http.MethodDelete)
		r.HandleFunc("/user/{id}", api.updateUser).Methods(http.MethodPost)
		r.HandleFunc("/configuration/", api.updateConfiguration).Methods(http.MethodPost)
	}
}

// StartJobApiResponse model
type StartJobApiResponse struct {
	// Database ID of new job
	DBID int64 `json:"id"`
}

// DeleteJobApiResponse model
type DeleteJobApiResponse struct {
	Message string `json:"msg"`
}

// UpdateUserApiResponse model
type UpdateUserApiResponse struct {
	Message string `json:"msg"`
}

// StopJobApiRequest model
type StopJobApiRequest struct {
	// Stop Time of job as epoch
	StopTime  int64           `json:"stopTime" validate:"required" example:"1649763839"`
	State     schema.JobState `json:"jobState" validate:"required" example:"completed"` // Final job state
	JobId     *int64          `json:"jobId" example:"123000"`                           // Cluster Job ID of job
	Cluster   *string         `json:"cluster" example:"fritz"`                          // Cluster of job
	StartTime *int64          `json:"startTime" example:"1649723812"`                   // Start Time of job as epoch
}

// DeleteJobApiRequest model
type DeleteJobApiRequest struct {
	JobId     *int64  `json:"jobId" validate:"required" example:"123000"` // Cluster Job ID of job
	Cluster   *string `json:"cluster" example:"fritz"`                    // Cluster of job
	StartTime *int64  `json:"startTime" example:"1649723812"`             // Start Time of job as epoch
}

// GetJobsApiResponse model
type GetJobsApiResponse struct {
	Jobs  []*schema.JobMeta `json:"jobs"`  // Array of jobs
	Items int               `json:"items"` // Number of jobs returned
	Page  int               `json:"page"`  // Page id returned
}

// ErrorResponse model
type ErrorResponse struct {
	// Statustext of Errorcode
	Status string `json:"status"`
	Error  string `json:"error"` // Error Message
}

// ApiTag model
type ApiTag struct {
	// Tag Type
	Type string `json:"type" example:"Debug"`
	Name string `json:"name" example:"Testjob"` // Tag Name
}

type TagJobApiRequest []*ApiTag

type GetJobApiRequest []string

type GetJobApiResponse struct {
	Meta *schema.Job
	Data []*JobMetricWithName
}

type JobMetricWithName struct {
	Name   string             `json:"name"`
	Scope  schema.MetricScope `json:"scope"`
	Metric *schema.JobMetric  `json:"metric"`
}

type ApiReturnedUser struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
	Email    string   `json:"email"`
	Projects []string `json:"projects"`
}

func handleError(err error, statusCode int, rw http.ResponseWriter) {
	log.Warnf("REST ERROR : %s", err.Error())
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	json.NewEncoder(rw).Encode(ErrorResponse{
		Status: http.StatusText(statusCode),
		Error:  err.Error(),
	})
}

func decode(r io.Reader, val interface{}) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(val)
}

func securedCheck(r *http.Request) error {
	user := repository.GetUserFromContext(r.Context())
	if user == nil {
		return fmt.Errorf("no user in context")
	}

	if user.AuthType == schema.AuthToken {
		// If nothing declared in config: deny all request to this endpoint
		if config.Keys.ApiAllowedIPs == nil || len(config.Keys.ApiAllowedIPs) == 0 {
			return fmt.Errorf("missing configuration key ApiAllowedIPs")
		}

		if config.Keys.ApiAllowedIPs[0] == "*" {
			return nil
		}

		// extract IP address
		IPAddress := r.Header.Get("X-Real-Ip")
		if IPAddress == "" {
			IPAddress = r.Header.Get("X-Forwarded-For")
		}
		if IPAddress == "" {
			IPAddress = r.RemoteAddr
		}

		// check if IP is allowed
		if !util.Contains(config.Keys.ApiAllowedIPs, IPAddress) {
			return fmt.Errorf("unknown ip: %v", IPAddress)
		}
	}

	return nil
}

// getJobs godoc
// @summary     Lists all jobs
// @tags query
// @description Get a list of all jobs. Filters can be applied using query parameters.
// @description Number of results can be limited by page. Results are sorted by descending startTime.
// @produce     json
// @param       state          query    string            false "Job State" Enums(running, completed, failed, cancelled, stopped, timeout)
// @param       cluster        query    string            false "Job Cluster"
// @param       start-time     query    string            false "Syntax: '$from-$to', as unix epoch timestamps in seconds"
// @param       items-per-page query    int               false "Items per page (Default: 25)"
// @param       page           query    int               false "Page Number (Default: 1)"
// @param       with-metadata  query    bool              false "Include metadata (e.g. jobScript) in response"
// @success     200            {object} api.GetJobsApiResponse  "Job array and page info"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401   		   {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/ [get]
func (api *RestApi) getJobs(rw http.ResponseWriter, r *http.Request) {

	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	withMetadata := false
	filter := &model.JobFilter{}
	page := &model.PageRequest{ItemsPerPage: 25, Page: 1}
	order := &model.OrderByInput{Field: "startTime", Order: model.SortDirectionEnumDesc}

	for key, vals := range r.URL.Query() {
		switch key {
		case "state":
			for _, s := range vals {
				state := schema.JobState(s)
				if !state.Valid() {
					handleError(fmt.Errorf("invalid query parameter value: state"),
						http.StatusBadRequest, rw)
					return
				}
				filter.State = append(filter.State, state)
			}
		case "cluster":
			filter.Cluster = &model.StringInput{Eq: &vals[0]}
		case "start-time":
			st := strings.Split(vals[0], "-")
			if len(st) != 2 {
				handleError(fmt.Errorf("invalid query parameter value: startTime"),
					http.StatusBadRequest, rw)
				return
			}
			from, err := strconv.ParseInt(st[0], 10, 64)
			if err != nil {
				handleError(err, http.StatusBadRequest, rw)
				return
			}
			to, err := strconv.ParseInt(st[1], 10, 64)
			if err != nil {
				handleError(err, http.StatusBadRequest, rw)
				return
			}
			ufrom, uto := time.Unix(from, 0), time.Unix(to, 0)
			filter.StartTime = &schema.TimeRange{From: &ufrom, To: &uto}
		case "page":
			x, err := strconv.Atoi(vals[0])
			if err != nil {
				handleError(err, http.StatusBadRequest, rw)
				return
			}
			page.Page = x
		case "items-per-page":
			x, err := strconv.Atoi(vals[0])
			if err != nil {
				handleError(err, http.StatusBadRequest, rw)
				return
			}
			page.ItemsPerPage = x
		case "with-metadata":
			withMetadata = true
		default:
			handleError(fmt.Errorf("invalid query parameter: %s", key),
				http.StatusBadRequest, rw)
			return
		}
	}

	jobs, err := api.JobRepository.QueryJobs(r.Context(), []*model.JobFilter{filter}, page, order)
	if err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}

	results := make([]*schema.JobMeta, 0, len(jobs))
	for _, job := range jobs {
		if withMetadata {
			if _, err = api.JobRepository.FetchMetadata(job); err != nil {
				handleError(err, http.StatusInternalServerError, rw)
				return
			}
		}

		res := &schema.JobMeta{
			ID:        &job.ID,
			BaseJob:   job.BaseJob,
			StartTime: job.StartTime.Unix(),
		}

		res.Tags, err = api.JobRepository.GetTags(&job.ID)
		if err != nil {
			handleError(err, http.StatusInternalServerError, rw)
			return
		}

		if res.MonitoringStatus == schema.MonitoringStatusArchivingSuccessful {
			res.Statistics, err = archive.GetStatistics(job)
			if err != nil {
				if err != nil {
					handleError(err, http.StatusInternalServerError, rw)
					return
				}
			}
		}

		results = append(results, res)
	}

	log.Debugf("/api/jobs: %d jobs returned", len(results))
	rw.Header().Add("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	payload := GetJobsApiResponse{
		Jobs:  results,
		Items: page.ItemsPerPage,
		Page:  page.Page,
	}

	if err := json.NewEncoder(bw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}

// getJobById godoc
// @summary   Get complete job meta and metric data
// @tags query
// @description Job to get is specified by database ID
// @description Returns full job resource information according to 'JobMeta' scheme and all metrics according to 'JobData'.
// @accept      json
// @produce     json
// @param       id      path     int                   true "Database ID of Job"
// @param       request body     api.GetJobApiRequest true  "Array of metric names"
// @success     200     {object} api.GetJobApiResponse      "Job resource"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/{id} [post]
func (api *RestApi) getJobById(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v",
			schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	// Fetch job from db
	id, ok := mux.Vars(r)["id"]
	var job *schema.Job
	var err error
	if ok {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for id: %w", e), http.StatusBadRequest, rw)
			return
		}

		job, err = api.JobRepository.FindById(id)
	} else {
		handleError(errors.New("the parameter 'id' is required"), http.StatusBadRequest, rw)
		return
	}
	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	var metrics GetJobApiRequest
	if err = decode(r.Body, &metrics); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var scopes []schema.MetricScope

	if job.NumNodes == 1 {
		scopes = []schema.MetricScope{"core"}
	} else {
		scopes = []schema.MetricScope{"node"}
	}

	data, err := metricdata.LoadData(job, metrics, scopes, r.Context())
	if err != nil {
		log.Warn("Error while loading job data")
		return
	}

	res := []*JobMetricWithName{}
	for name, md := range data {
		for scope, metric := range md {
			res = append(res, &JobMetricWithName{
				Name:   name,
				Scope:  scope,
				Metric: metric,
			})
		}
	}

	log.Debugf("/api/job/%s: get job %d", id, job.JobID)
	rw.Header().Add("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	payload := GetJobApiResponse{
		Meta: job,
		Data: res,
	}

	if err := json.NewEncoder(bw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}

// tagJob godoc
// @summary     Adds one or more tags to a job
// @tags add and modify
// @description Adds tag(s) to a job specified by DB ID. Name and Type of Tag(s) can be chosen freely.
// @description If tagged job is already finished: Tag will be written directly to respective archive files.
// @accept      json
// @produce     json
// @param       id      path     int                  true "Job Database ID"
// @param       request body     api.TagJobApiRequest true "Array of tag-objects to add"
// @success     200     {object} schema.Job                "Updated job resource"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     404     {object} api.ErrorResponse         "Job or tag does not exist"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/tag_job/{id} [post]
func (api *RestApi) tagJob(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	iid, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	job, err := api.JobRepository.FindById(iid)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}

	job.Tags, err = api.JobRepository.GetTags(&job.ID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	var req TagJobApiRequest
	if err := decode(r.Body, &req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	for _, tag := range req {
		tagId, err := api.JobRepository.AddTagOrCreate(job.ID, tag.Type, tag.Name)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		job.Tags = append(job.Tags, &schema.Tag{
			ID:   tagId,
			Type: tag.Type,
			Name: tag.Name,
		})
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(job)
}

// startJob godoc
// @summary     Adds a new job as "running"
// @tags add and modify
// @description Job specified in request body will be saved to database as "running" with new DB ID.
// @description Job specifications follow the 'JobMeta' scheme, API will fail to execute if requirements are not met.
// @accept      json
// @produce     json
// @param       request body     schema.JobMeta          true "Job to add"
// @success     201     {object} api.StartJobApiResponse      "Job added successfully"
// @failure     400     {object} api.ErrorResponse            "Bad Request"
// @failure     401     {object} api.ErrorResponse            "Unauthorized"
// @failure     403     {object} api.ErrorResponse            "Forbidden"
// @failure     422     {object} api.ErrorResponse            "Unprocessable Entity: The combination of jobId, clusterId and startTime does already exist"
// @failure     500     {object} api.ErrorResponse            "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/start_job/ [post]
func (api *RestApi) startJob(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	req := schema.JobMeta{BaseJob: schema.JobDefaults}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	if req.State == "" {
		req.State = schema.JobStateRunning
	}
	if err := importer.SanityChecks(&req.BaseJob); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}

	// aquire lock to avoid race condition between API calls
	var unlockOnce sync.Once
	api.RepositoryMutex.Lock()
	defer unlockOnce.Do(api.RepositoryMutex.Unlock)

	// Check if combination of (job_id, cluster_id, start_time) already exists:
	jobs, err := api.JobRepository.FindAll(&req.JobID, &req.Cluster, nil)
	if err != nil && err != sql.ErrNoRows {
		handleError(fmt.Errorf("checking for duplicate failed: %w", err), http.StatusInternalServerError, rw)
		return
	} else if err == nil {
		for _, job := range jobs {
			if (req.StartTime - job.StartTimeUnix) < 86400 {
				handleError(fmt.Errorf("a job with that jobId, cluster and startTime already exists: dbid: %d, jobid: %d", job.ID, job.JobID), http.StatusUnprocessableEntity, rw)
				return
			}
		}
	}

	id, err := api.JobRepository.Start(&req)
	if err != nil {
		handleError(fmt.Errorf("insert into database failed: %w", err), http.StatusInternalServerError, rw)
		return
	}
	// unlock here, adding Tags can be async
	unlockOnce.Do(api.RepositoryMutex.Unlock)

	for _, tag := range req.Tags {
		if _, err := api.JobRepository.AddTagOrCreate(id, tag.Type, tag.Name); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			handleError(fmt.Errorf("adding tag to new job %d failed: %w", id, err), http.StatusInternalServerError, rw)
			return
		}
	}

	log.Printf("new job (id: %d): cluster=%s, jobId=%d, user=%s, startTime=%d", id, req.Cluster, req.JobID, req.User, req.StartTime)
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(StartJobApiResponse{
		DBID: id,
	})
}

// stopJobById godoc
// @summary     Marks job as completed and triggers archiving
// @tags add and modify
// @description Job to stop is specified by database ID. Only stopTime and final state are required in request body.
// @description Returns full job resource information according to 'JobMeta' scheme.
// @accept      json
// @produce     json
// @param       id      path     int                   true "Database ID of Job"
// @param       request body     api.StopJobApiRequest true "stopTime and final state in request body"
// @success     200     {object} schema.JobMeta             "Job resource"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/stop_job/{id} [post]
func (api *RestApi) stopJobById(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	// Parse request body: Only StopTime and State
	req := StopJobApiRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	// Fetch job (that will be stopped) from db
	id, ok := mux.Vars(r)["id"]
	var job *schema.Job
	var err error
	if ok {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for id: %w", e), http.StatusBadRequest, rw)
			return
		}

		job, err = api.JobRepository.FindById(id)
	} else {
		handleError(errors.New("the parameter 'id' is required"), http.StatusBadRequest, rw)
		return
	}
	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	api.checkAndHandleStopJob(rw, job, req)
}

// stopJobByRequest godoc
// @summary     Marks job as completed and triggers archiving
// @tags add and modify
// @description Job to stop is specified by request body. All fields are required in this case.
// @description Returns full job resource information according to 'JobMeta' scheme.
// @produce     json
// @param       request body     api.StopJobApiRequest true "All fields required"
// @success     200     {object} schema.JobMeta             "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/stop_job/ [post]
func (api *RestApi) stopJobByRequest(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	// Parse request body
	req := StopJobApiRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	// Fetch job (that will be stopped) from db
	var job *schema.Job
	var err error
	if req.JobId == nil {
		handleError(errors.New("the field 'jobId' is required"), http.StatusBadRequest, rw)
		return
	}

	job, err = api.JobRepository.Find(req.JobId, req.Cluster, req.StartTime)

	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	api.checkAndHandleStopJob(rw, job, req)
}

// deleteJobById godoc
// @summary     Remove a job from the sql database
// @tags remove
// @description Job to remove is specified by database ID. This will not remove the job from the job archive.
// @produce     json
// @param       id      path     int                   true "Database ID of Job"
// @success     200     {object} api.DeleteJobApiResponse     "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/delete_job/{id} [delete]
func (api *RestApi) deleteJobById(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil && !user.HasRole(schema.RoleApi) {
		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	// Fetch job (that will be stopped) from db
	id, ok := mux.Vars(r)["id"]
	var err error
	if ok {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for id: %w", e), http.StatusBadRequest, rw)
			return
		}

		err = api.JobRepository.DeleteJobById(id)
	} else {
		handleError(errors.New("the parameter 'id' is required"), http.StatusBadRequest, rw)
		return
	}
	if err != nil {
		handleError(fmt.Errorf("deleting job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(DeleteJobApiResponse{
		Message: fmt.Sprintf("Successfully deleted job %s", id),
	})
}

// deleteJobByRequest godoc
// @summary     Remove a job from the sql database
// @tags remove
// @description Job to delete is specified by request body. All fields are required in this case.
// @accept      json
// @produce     json
// @param       request body     api.DeleteJobApiRequest true "All fields required"
// @success     200     {object} api.DeleteJobApiResponse     "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/delete_job/ [delete]
func (api *RestApi) deleteJobByRequest(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {
		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	// Parse request body
	req := DeleteJobApiRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	// Fetch job (that will be deleted) from db
	var job *schema.Job
	var err error
	if req.JobId == nil {
		handleError(errors.New("the field 'jobId' is required"), http.StatusBadRequest, rw)
		return
	}

	job, err = api.JobRepository.Find(req.JobId, req.Cluster, req.StartTime)

	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	err = api.JobRepository.DeleteJobById(job.ID)
	if err != nil {
		handleError(fmt.Errorf("deleting job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(DeleteJobApiResponse{
		Message: fmt.Sprintf("Successfully deleted job %d", job.ID),
	})
}

// deleteJobBefore godoc
// @summary     Remove a job from the sql database
// @tags remove
// @description Remove all jobs with start time before timestamp. The jobs will not be removed from the job archive.
// @produce     json
// @param       ts      path     int                   true "Unix epoch timestamp"
// @success     200     {object} api.DeleteJobApiResponse     "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/delete_job_before/{ts} [delete]
func (api *RestApi) deleteJobBefore(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil && !user.HasRole(schema.RoleApi) {
		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	var cnt int
	// Fetch job (that will be stopped) from db
	id, ok := mux.Vars(r)["ts"]
	var err error
	if ok {
		ts, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for ts: %w", e), http.StatusBadRequest, rw)
			return
		}

		cnt, err = api.JobRepository.DeleteJobsBefore(ts)
	} else {
		handleError(errors.New("the parameter 'ts' is required"), http.StatusBadRequest, rw)
		return
	}
	if err != nil {
		handleError(fmt.Errorf("deleting jobs failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(DeleteJobApiResponse{
		Message: fmt.Sprintf("Successfully deleted %d jobs", cnt),
	})
}

func (api *RestApi) checkAndHandleStopJob(rw http.ResponseWriter, job *schema.Job, req StopJobApiRequest) {

	// Sanity checks
	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != schema.JobStateRunning {
		handleError(errors.New("stopTime must be larger than startTime and only running jobs can be stopped"), http.StatusBadRequest, rw)
		return
	}

	if req.State != "" && !req.State.Valid() {
		handleError(fmt.Errorf("invalid job state: %#v", req.State), http.StatusBadRequest, rw)
		return
	} else if req.State == "" {
		req.State = schema.JobStateCompleted
	}

	// Mark job as stopped in the database (update state and duration)
	job.Duration = int32(req.StopTime - job.StartTime.Unix())
	job.State = req.State
	if err := api.JobRepository.Stop(job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
		handleError(fmt.Errorf("marking job as stopped failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	log.Printf("archiving job... (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%s", job.ID, job.Cluster, job.JobID, job.User, job.StartTime)

	// Send a response (with status OK). This means that erros that happen from here on forward
	// can *NOT* be communicated to the client. If reading from a MetricDataRepository or
	// writing to the filesystem fails, the client will not know.
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(job)

	// Monitoring is disabled...
	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return
	}

	// Trigger async archiving
	api.JobRepository.TriggerArchiving(job)
}

func (api *RestApi) getJobMetrics(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	metrics := r.URL.Query()["metric"]
	var scopes []schema.MetricScope
	for _, scope := range r.URL.Query()["scope"] {
		var s schema.MetricScope
		if err := s.UnmarshalGQL(scope); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		scopes = append(scopes, s)
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	type Respone struct {
		Data *struct {
			JobMetrics []*model.JobMetricWithName `json:"jobMetrics"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	data, err := api.Resolver.Query().JobMetrics(r.Context(), id, metrics, scopes)
	if err != nil {
		json.NewEncoder(rw).Encode(Respone{
			Error: &struct {
				Message string "json:\"message\""
			}{Message: err.Error()},
		})
		return
	}

	json.NewEncoder(rw).Encode(Respone{
		Data: &struct {
			JobMetrics []*model.JobMetricWithName "json:\"jobMetrics\""
		}{JobMetrics: data},
	})
}

// createUser godoc
// @summary     Adds a new user
// @tags add and modify
// @description User specified in form data will be saved to database.
// @accept      mpfd
// @produce     plain
// @param       username formData string                       true  "Unique user ID"
// @param       password formData string                       true  "User password"
// @param       role 	 formData string                       true  "User role" Enums(admin, support, manager, user, api)
// @param       project  formData string                       false "Managed project, required for new manager role user"
// @param       name 	 formData string                       false "Users name"
// @param       email 	 formData string                       false "Users email"
// @success     200      {string} string                       "Success Response"
// @failure     400      {string} string                       "Bad Request"
// @failure     401      {string} string                       "Unauthorized"
// @failure     403      {string} string                       "Forbidden"
// @failure     422      {string} string                       "Unprocessable Entity: creating user failed"
// @failure     500      {string} string                       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /users/ [post]
func (api *RestApi) createUser(rw http.ResponseWriter, r *http.Request) {
	err := securedCheck(r)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	rw.Header().Set("Content-Type", "text/plain")
	me := repository.GetUserFromContext(r.Context())
	if !me.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to create new users", http.StatusForbidden)
		return
	}

	username, password, role, name, email, project := r.FormValue("username"),
		r.FormValue("password"), r.FormValue("role"), r.FormValue("name"),
		r.FormValue("email"), r.FormValue("project")

	if len(password) == 0 && role != schema.GetRoleString(schema.RoleApi) {
		http.Error(rw, "Only API users are allowed to have a blank password (login will be impossible)", http.StatusBadRequest)
		return
	}

	if len(project) != 0 && role != schema.GetRoleString(schema.RoleManager) {
		http.Error(rw, "only managers require a project (can be changed later)",
			http.StatusBadRequest)
		return
	} else if len(project) == 0 && role == schema.GetRoleString(schema.RoleManager) {
		http.Error(rw, "managers require a project to manage (can be changed later)",
			http.StatusBadRequest)
		return
	}

	if err := repository.GetUserRepository().AddUser(&schema.User{
		Username: username,
		Name:     name,
		Password: password,
		Email:    email,
		Projects: []string{project},
		Roles:    []string{role}}); err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	rw.Write([]byte(fmt.Sprintf("User %v successfully created!\n", username)))
}

// deleteUser godoc
// @summary     Deletes a user
// @tags remove
// @description User defined by username in form data will be deleted from database.
// @accept      mpfd
// @produce     plain
// @param       username formData string         true "User ID to delete"
// @success     200      "User deleted successfully"
// @failure     400      {string} string              "Bad Request"
// @failure     401      {string} string              "Unauthorized"
// @failure     403      {string} string              "Forbidden"
// @failure     422      {string} string              "Unprocessable Entity: deleting user failed"
// @failure     500      {string} string              "Internal Server Error"
// @security    ApiKeyAuth
// @router      /users/ [delete]
func (api *RestApi) deleteUser(rw http.ResponseWriter, r *http.Request) {
	err := securedCheck(r)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to delete a user", http.StatusForbidden)
		return
	}

	username := r.FormValue("username")
	if err := repository.GetUserRepository().DelUser(username); err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	rw.WriteHeader(http.StatusOK)
}

// getUsers godoc
// @summary     Returns a list of users
// @tags query
// @description Returns a JSON-encoded list of users.
// @description Required query-parameter defines if all users or only users with additional special roles are returned.
// @produce     json
// @param       not-just-user query bool true "If returned list should contain all users or only users with additional special roles"
// @success     200     {array} api.ApiReturnedUser "List of users returned successfully"
// @failure     400     {string} string             "Bad Request"
// @failure     401     {string} string             "Unauthorized"
// @failure     403     {string} string             "Forbidden"
// @failure     500     {string} string             "Internal Server Error"
// @security    ApiKeyAuth
// @router      /users/ [get]
func (api *RestApi) getUsers(rw http.ResponseWriter, r *http.Request) {
	err := securedCheck(r)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to fetch a list of users", http.StatusForbidden)
		return
	}

	users, err := repository.GetUserRepository().ListUsers(r.URL.Query().Get("not-just-user") == "true")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(rw).Encode(users)
}

// updateUser godoc
// @summary     Updates an existing user
// @tags add and modify
// @description Modifies user defined by username (id) in one of four possible ways.
// @description If more than one formValue is set then only the highest priority field is used.
// @accept      mpfd
// @produce     plain
// @param       id             path     string     true  "Database ID of User"
// @param       add-role       formData string     false "Priority 1: Role to add" Enums(admin, support, manager, user, api)
// @param       remove-role    formData string     false "Priority 2: Role to remove" Enums(admin, support, manager, user, api)
// @param       add-project    formData string     false "Priority 3: Project to add"
// @param       remove-project formData string     false "Priority 4: Project to remove"
// @success     200     {string} string            "Success Response Message"
// @failure     400     {string} string            "Bad Request"
// @failure     401     {string} string            "Unauthorized"
// @failure     403     {string} string            "Forbidden"
// @failure     422     {string} string            "Unprocessable Entity: The user could not be updated"
// @failure     500     {string} string            "Internal Server Error"
// @security    ApiKeyAuth
// @router      /user/{id} [post]
func (api *RestApi) updateUser(rw http.ResponseWriter, r *http.Request) {
	err := securedCheck(r)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to update a user", http.StatusForbidden)
		return
	}

	// Get Values
	newrole := r.FormValue("add-role")
	delrole := r.FormValue("remove-role")
	newproj := r.FormValue("add-project")
	delproj := r.FormValue("remove-project")

	// TODO: Handle anything but roles...
	if newrole != "" {
		if err := repository.GetUserRepository().AddRole(r.Context(), mux.Vars(r)["id"], newrole); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Add Role Success"))
	} else if delrole != "" {
		if err := repository.GetUserRepository().RemoveRole(r.Context(), mux.Vars(r)["id"], delrole); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Remove Role Success"))
	} else if newproj != "" {
		if err := repository.GetUserRepository().AddProject(r.Context(), mux.Vars(r)["id"], newproj); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Add Project Success"))
	} else if delproj != "" {
		if err := repository.GetUserRepository().RemoveProject(r.Context(), mux.Vars(r)["id"], delproj); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Remove Project Success"))
	} else {
		http.Error(rw, "Not Add or Del [role|project]?", http.StatusInternalServerError)
	}
}

func (api *RestApi) getJWT(rw http.ResponseWriter, r *http.Request) {
	err := securedCheck(r)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

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
	err := securedCheck(r)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

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

	fmt.Printf("REST > KEY: %#v\nVALUE: %#v\n", key, value)

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
	if err := os.MkdirAll(dir, 0755); err != nil {
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
