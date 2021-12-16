package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/metricdata"
	sq "github.com/Masterminds/squirrel"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

type RestApi struct {
	DB             *sqlx.DB
	Resolver       *graph.Resolver
	AsyncArchiving bool
}

func (api *RestApi) MountRoutes(r *mux.Router) {
	r.HandleFunc("/api/jobs/start_job/", api.startJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/api/jobs/stop_job/", api.stopJob).Methods(http.MethodPost, http.MethodPut)
	r.HandleFunc("/api/jobs/stop_job/{id}", api.stopJob).Methods(http.MethodPost, http.MethodPut)

	r.HandleFunc("/api/jobs/{id}", api.getJob).Methods(http.MethodGet)
	r.HandleFunc("/api/jobs/tag_job/{id}", api.tagJob).Methods(http.MethodPost, http.MethodPatch)
}

type StartJobApiRequest struct {
	JobId     int64    `json:"jobId"`
	UserId    string   `json:"userId"`
	ClusterId string   `json:"clusterId"`
	StartTime int64    `json:"startTime"`
	MetaData  string   `json:"metaData"`
	ProjectId string   `json:"projectId"`
	Nodes     []string `json:"nodes"`
	NodeList  string   `json:"nodeList"`
}

type StartJobApiRespone struct {
	DBID int64 `json:"id"`
}

type StopJobApiRequest struct {
	// JobId, ClusterId and StartTime are optional.
	// They are only used if no database id was provided.
	JobId     *string `json:"jobId"`
	ClusterId *string `json:"clusterId"`
	StartTime *int64  `json:"startTime"`

	// Payload
	StopTime int64 `json:"stopTime"`
}

type StopJobApiRespone struct {
	DBID string `json:"id"`
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
		var tagId string
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

		job.Tags = append(job.Tags, &model.JobTag{
			ID:      tagId,
			TagType: tag.Type,
			TagName: tag.Name,
		})
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(job)
}

func (api *RestApi) startJob(rw http.ResponseWriter, r *http.Request) {
	req := StartJobApiRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if config.GetClusterConfig(req.ClusterId) == nil {
		http.Error(rw, fmt.Sprintf("cluster '%s' does not exist", req.ClusterId), http.StatusBadRequest)
		return
	}

	if req.Nodes == nil {
		req.Nodes = strings.Split(req.NodeList, "|")
		if len(req.Nodes) == 1 {
			req.Nodes = strings.Split(req.NodeList, ",")
		}
	}
	if len(req.Nodes) == 0 || len(req.Nodes[0]) == 0 || len(req.UserId) == 0 {
		http.Error(rw, "required fields are missing", http.StatusBadRequest)
		return
	}

	// Check if combination of (job_id, cluster_id, start_time) already exists:
	rows, err := api.DB.Query(`SELECT job.id FROM job WHERE job.job_id = ? AND job.cluster_id = ? AND job.start_time = ?`,
		req.JobId, req.ClusterId, req.StartTime)
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

	res, err := api.DB.Exec(
		`INSERT INTO job (job_id, user_id, project_id, cluster_id, start_time, duration, job_state, num_nodes, node_list, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		req.JobId, req.UserId, req.ProjectId, req.ClusterId, req.StartTime, 0, model.JobStateRunning, len(req.Nodes), strings.Join(req.Nodes, ","), req.MetaData)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("new job (id: %d): clusterId=%s, jobId=%d, userId=%s, startTime=%d, nodes=%v\n", id, req.ClusterId, req.JobId, req.UserId, req.StartTime, req.Nodes)
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
	var job *model.Job
	id, ok := mux.Vars(r)["id"]
	if ok {
		job, err = graph.ScanJob(sq.Select(graph.JobTableCols...).From("job").Where("job.id = ?", id).RunWith(api.DB).QueryRow())
	} else {
		job, err = graph.ScanJob(sq.Select(graph.JobTableCols...).From("job").
			Where("job.job_id = ?", req.JobId).
			Where("job.cluster_id = ?", req.ClusterId).
			Where("job.start_time = ?", req.StartTime).
			RunWith(api.DB).QueryRow())
	}
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}

	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != model.JobStateRunning {
		http.Error(rw, "stop_time must be larger than start_time and only running jobs can be stopped", http.StatusBadRequest)
		return
	}

	doArchiving := func(job *model.Job, ctx context.Context) error {
		job.Duration = int(req.StopTime - job.StartTime.Unix())
		jobMeta, err := metricdata.ArchiveJob(job, ctx)
		if err != nil {
			log.Printf("archiving job (id: %s) failed: %s\n", job.ID, err.Error())
			return err
		}

		getAvg := func(metric string) sql.NullFloat64 {
			stats, ok := jobMeta.Statistics[metric]
			if !ok {
				return sql.NullFloat64{Valid: false}
			}
			return sql.NullFloat64{Valid: true, Float64: stats.Avg}
		}

		if _, err := api.DB.Exec(
			`UPDATE job SET
			job_state = ?, duration = ?,
			flops_any_avg = ?, mem_bw_avg = ?, net_bw_avg = ?, file_bw_avg = ?, load_avg = ?
			WHERE job.id = ?`,
			model.JobStateCompleted, job.Duration,
			getAvg("flops_any"), getAvg("mem_bw"), getAvg("net_bw"), getAvg("file_bw"), getAvg("load"),
			job.ID); err != nil {
			log.Printf("archiving job (id: %s) failed: %s\n", job.ID, err.Error())
			return err
		}

		log.Printf("job stopped and archived (id: %s)\n", job.ID)
		return nil
	}

	log.Printf("archiving job... (id: %s): clusterId=%s, jobId=%s, userId=%s, startTime=%s, nodes=%v\n", job.ID, job.ClusterID, job.JobID, job.UserID, job.StartTime, job.Nodes)
	if api.AsyncArchiving {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(StopJobApiRespone{
			DBID: job.ID,
		})
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
