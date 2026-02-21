// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-chi/chi/v5"
)

const (
	// secondsPerDay is the number of seconds in 24 hours.
	// Used for duplicate job detection within a day window.
	secondsPerDay = 86400
)

// StopJobAPIRequest model
type StopJobAPIRequest struct {
	JobID     *int64          `json:"jobId" example:"123000"`
	Cluster   *string         `json:"cluster" example:"fritz"`
	StartTime *int64          `json:"startTime" example:"1649723812"`
	State     schema.JobState `json:"jobState" validate:"required" example:"completed"`
	StopTime  int64           `json:"stopTime" validate:"required" example:"1649763839"`
}

// DeleteJobAPIRequest model
type DeleteJobAPIRequest struct {
	JobID     *int64  `json:"jobId" validate:"required" example:"123000"` // Cluster Job ID of job
	Cluster   *string `json:"cluster" example:"fritz"`                    // Cluster of job
	StartTime *int64  `json:"startTime" example:"1649723812"`             // Start Time of job as epoch
}

// GetJobsAPIResponse model
type GetJobsAPIResponse struct {
	Jobs  []*schema.Job `json:"jobs"`  // Array of jobs
	Items int           `json:"items"` // Number of jobs returned
	Page  int           `json:"page"`  // Page id returned
}

// APITag model
type APITag struct {
	// Tag Type
	Type  string `json:"type" example:"Debug"`
	Name  string `json:"name" example:"Testjob"` // Tag Name
	Scope string `json:"scope" example:"global"` // Tag Scope for Frontend Display
}

type EditMetaRequest struct {
	Key   string `json:"key" example:"jobScript"`
	Value string `json:"value" example:"bash script"`
}

type TagJobAPIRequest []*APITag

type GetJobAPIRequest []string

type GetJobAPIResponse struct {
	Meta *schema.Job
	Data []*JobMetricWithName
}

type GetCompleteJobAPIResponse struct {
	Meta *schema.Job
	Data schema.JobData
}

type JobMetricWithName struct {
	Metric *schema.JobMetric  `json:"metric"`
	Name   string             `json:"name"`
	Scope  schema.MetricScope `json:"scope"`
}

// getJobs godoc
// @summary     Lists all jobs
// @tags Job query
// @description Get a list of all jobs. Filters can be applied using query parameters.
// @description Number of results can be limited by page. Results are sorted by descending startTime.
// @produce     json
// @param       state          query    string            false "Job State" Enums(running, completed, failed, cancelled, stopped, timeout)
// @param       cluster        query    string            false "Job Cluster"
// @param       start-time     query    string            false "Syntax: '$from-$to', as unix epoch timestamps in seconds"
// @param       items-per-page query    int               false "Items per page (Default: 25)"
// @param       page           query    int               false "Page Number (Default: 1)"
// @param       with-metadata  query    bool              false "Include metadata (e.g. jobScript) in response"
// @success     200            {object} api.GetJobsAPIResponse  "Job array and page info"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401   		   {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/ [get]
func (api *RestAPI) getJobs(rw http.ResponseWriter, r *http.Request) {
	withMetadata := false
	filter := &model.JobFilter{}
	page := &model.PageRequest{ItemsPerPage: 25, Page: 1}
	order := &model.OrderByInput{Field: "startTime", Type: "col", Order: model.SortDirectionEnumDesc}

	for key, vals := range r.URL.Query() {
		switch key {
		case "project":
			filter.Project = &model.StringInput{Eq: &vals[0]}
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
		case "start-time": // ?startTime=1753707480-1754053139
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
			filter.StartTime = &config.TimeRange{From: &ufrom, To: &uto}
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

	results := make([]*schema.Job, 0, len(jobs))
	for _, job := range jobs {
		if withMetadata {
			if _, err = api.JobRepository.FetchMetadata(job); err != nil {
				handleError(err, http.StatusInternalServerError, rw)
				return
			}
		}

		job.Tags, err = api.JobRepository.GetTags(repository.GetUserFromContext(r.Context()), job.ID)
		if err != nil {
			handleError(err, http.StatusInternalServerError, rw)
			return
		}

		if job.MonitoringStatus == schema.MonitoringStatusArchivingSuccessful {
			job.Statistics, err = archive.GetStatistics(job)
			if err != nil {
				handleError(err, http.StatusInternalServerError, rw)
				return
			}
		}

		results = append(results, job)
	}

	cclog.Debugf("/api/jobs: %d jobs returned", len(results))
	rw.Header().Add("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	payload := GetJobsAPIResponse{
		Jobs:  results,
		Items: page.ItemsPerPage,
		Page:  page.Page,
	}

	if err := json.NewEncoder(bw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}

// getCompleteJobByID godoc
// @summary   Get job meta and optional all metric data
// @tags Job query
// @description Job to get is specified by database ID
// @description Returns full job resource information according to 'Job' scheme and all metrics according to 'JobData'.
// @produce     json
// @param       id          path     int                  true "Database ID of Job"
// @param       all-metrics query    bool                 false "Include all available metrics"
// @success     200     {object} api.GetJobAPIResponse      "Job resource"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/{id} [get]
func (api *RestAPI) getCompleteJobByID(rw http.ResponseWriter, r *http.Request) {
	// Fetch job from db
	id := chi.URLParam(r, "id")
	var job *schema.Job
	var err error
	if id != "" {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for id: %w", e), http.StatusBadRequest, rw)
			return
		}

		job, err = api.JobRepository.FindByID(r.Context(), id) // Get Job from Repo by ID
	} else {
		handleError(fmt.Errorf("the parameter 'id' is required"), http.StatusBadRequest, rw)
		return
	}
	if err != nil {
		handleError(fmt.Errorf("finding job with db id %s failed: %w", id, err), http.StatusUnprocessableEntity, rw)
		return
	}

	job.Tags, err = api.JobRepository.GetTags(repository.GetUserFromContext(r.Context()), job.ID)
	if err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return

	}
	if _, err = api.JobRepository.FetchMetadata(job); err != nil {

		handleError(err, http.StatusInternalServerError, rw)
		return
	}

	var scopes []schema.MetricScope

	if job.NumNodes == 1 {
		scopes = []schema.MetricScope{"core"}
	} else {
		scopes = []schema.MetricScope{"node"}
	}

	var data schema.JobData

	metricConfigs := archive.GetCluster(job.Cluster).MetricConfig
	resolution := 0

	for _, mc := range metricConfigs {
		resolution = max(resolution, mc.Timestep)
	}

	if r.URL.Query().Get("all-metrics") == "true" {
		data, err = metricdispatch.LoadData(job, nil, scopes, r.Context(), resolution)
		if err != nil {
			cclog.Warnf("REST: error while loading all-metrics job data for JobID %d on %s", job.JobID, job.Cluster)
			return
		}
	}

	cclog.Debugf("/api/job/%s: get job %d", id, job.JobID)
	rw.Header().Add("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	payload := GetCompleteJobAPIResponse{
		Meta: job,
		Data: data,
	}

	if err := json.NewEncoder(bw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}

// getJobByID godoc
// @summary   Get job meta and configurable metric data
// @tags Job query
// @description Job to get is specified by database ID
// @description Returns full job resource information according to 'Job' scheme and all metrics according to 'JobData'.
// @accept      json
// @produce     json
// @param       id          path     int                  true "Database ID of Job"
// @param       request     body     api.GetJobAPIRequest true  "Array of metric names"
// @success     200     {object} api.GetJobAPIResponse      "Job resource"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/{id} [post]
func (api *RestAPI) getJobByID(rw http.ResponseWriter, r *http.Request) {
	// Fetch job from db
	id := chi.URLParam(r, "id")
	var job *schema.Job
	var err error
	if id != "" {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for id: %w", e), http.StatusBadRequest, rw)
			return
		}

		job, err = api.JobRepository.FindByID(r.Context(), id)
	} else {
		handleError(errors.New("the parameter 'id' is required"), http.StatusBadRequest, rw)
		return
	}
	if err != nil {
		handleError(fmt.Errorf("finding job with db id %s failed: %w", id, err), http.StatusUnprocessableEntity, rw)
		return
	}

	job.Tags, err = api.JobRepository.GetTags(repository.GetUserFromContext(r.Context()), job.ID)
	if err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return

	}
	if _, err = api.JobRepository.FetchMetadata(job); err != nil {

		handleError(err, http.StatusInternalServerError, rw)
		return
	}

	var metrics GetJobAPIRequest
	if err = decode(r.Body, &metrics); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	var scopes []schema.MetricScope

	if job.NumNodes == 1 {
		scopes = []schema.MetricScope{"core"}
	} else {
		scopes = []schema.MetricScope{"node"}
	}

	metricConfigs := archive.GetCluster(job.Cluster).MetricConfig
	resolution := 0

	for _, mc := range metricConfigs {
		resolution = max(resolution, mc.Timestep)
	}

	data, err := metricdispatch.LoadData(job, metrics, scopes, r.Context(), resolution)
	if err != nil {
		cclog.Warnf("REST: error while loading job data for JobID %d on %s", job.JobID, job.Cluster)
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

	cclog.Debugf("/api/job/%s: get job %d", id, job.JobID)
	rw.Header().Add("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	payload := GetJobAPIResponse{
		Meta: job,
		Data: res,
	}

	if err := json.NewEncoder(bw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}

// editMeta godoc
// @summary    Edit meta-data json
// @tags Job add and modify
// @description Edit key value pairs in job metadata json
// @description If a key already exists its content will be overwritten
// @accept      json
// @produce     json
// @param       id      path     int                  true "Job Database ID"
// @param       request body     api.EditMetaRequest  true "Kay value pair to add"
// @success     200     {object} schema.Job                "Updated job resource"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     404     {object} api.ErrorResponse         "Job does not exist"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/edit_meta/{id} [post]
func (api *RestAPI) editMeta(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		handleError(fmt.Errorf("parsing job ID failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	job, err := api.JobRepository.FindByID(r.Context(), id)
	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusNotFound, rw)
		return
	}

	var req EditMetaRequest
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	if err := api.JobRepository.UpdateMetadata(job, req.Key, req.Value); err != nil {
		handleError(fmt.Errorf("updating metadata failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(rw).Encode(job); err != nil {
		cclog.Errorf("Failed to encode job response: %v", err)
	}
}

// tagJob godoc
// @summary     Adds one or more tags to a job
// @tags Job add and modify
// @description Adds tag(s) to a job specified by DB ID. Name and Type of Tag(s) can be chosen freely.
// @description Tag Scope for frontend visibility will default to "global" if none entered, other options: "admin" or specific username.
// @description If tagged job is already finished: Tag will be written directly to respective archive files.
// @accept      json
// @produce     json
// @param       id      path     int                  true "Job Database ID"
// @param       request body     api.TagJobAPIRequest true "Array of tag-objects to add"
// @success     200     {object} schema.Job                "Updated job resource"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     404     {object} api.ErrorResponse         "Job or tag does not exist"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/tag_job/{id} [post]
func (api *RestAPI) tagJob(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		handleError(fmt.Errorf("parsing job ID failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	job, err := api.JobRepository.FindByID(r.Context(), id)
	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusNotFound, rw)
		return
	}

	job.Tags, err = api.JobRepository.GetTags(repository.GetUserFromContext(r.Context()), job.ID)
	if err != nil {
		handleError(fmt.Errorf("getting tags failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	var req TagJobAPIRequest
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	for _, tag := range req {
		tagID, err := api.JobRepository.AddTagOrCreate(repository.GetUserFromContext(r.Context()), *job.ID, tag.Type, tag.Name, tag.Scope)
		if err != nil {
			handleError(fmt.Errorf("adding tag failed: %w", err), http.StatusInternalServerError, rw)
			return
		}

		job.Tags = append(job.Tags, &schema.Tag{
			ID:    tagID,
			Type:  tag.Type,
			Name:  tag.Name,
			Scope: tag.Scope,
		})
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(rw).Encode(job); err != nil {
		cclog.Errorf("Failed to encode job response: %v", err)
	}
}

// removeTagJob godoc
// @summary     Removes one or more tags from a job
// @tags Job add and modify
// @description Removes tag(s) from a job specified by DB ID. Name and Type of Tag(s) must match.
// @description Tag Scope is required for matching, options: "global", "admin". Private tags can not be deleted via API.
// @description If tagged job is already finished: Tag will be removed from respective archive files.
// @accept      json
// @produce     json
// @param       id      path     int                  true "Job Database ID"
// @param       request body     api.TagJobAPIRequest true "Array of tag-objects to remove"
// @success     200     {object} schema.Job                "Updated job resource"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     404     {object} api.ErrorResponse         "Job or tag does not exist"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /jobs/tag_job/{id} [delete]
func (api *RestAPI) removeTagJob(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		handleError(fmt.Errorf("parsing job ID failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	job, err := api.JobRepository.FindByID(r.Context(), id)
	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusNotFound, rw)
		return
	}

	job.Tags, err = api.JobRepository.GetTags(repository.GetUserFromContext(r.Context()), job.ID)
	if err != nil {
		handleError(fmt.Errorf("getting tags failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	var req TagJobAPIRequest
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	for _, rtag := range req {
		// Only Global and Admin Tags
		if rtag.Scope != "global" && rtag.Scope != "admin" {
			cclog.Warnf("Cannot delete private tag for job %d: Skip", job.JobID)
			continue
		}

		remainingTags, err := api.JobRepository.RemoveJobTagByRequest(repository.GetUserFromContext(r.Context()), *job.ID, rtag.Type, rtag.Name, rtag.Scope)
		if err != nil {
			handleError(fmt.Errorf("removing tag failed: %w", err), http.StatusInternalServerError, rw)
			return
		}

		job.Tags = remainingTags
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(rw).Encode(job); err != nil {
		cclog.Errorf("Failed to encode job response: %v", err)
	}
}

// removeTags godoc
// @summary     Removes all tags and job-relations for type:name tuple
// @tags Tag remove
// @description Removes tags by type and name. Name and Type of Tag(s) must match.
// @description Tag Scope is required for matching, options: "global", "admin". Private tags can not be deleted via API.
// @description Tag wills be removed from respective archive files.
// @accept      json
// @produce     plain
// @param       request body     api.TagJobAPIRequest true "Array of tag-objects to remove"
// @success     200     {string} string                    "Success Response"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     404     {object} api.ErrorResponse         "Job or tag does not exist"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /tags/ [delete]
func (api *RestAPI) removeTags(rw http.ResponseWriter, r *http.Request) {
	var req TagJobAPIRequest
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	targetCount := len(req)
	currentCount := 0
	for _, rtag := range req {
		// Only Global and Admin Tags
		if rtag.Scope != "global" && rtag.Scope != "admin" {
			cclog.Warn("Cannot delete private tag: Skip")
			continue
		}

		err := api.JobRepository.RemoveTagByRequest(rtag.Type, rtag.Name, rtag.Scope)
		if err != nil {
			handleError(fmt.Errorf("removing tag failed: %w", err), http.StatusInternalServerError, rw)
			return
		}
		currentCount++
	}

	rw.WriteHeader(http.StatusOK)
	fmt.Fprintf(rw, "Deleted Tags from DB: %d successfull of %d requested\n", currentCount, targetCount)
}

// startJob godoc
// @summary     Adds a new job as "running"
// @tags Job add and modify
// @description Job specified in request body will be saved to database as "running" with new DB ID.
// @description Job specifications follow the 'Job' scheme, API will fail to execute if requirements are not met.
// @accept      json
// @produce     json
// @param       request body     schema.Job true "Job to add"
// @success     201     {object} api.DefaultAPIResponse    "Job added successfully"
// @failure     400     {object} api.ErrorResponse            "Bad Request"
// @failure     401     {object} api.ErrorResponse            "Unauthorized"
// @failure     403     {object} api.ErrorResponse            "Forbidden"
// @failure     422     {object} api.ErrorResponse            "Unprocessable Entity: The combination of jobId, clusterId and startTime does already exist"
// @failure     500     {object} api.ErrorResponse            "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/start_job/ [post]
func (api *RestAPI) startJob(rw http.ResponseWriter, r *http.Request) {
	req := schema.Job{
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
	}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	cclog.Debugf("REST: %s", req.GoString())
	req.State = schema.JobStateRunning

	if err := importer.SanityChecks(&req); err != nil {
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
	}
	if err == nil {
		for _, job := range jobs {
			// Check if jobs are within the same day (prevent duplicates)
			if (req.StartTime - job.StartTime) < secondsPerDay {
				handleError(fmt.Errorf("a job with that jobId, cluster and startTime already exists: dbid: %d, jobid: %d", *job.ID, job.JobID), http.StatusUnprocessableEntity, rw)
				return
			}
		}
	}

	// When tags are present, insert directly into the job table so that the
	// returned ID can be used with AddTagOrCreate (which queries the job table).
	// Jobs without tags use the cache path as before.
	var id int64
	if len(req.Tags) > 0 {
		id, err = api.JobRepository.StartDirect(&req)
	} else {
		id, err = api.JobRepository.Start(&req)
	}
	if err != nil {
		handleError(fmt.Errorf("insert into database failed: %w", err), http.StatusInternalServerError, rw)
		return
	}
	// unlock here, adding Tags can be async
	unlockOnce.Do(api.RepositoryMutex.Unlock)

	for _, tag := range req.Tags {
		if _, err := api.JobRepository.AddTagOrCreate(repository.GetUserFromContext(r.Context()), id, tag.Type, tag.Name, tag.Scope); err != nil {
			handleError(fmt.Errorf("adding tag to new job %d failed: %w", id, err), http.StatusInternalServerError, rw)
			return
		}
	}

	cclog.Infof("new job (id: %d): cluster=%s, jobId=%d, user=%s, startTime=%d", id, req.Cluster, req.JobID, req.User, req.StartTime)
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{
		Message: "success",
	}); err != nil {
		cclog.Errorf("Failed to encode response: %v", err)
	}
}

// stopJobByRequest godoc
// @summary     Marks job as completed and triggers archiving
// @tags Job add and modify
// @description Job to stop is specified by request body. All fields are required in this case.
// @description Returns full job resource information according to 'Job' scheme.
// @produce     json
// @param       request body     api.StopJobAPIRequest true "All fields required"
// @success     200     {object} schema.Job                 "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: job has already been stopped"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/stop_job/ [post]
func (api *RestAPI) stopJobByRequest(rw http.ResponseWriter, r *http.Request) {
	// Parse request body
	req := StopJobAPIRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	// Fetch job (that will be stopped) from db
	var job *schema.Job
	var err error
	if req.JobID == nil {
		handleError(errors.New("the field 'jobId' is required"), http.StatusBadRequest, rw)
		return
	}

	isCached := false
	job, err = api.JobRepository.FindCached(req.JobID, req.Cluster, req.StartTime)
	if err != nil {
		// Not in cache, try main job table
		job, err = api.JobRepository.Find(req.JobID, req.Cluster, req.StartTime)
		if err != nil {
			handleError(fmt.Errorf("finding job failed: %w", err), http.StatusNotFound, rw)
			return
		}
	} else {
		isCached = true
	}

	api.checkAndHandleStopJob(rw, job, req, isCached)
}

// deleteJobByID godoc
// @summary     Remove a job from the sql database
// @tags Job remove
// @description Job to remove is specified by database ID. This will not remove the job from the job archive.
// @produce     json
// @param       id      path     int                   true "Database ID of Job"
// @success     200     {object} api.DefaultAPIResponse  "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/delete_job/{id} [delete]
func (api *RestAPI) deleteJobByID(rw http.ResponseWriter, r *http.Request) {
	// Fetch job (that will be stopped) from db
	id := chi.URLParam(r, "id")
	var err error
	if id != "" {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for id: %w", e), http.StatusBadRequest, rw)
			return
		}

		err = api.JobRepository.DeleteJobByID(id)
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
	if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{
		Message: fmt.Sprintf("Successfully deleted job %s", id),
	}); err != nil {
		cclog.Errorf("Failed to encode response: %v", err)
	}
}

// deleteJobByRequest godoc
// @summary     Remove a job from the sql database
// @tags Job remove
// @description Job to delete is specified by request body. All fields are required in this case.
// @accept      json
// @produce     json
// @param       request body     api.DeleteJobAPIRequest true "All fields required"
// @success     200     {object} api.DefaultAPIResponse  "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/delete_job/ [delete]
func (api *RestAPI) deleteJobByRequest(rw http.ResponseWriter, r *http.Request) {
	// Parse request body
	req := DeleteJobAPIRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	// Fetch job (that will be deleted) from db
	var job *schema.Job
	var err error
	if req.JobID == nil {
		handleError(errors.New("the field 'jobId' is required"), http.StatusBadRequest, rw)
		return
	}

	job, err = api.JobRepository.Find(req.JobID, req.Cluster, req.StartTime)
	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	err = api.JobRepository.DeleteJobByID(*job.ID)
	if err != nil {
		handleError(fmt.Errorf("deleting job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{
		Message: fmt.Sprintf("Successfully deleted job %d", *job.ID),
	}); err != nil {
		cclog.Errorf("Failed to encode response: %v", err)
	}
}

// deleteJobBefore godoc
// @summary     Remove a job from the sql database
// @tags Job remove
// @description Remove all jobs with start time before timestamp. The jobs will not be removed from the job archive.
// @produce     json
// @param       ts      path     int                   true "Unix epoch timestamp"
// @success     200     {object} api.DefaultAPIResponse  "Success message"
// @failure     400     {object} api.ErrorResponse          "Bad Request"
// @failure     401     {object} api.ErrorResponse          "Unauthorized"
// @failure     403     {object} api.ErrorResponse          "Forbidden"
// @failure     404     {object} api.ErrorResponse          "Resource not found"
// @failure     422     {object} api.ErrorResponse          "Unprocessable Entity: finding job failed: sql: no rows in result set"
// @failure     500     {object} api.ErrorResponse          "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/delete_job_before/{ts} [delete]
// @param       omit-tagged query bool false "Omit jobs with tags from deletion"
func (api *RestAPI) deleteJobBefore(rw http.ResponseWriter, r *http.Request) {
	var cnt int
	// Fetch job (that will be stopped) from db
	id := chi.URLParam(r, "ts")
	var err error
	if id != "" {
		ts, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			handleError(fmt.Errorf("integer expected in path for ts: %w", e), http.StatusBadRequest, rw)
			return
		}

		// Check for omit-tagged query parameter
		omitTagged := false
		if omitTaggedStr := r.URL.Query().Get("omit-tagged"); omitTaggedStr != "" {
			omitTagged, e = strconv.ParseBool(omitTaggedStr)
			if e != nil {
				handleError(fmt.Errorf("boolean expected for omit-tagged parameter: %w", e), http.StatusBadRequest, rw)
				return
			}
		}

		cnt, err = api.JobRepository.DeleteJobsBefore(ts, omitTagged)
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
	if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{
		Message: fmt.Sprintf("Successfully deleted %d jobs", cnt),
	}); err != nil {
		cclog.Errorf("Failed to encode response: %v", err)
	}
}

func (api *RestAPI) checkAndHandleStopJob(rw http.ResponseWriter, job *schema.Job, req StopJobAPIRequest, isCached bool) {
	// Sanity checks
	if job.State != schema.JobStateRunning {
		handleError(fmt.Errorf("jobId %d (id %d) on %s : job has already been stopped (state is: %s)", job.JobID, *job.ID, job.Cluster, job.State), http.StatusUnprocessableEntity, rw)
		return
	}

	if job.StartTime > req.StopTime {
		handleError(fmt.Errorf("jobId %d (id %d) on %s : stopTime %d must be larger/equal than startTime %d", job.JobID, *job.ID, job.Cluster, req.StopTime, job.StartTime), http.StatusBadRequest, rw)
		return
	}

	if req.State != "" && !req.State.Valid() {
		handleError(fmt.Errorf("jobId %d (id %d) on %s : invalid requested job state: %#v", job.JobID, *job.ID, job.Cluster, req.State), http.StatusBadRequest, rw)
		return
	} else if req.State == "" {
		req.State = schema.JobStateCompleted
	}

	// Mark job as stopped in the database (update state and duration)
	job.Duration = int32(req.StopTime - job.StartTime)
	job.State = req.State
	api.JobRepository.Mutex.Lock()
	defer api.JobRepository.Mutex.Unlock()

	// If the job is still in job_cache, transfer it to the job table first
	// so that job.ID always points to the job table for downstream code
	if isCached {
		newID, err := api.JobRepository.TransferCachedJobToMain(*job.ID)
		if err != nil {
			handleError(fmt.Errorf("jobId %d (id %d) on %s : transferring cached job failed: %w", job.JobID, *job.ID, job.Cluster, err), http.StatusInternalServerError, rw)
			return
		}
		cclog.Infof("transferred cached job to main table: old id %d -> new id %d (jobId=%d)", *job.ID, newID, job.JobID)
		job.ID = &newID
	}

	if err := api.JobRepository.Stop(*job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
		handleError(fmt.Errorf("jobId %d (id %d) on %s : marking job as '%s' (duration: %d) in DB failed: %w", job.JobID, *job.ID, job.Cluster, job.State, job.Duration, err), http.StatusInternalServerError, rw)
		return
	}

	cclog.Infof("archiving job... (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%d, duration=%d, state=%s", *job.ID, job.Cluster, job.JobID, job.User, job.StartTime, job.Duration, job.State)

	// Send a response (with status OK). This means that errors that happen from here on forward
	// can *NOT* be communicated to the client. If reading from a MetricDataRepository or
	// writing to the filesystem fails, the client will not know.
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(rw).Encode(job); err != nil {
		cclog.Errorf("Failed to encode job response: %v", err)
	}

	// Monitoring is disabled...
	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return
	}

	// Trigger async archiving
	archiver.TriggerArchiving(job)
}

func (api *RestAPI) getJobMetrics(rw http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	metrics := r.URL.Query()["metric"]
	var scopes []schema.MetricScope
	for _, scope := range r.URL.Query()["scope"] {
		var s schema.MetricScope
		if err := s.UnmarshalGQL(scope); err != nil {
			handleError(fmt.Errorf("unmarshaling scope failed: %w", err), http.StatusBadRequest, rw)
			return
		}
		scopes = append(scopes, s)
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	type Response struct {
		Data *struct {
			JobMetrics []*model.JobMetricWithName `json:"jobMetrics"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	resolver := graph.GetResolverInstance()
	data, err := resolver.Query().JobMetrics(r.Context(), id, metrics, scopes, nil)
	if err != nil {
		if err := json.NewEncoder(rw).Encode(Response{
			Error: &struct {
				Message string `json:"message"`
			}{Message: err.Error()},
		}); err != nil {
			cclog.Errorf("Failed to encode error response: %v", err)
		}
		return
	}

	if err := json.NewEncoder(rw).Encode(Response{
		Data: &struct {
			JobMetrics []*model.JobMetricWithName `json:"jobMetrics"`
		}{JobMetrics: data},
	}); err != nil {
		cclog.Errorf("Failed to encode response: %v", err)
	}
}

// GetUsedNodesAPIResponse model
type GetUsedNodesAPIResponse struct {
	UsedNodes map[string][]string `json:"usedNodes"` // Map of cluster names to lists of used node hostnames
}

// getUsedNodes godoc
// @summary     Lists used nodes by cluster
// @tags Job query
// @description Get a map of cluster names to lists of unique hostnames that are currently in use by running jobs that started before the specified timestamp.
// @produce     json
// @param       ts             query    int               true  "Unix timestamp to filter jobs (jobs with start_time < ts)"
// @success     200            {object} api.GetUsedNodesAPIResponse  "Map of cluster names to hostname lists"
// @failure     400            {object} api.ErrorResponse            "Bad Request"
// @failure     401            {object} api.ErrorResponse            "Unauthorized"
// @failure     403            {object} api.ErrorResponse            "Forbidden"
// @failure     500            {object} api.ErrorResponse            "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/jobs/used_nodes [get]
func (api *RestAPI) getUsedNodes(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleAPI) {
		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleAPI)), http.StatusForbidden, rw)
		return
	}

	tsStr := r.URL.Query().Get("ts")
	if tsStr == "" {
		handleError(fmt.Errorf("missing required query parameter: ts"), http.StatusBadRequest, rw)
		return
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		handleError(fmt.Errorf("invalid timestamp format: %w", err), http.StatusBadRequest, rw)
		return
	}

	usedNodes, err := api.JobRepository.GetUsedNodes(ts)
	if err != nil {
		handleError(fmt.Errorf("failed to get used nodes: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	payload := GetUsedNodesAPIResponse{
		UsedNodes: usedNodes,
	}

	if err := json.NewEncoder(rw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}
