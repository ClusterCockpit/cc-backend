package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
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
		var tagId int64
		exists := false

		if exists, tagId = api.JobRepository.TagExists(tag.Type, tag.Name); exists {
			http.Error(rw, fmt.Sprintf("the tag '%s:%s' does not exist", tag.Type, tag.Name), http.StatusNotFound)
			return
		}

		if err := api.JobRepository.AddTag(job.JobID, tagId); err != nil {
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
		log.Warnf("user '%s' used /api/jobs/start_job/ without having the API role", user.Username)
		http.Error(rw, "missing 'api' role", http.StatusForbidden)
		return
	}

	req := schema.JobMeta{BaseJob: schema.JobDefaults}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if config.GetClusterConfig(req.Cluster) == nil || config.GetPartition(req.Cluster, req.Partition) == nil {
		http.Error(rw, fmt.Sprintf("cluster %#v or partition %#v does not exist", req.Cluster, req.Partition), http.StatusBadRequest)
		return
	}

	// TODO: Do more such checks, be smarter with them.
	if len(req.Resources) == 0 || len(req.User) == 0 || req.NumNodes == 0 {
		http.Error(rw, "required fields are missing", http.StatusBadRequest)
		return
	}

	// Check if combination of (job_id, cluster_id, start_time) already exists:
	job, err := api.JobRepository.Find(req.JobID, req.Cluster, req.StartTime)
	if err != nil && err != sql.ErrNoRows {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if err != sql.ErrNoRows {
		http.Error(rw, fmt.Sprintf("a job with that job_id, cluster_id and start_time already exists (database id: %d)", job.ID), http.StatusUnprocessableEntity)
		return
	}

	if req.State == "" {
		req.State = schema.JobStateRunning
	}

	req.RawResources, err = json.Marshal(req.Resources)
	if err != nil {
		http.Error(rw, "while parsing resources: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := api.JobRepository.Start(req)
	if err != nil {
		log.Errorf("insert into job table failed: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
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
		http.Error(rw, "Missing 'api' role", http.StatusForbidden)
		return
	}

	req := StopJobApiRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	id, ok := mux.Vars(r)["id"]
	var job *schema.Job
	var err error
	if ok {
		id, e := strconv.ParseInt(id, 10, 64)
		if e != nil {
			http.Error(rw, e.Error(), http.StatusBadRequest)
			return
		}

		job, err = api.JobRepository.FindById(id)
	} else {
		job, err = api.JobRepository.Find(*req.JobId, *req.Cluster, *req.StartTime)
	}

	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != schema.JobStateRunning {
		http.Error(rw, "stop_time must be larger than start_time and only running jobs can be stopped", http.StatusBadRequest)
		return
	}

	if req.State != "" && !req.State.Valid() {
		http.Error(rw, fmt.Sprintf("invalid job state: '%s'", req.State), http.StatusBadRequest)
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
			http.Error(rw, err.Error(), http.StatusInternalServerError)
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
