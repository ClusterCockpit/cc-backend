package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph"
	"github.com/ClusterCockpit/cc-jobarchive/metricdata"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

type RestApi struct {
	DB              *sqlx.DB
	Resolver        *graph.Resolver
	AsyncArchiving  bool
	MachineStateDir string
}

func (api *RestApi) MountRoutes(r *mux.Router) {
	r.HandleFunc("/api/jobs/start_job/", api.startJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/api/jobs/stop_job/", api.stopJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/api/jobs/stop_job/{id}", api.stopJob).Methods(http.MethodPost, http.MethodPut)

	r.HandleFunc("/api/jobs/{id}", api.getJob).Methods(http.MethodGet)
	r.HandleFunc("/api/jobs/tag_job/{id}", api.tagJob).Methods(http.MethodPost, http.MethodPatch)

	r.HandleFunc("/api/machine_state/{cluster}/{host}", api.getMachineState).Methods(http.MethodGet)
	r.HandleFunc("/api/machine_state/{cluster}/{host}", api.putMachineState).Methods(http.MethodPut, http.MethodPost)
}

type StartJobApiRespone struct {
	DBID int64 `json:"id"`
}

type StopJobApiRequest struct {
	// JobId, ClusterId and StartTime are optional.
	// They are only used if no database id was provided.
	JobId     *string `json:"jobId"`
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
		if err := sq.Select("id").From("tag").
			Where("tag.tag_type = ?", tag.Type).Where("tag.tag_name = ?", tag.Name).
			RunWith(api.DB).QueryRow().Scan(&tagId); err != nil {
			http.Error(rw, fmt.Sprintf("the tag '%s:%s' does not exist", tag.Type, tag.Name), http.StatusNotFound)
			return
		}

		if _, err := api.DB.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, job.ID, tagId); err != nil {
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

func (api *RestApi) startJob(rw http.ResponseWriter, r *http.Request) {
	req := schema.JobMeta{BaseJob: schema.JobDefaults}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if config.GetClusterConfig(req.Cluster) == nil {
		http.Error(rw, fmt.Sprintf("cluster '%s' does not exist", req.Cluster), http.StatusBadRequest)
		return
	}

	if len(req.Resources) == 0 || len(req.User) == 0 || req.NumNodes == 0 {
		http.Error(rw, "required fields are missing", http.StatusBadRequest)
		return
	}

	// Check if combination of (job_id, cluster_id, start_time) already exists:
	rows, err := api.DB.Query(`SELECT job.id FROM job WHERE job.job_id = ? AND job.cluster = ? AND job.start_time = ?`,
		req.JobID, req.Cluster, req.StartTime)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if rows.Next() {
		var id int64 = -1
		rows.Scan(&id)
		http.Error(rw, fmt.Sprintf("a job with that job_id, cluster_id and start_time already exists (database id: %d)", id), http.StatusUnprocessableEntity)
		return
	}

	job := schema.Job{
		BaseJob:   req.BaseJob,
		StartTime: time.Unix(req.StartTime, 0),
	}

	job.RawResources, err = json.Marshal(req.Resources)
	if err != nil {
		log.Fatal(err)
	}

	res, err := api.DB.NamedExec(schema.JobInsertStmt, job)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("new job (id: %d): cluster=%s, jobId=%d, user=%s, startTime=%d\n", id, req.Cluster, req.JobID, req.User, req.StartTime)
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(StartJobApiRespone{
		DBID: id,
	})
}

func (api *RestApi) stopJob(rw http.ResponseWriter, r *http.Request) {
	req := StopJobApiRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	var sql string
	var args []interface{}
	id, ok := mux.Vars(r)["id"]
	if ok {
		sql, args, err = sq.Select(schema.JobColumns...).From("job").Where("job.id = ?", id).ToSql()
	} else {
		sql, args, err = sq.Select(schema.JobColumns...).From("job").
			Where("job.job_id = ?", req.JobId).
			Where("job.cluster = ?", req.Cluster).
			Where("job.start_time = ?", req.StartTime).ToSql()
	}
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	job, err := schema.ScanJob(api.DB.QueryRowx(sql, args...))
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

	doArchiving := func(job *schema.Job, ctx context.Context) error {
		job.Duration = int32(req.StopTime - job.StartTime.Unix())
		jobMeta, err := metricdata.ArchiveJob(job, ctx)
		if err != nil {
			log.Printf("archiving job (dbid: %d) failed: %s\n", job.ID, err.Error())
			return err
		}

		stmt := sq.Update("job").
			Set("job_state", req.State).
			Set("duration", job.Duration).
			Where("job.id = ?", job.ID)

		for metric, stats := range jobMeta.Statistics {
			switch metric {
			case "flops_any":
				stmt = stmt.Set("flops_any_avg", stats.Avg)
			case "mem_used":
				stmt = stmt.Set("mem_used_max", stats.Max)
			case "mem_bw":
				stmt = stmt.Set("mem_bw_avg", stats.Avg)
			case "load":
				stmt = stmt.Set("load_avg", stats.Avg)
			case "net_bw":
				stmt = stmt.Set("net_bw_avg", stats.Avg)
			case "file_bw":
				stmt = stmt.Set("file_bw_avg", stats.Avg)
			}
		}

		sql, args, err := stmt.ToSql()
		if err != nil {
			log.Printf("archiving job (dbid: %d) failed: %s\n", job.ID, err.Error())
			return err
		}

		if _, err := api.DB.Exec(sql, args...); err != nil {
			log.Printf("archiving job (dbid: %d) failed: %s\n", job.ID, err.Error())
			return err
		}

		log.Printf("job stopped and archived (dbid: %d)\n", job.ID)
		return nil
	}

	log.Printf("archiving job... (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%s\n", job.ID, job.Cluster, job.JobID, job.User, job.StartTime)
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
