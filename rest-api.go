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
)

type StartJobRequestBody struct {
	JobId     string   `json:"job_id"`
	UserId    string   `json:"user_id"`
	ProjectId string   `json:"project_id"`
	ClusterId string   `json:"cluster_id"`
	StartTime int64    `json:"start_time"`
	Nodes     []string `json:"nodes"`
	Metadata  string   `json:"metadata"`
}

type StartJobResponeBody struct {
	DBID int64 `json:"db_id"`
}

type StopJobRequestBody struct {
	DBID      *int64 `json:"db_id"`
	JobId     string `json:"job_id"`
	ClusterId string `json:"cluster_id"`
	StartTime int64  `json:"start_time"`

	StopTime int64 `json:"stop_time"`
}

func startJob(rw http.ResponseWriter, r *http.Request) {
	req := StartJobRequestBody{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if config.GetClusterConfig(req.ClusterId) == nil {
		http.Error(rw, fmt.Sprintf("cluster '%s' does not exist", req.ClusterId), http.StatusBadRequest)
		return
	}

	res, err := db.Exec(
		`INSERT INTO job (job_id, user_id, cluster_id, start_time, duration, job_state, num_nodes, node_list, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		req.JobId, req.UserId, req.ClusterId, req.StartTime, 0, model.JobStateRunning, len(req.Nodes), strings.Join(req.Nodes, ","), req.Metadata)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("New job started (db-id=%d)\n", id)
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(StartJobResponeBody{
		DBID: id,
	})
}

func stopJob(rw http.ResponseWriter, r *http.Request) {
	req := StopJobRequestBody{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	var job *model.Job
	if req.DBID != nil {
		job, err = graph.ScanJob(sq.Select(graph.JobTableCols...).From("job").Where("job.id = ?", req.DBID).RunWith(db).QueryRow())
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

	job.Duration = int(job.StartTime.Unix() - req.StopTime)
	if err := metricdata.ArchiveJob(job, r.Context()); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := db.Exec(`UPDATE job SET job.duration = ?, job.job_state = ? WHERE job.id = ?;`,
		job.Duration, model.JobStateCompleted, job.ID); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
}
