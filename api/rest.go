package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/graph"
	"github.com/ClusterCockpit/cc-backend/graph/model"
	"github.com/ClusterCockpit/cc-backend/log"
	"github.com/ClusterCockpit/cc-backend/metricdata"
	"github.com/ClusterCockpit/cc-backend/repository"
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/gorilla/mux"
)

type RestApi struct {
	JobRepository     *repository.JobRepository
	Resolver          *graph.Resolver
	AsyncArchiving    bool
	MachineStateDir   string
	OngoingArchivings sync.WaitGroup
}

func (api *RestApi) MountRoutes(r *mux.Router) {
	r = r.PathPrefix("/api").Subrouter()
	r.StrictSlash(true)

	r.HandleFunc("/jobs/start_job/", api.startJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/jobs/stop_job/", api.stopJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/jobs/stop_job/{id}", api.stopJob).Methods(http.MethodPost, http.MethodPut)

	r.HandleFunc("/jobs/", api.getJobs).Methods(http.MethodGet)
	r.HandleFunc("/jobs/{id}", api.getJob).Methods(http.MethodGet)
	r.HandleFunc("/jobs/tag_job/{id}", api.tagJob).Methods(http.MethodPost, http.MethodPatch)

	r.HandleFunc("/jobs/metrics/{id}", api.getJobMetrics).Methods(http.MethodGet)

	if api.MachineStateDir != "" {
		r.HandleFunc("/machine_state/{cluster}/{host}", api.getMachineState).Methods(http.MethodGet)
		r.HandleFunc("/machine_state/{cluster}/{host}", api.putMachineState).Methods(http.MethodPut, http.MethodPost)
	}
}

type StartJobApiResponse struct {
	DBID int64 `json:"id"`
}

type StopJobApiRequest struct {
	// JobId, ClusterId and StartTime are optional.
	// They are only used if no database id was provided.
	JobId     *int64  `json:"jobId"`
	Cluster   *string `json:"cluster"`
	StartTime *int64  `json:"startTime"`

	// Payload
	StopTime int64           `json:"stopTime"`
	State    schema.JobState `json:"jobState"`
}

type ErrorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

func handleError(err error, statusCode int, rw http.ResponseWriter) {
	log.Printf("REST API error: %s", err.Error())
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	json.NewEncoder(rw).Encode(ErrorResponse{
		Status: http.StatusText(statusCode),
		Error:  err.Error(),
	})
}

type TagJobApiRequest []*struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Return a list of jobs
func (api *RestApi) getJobs(rw http.ResponseWriter, r *http.Request) {
	filter := model.JobFilter{}
	for key, vals := range r.URL.Query() {
		switch key {
		case "state":
			for _, s := range vals {
				state := schema.JobState(s)
				if !state.Valid() {
					http.Error(rw, "invalid query parameter value: state", http.StatusBadRequest)
					return
				}
				filter.State = append(filter.State, state)
			}
		case "cluster":
			filter.Cluster = &model.StringInput{Eq: &vals[0]}
		default:
			http.Error(rw, "invalid query parameter: "+key, http.StatusBadRequest)
			return
		}
	}

	results, err := api.Resolver.Query().Jobs(r.Context(), []*model.JobFilter{&filter}, nil, nil)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	if err := json.NewEncoder(bw).Encode(results.Items); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Return a single job
func (api *RestApi) getJob(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	job, err := api.Resolver.Query().Job(r.Context(), id)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}

	job.Tags, err = api.Resolver.Job().Tags(r.Context(), job)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(job)
}

// Add a tag to a job
func (api *RestApi) tagJob(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	job, err := api.Resolver.Query().Job(r.Context(), id)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}

	job.Tags, err = api.Resolver.Job().Tags(r.Context(), job)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	var req TagJobApiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

// A new job started. The body should be in the `meta.json` format, but some fields required
// there are optional here (e.g. `jobState` defaults to "running").
func (api *RestApi) startJob(rw http.ResponseWriter, r *http.Request) {
	if user := auth.GetUser(r.Context()); user != nil && !user.HasRole(auth.RoleApi) {
		handleError(fmt.Errorf("missing role: %#v", auth.RoleApi), http.StatusForbidden, rw)
		return
	}

	req := schema.JobMeta{BaseJob: schema.JobDefaults}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	if config.GetClusterConfig(req.Cluster) == nil || config.GetPartition(req.Cluster, req.Partition) == nil {
		handleError(fmt.Errorf("cluster or partition does not exist: %#v/%#v", req.Cluster, req.Partition), http.StatusBadRequest, rw)
		return
	}

	// TODO: Do more such checks, be smarter with them.
	if len(req.Resources) == 0 || len(req.User) == 0 || req.NumNodes == 0 {
		handleError(errors.New("the fields 'resources', 'user' and 'numNodes' are required"), http.StatusBadRequest, rw)
		return
	}

	// Check if combination of (job_id, cluster_id, start_time) already exists:
	job, err := api.JobRepository.Find(req.JobID, req.Cluster, req.StartTime)
	if err != nil && err != sql.ErrNoRows {
		handleError(fmt.Errorf("checking for duplicate failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	if err != sql.ErrNoRows {
		handleError(fmt.Errorf("a job with that jobId, cluster and startTime already exists: dbid: %d", job.ID), http.StatusUnprocessableEntity, rw)
		return
	}

	if req.State == "" {
		req.State = schema.JobStateRunning
	}

	req.RawResources, err = json.Marshal(req.Resources)
	if err != nil {
		handleError(fmt.Errorf("basically impossible: %w", err), http.StatusBadRequest, rw)
		return
	}

	id, err := api.JobRepository.Start(&req)
	if err != nil {
		handleError(fmt.Errorf("insert into database failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

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

// A job has stopped and should be archived.
func (api *RestApi) stopJob(rw http.ResponseWriter, r *http.Request) {
	if user := auth.GetUser(r.Context()); user != nil && !user.HasRole(auth.RoleApi) {
		handleError(fmt.Errorf("missing role: %#v", auth.RoleApi), http.StatusForbidden, rw)
		return
	}

	req := StopJobApiRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}

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
		if req.JobId == nil || req.Cluster == nil || req.StartTime == nil {
			handleError(errors.New("the fields 'jobId', 'cluster' and 'startTime' are required"), http.StatusBadRequest, rw)
			return
		}

		job, err = api.JobRepository.Find(*req.JobId, *req.Cluster, *req.StartTime)
	}

	if err != nil {
		handleError(fmt.Errorf("finding job failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != schema.JobStateRunning {
		handleError(errors.New("stopTime must be larger than startTime and only running jobs can be stopped"), http.StatusBadRequest, rw)
		return
	}

	if req.State != "" && !req.State.Valid() {
		handleError(fmt.Errorf("invalid job state: %#v", req.State), http.StatusBadRequest, rw)
		return
	} else {
		req.State = schema.JobStateCompleted
	}

	// This closure does the real work. It needs to be its own
	// function so that it can be done in the background.
	// TODO: Throttle/Have a max. number or parallel archivngs
	// or use a long-running goroutine receiving jobs by a channel.
	doArchiving := func(job *schema.Job, ctx context.Context) error {
		api.OngoingArchivings.Add(1)
		defer api.OngoingArchivings.Done()

		job.Duration = int32(req.StopTime - job.StartTime.Unix())
		jobMeta, err := metricdata.ArchiveJob(job, ctx)
		if err != nil {
			log.Errorf("archiving job (dbid: %d) failed: %s", job.ID, err.Error())
			return err
		}

		api.JobRepository.Stop(job.ID, job.Duration, req.State, jobMeta.Statistics)
		log.Printf("job stopped and archived (dbid: %d)", job.ID)
		return nil
	}

	log.Printf("archiving job... (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%s", job.ID, job.Cluster, job.JobID, job.User, job.StartTime)
	if api.AsyncArchiving {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(job)
		go doArchiving(job, context.Background())
	} else {
		err := doArchiving(job, r.Context())
		if err != nil {
			handleError(fmt.Errorf("archiving failed: %w", err), http.StatusInternalServerError, rw)
		} else {
			rw.Header().Add("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			json.NewEncoder(rw).Encode(job)
		}
	}
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

func (api *RestApi) putMachineState(rw http.ResponseWriter, r *http.Request) {
	if api.MachineStateDir == "" {
		http.Error(rw, "not enabled", http.StatusNotFound)
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
		http.Error(rw, "not enabled", http.StatusNotFound)
		return
	}

	vars := mux.Vars(r)
	filename := filepath.Join(api.MachineStateDir, vars["cluster"], fmt.Sprintf("%s.json", vars["host"]))

	// Sets the content-type and 'Last-Modified' Header and so on automatically
	http.ServeFile(rw, r, filename)
}
