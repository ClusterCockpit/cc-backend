package main

import (
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
)

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

	StopTime int64 `json:"stopTime"`
}

type StopJobApiRespone struct {
	DBID string `json:"id"`
}

func startJob(rw http.ResponseWriter, r *http.Request) {
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

	res, err := db.Exec(
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

func stopJob(rw http.ResponseWriter, r *http.Request) {
	req := StopJobApiRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	var job *model.Job
	id, ok := mux.Vars(r)["id"]
	if ok {
		job, err = graph.ScanJob(sq.Select(graph.JobTableCols...).From("job").Where("job.id = ?", id).RunWith(db).QueryRow())
	} else {
		job, err = graph.ScanJob(sq.Select(graph.JobTableCols...).From("job").
			Where("job.job_id = ?", req.JobId).
			Where("job.cluster_id = ?", req.ClusterId).
			Where("job.start_time = ?", req.StartTime).
			RunWith(db).QueryRow())
	}
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != model.JobStateRunning {
		http.Error(rw, "stop_time must be larger than start_time and only running jobs can be stopped", http.StatusBadRequest)
		return
	}

	job.Duration = int(req.StopTime - job.StartTime.Unix())

	if err := metricdata.ArchiveJob(job, r.Context()); err != nil {
		log.Printf("archiving job (id: %s) failed: %s\n", job.ID, err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := db.Exec(`UPDATE job SET job_state = ? WHERE job.id = ?`, model.JobStateCompleted, job.ID); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("job stoped and archived (id: %s): clusterId=%s, jobId=%s, userId=%s, startTime=%s, nodes=%v\n", job.ID, job.ClusterID, job.JobID, job.UserID, job.StartTime, job.Nodes)
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(StopJobApiRespone{
		DBID: job.ID,
	})
}
