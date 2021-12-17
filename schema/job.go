package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type BaseJob struct {
	ID               int64       `json:"id" db:"id"`
	JobID            int64       `json:"jobId" db:"job_id"`
	User             string      `json:"user" db:"user"`
	Project          string      `json:"project" db:"project"`
	Cluster          string      `json:"cluster" db:"cluster"`
	Partition        string      `json:"partition" db:"partition"`
	ArrayJobId       int32       `json:"arrayJobId" db:"array_job_id"`
	NumNodes         int32       `json:"numNodes" db:"num_nodes"`
	NumHWThreads     int32       `json:"numHwthreads" db:"num_hwthreads"`
	NumAcc           int32       `json:"numAcc" db:"num_acc"`
	Exclusive        int32       `json:"exclusive" db:"exclusive"`
	MonitoringStatus int32       `json:"monitoringStatus" db:"monitoring_status"`
	SMT              int32       `json:"smt" db:"smt"`
	State            JobState    `json:"jobState" db:"job_state"`
	Duration         int32       `json:"duration" db:"duration"`
	Tags             []*Tag      `json:"tags"`
	RawResources     []byte      `json:"-" db:"resources"`
	Resources        []Resource  `json:"resources"`
	MetaData         interface{} `json:"metaData" db:"meta_data"`

	MemUsedMax       float64 `json:"-" db:"mem_used_max"`
	FlopsAnyAvg      float64 `json:"-" db:"flops_any_avg"`
	MemBwAvg         float64 `json:"-" db:"mem_bw_avg"`
	LoadAvg          float64 `json:"-" db:"load_avg"`
	NetBwAvg         float64 `json:"-" db:"net_bw_avg"`
	NetDataVolTotal  float64 `json:"-" db:"net_data_vol_total"`
	FileBwAvg        float64 `json:"-" db:"file_bw_avg"`
	FileDataVolTotal float64 `json:"-" db:"file_data_vol_total"`
}

type JobMeta struct {
	BaseJob
	StartTime  int64                    `json:"startTime" db:"start_time"`
	Statistics map[string]JobStatistics `json:"statistics,omitempty"`
}

var JobDefaults BaseJob = BaseJob{
	Exclusive:        1,
	MonitoringStatus: 1,
	MetaData:         "",
}

var JobColumns []string = []string{
	"id", "job_id", "user", "project", "cluster", "partition", "array_job_id", "num_nodes",
	"num_hwthreads", "num_acc", "exclusive", "monitoring_status", "smt", "job_state",
	"duration", "resources", "meta_data",
}

const JobInsertStmt string = `INSERT INTO job (
	job_id, user, project, cluster, partition, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, resources, meta_data,
	mem_used_max, flops_any_avg, mem_bw_avg, load_avg, net_bw_avg, net_data_vol_total, file_bw_avg, file_data_vol_total
) VALUES (
	:job_id, :user, :project, :cluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
	:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :resources, :meta_data,
	:mem_used_max, :flops_any_avg, :mem_bw_avg, :load_avg, :net_bw_avg, :net_data_vol_total, :file_bw_avg, :file_data_vol_total
);`

type Job struct {
	BaseJob
	StartTime time.Time `json:"startTime" db:"start_time"`
}

type Scannable interface {
	StructScan(dest interface{}) error
}

// Helper function for scanning jobs with the `jobTableCols` columns selected.
func ScanJob(row Scannable) (*Job, error) {
	job := &Job{BaseJob: JobDefaults}
	if err := row.StructScan(&job); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(job.RawResources, &job.Resources); err != nil {
		return nil, err
	}

	if job.Duration == 0 && job.State == JobStateRunning {
		job.Duration = int32(time.Since(job.StartTime).Seconds())
	}

	return job, nil
}

type JobStatistics struct {
	Unit string  `json:"unit"`
	Avg  float64 `json:"avg"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
}

type Tag struct {
	ID   int64  `json:"id" db:"id"`
	Type string `json:"type" db:"tag_type"`
	Name string `json:"name" db:"tag_name"`
}

type Resource struct {
	Hostname      string `json:"hostname"`
	HWThreads     []int  `json:"hwthreads,omitempty"`
	Accelerators  []int  `json:"accelerators,omitempty"`
	Configuration string `json:"configuration,omitempty"`
}

type JobState string

const (
	JobStateRunning   JobState = "running"
	JobStateCompleted JobState = "completed"
	JobStateFailed    JobState = "failed"
	JobStateCanceled  JobState = "canceled"
	JobStateStopped   JobState = "stopped"
	JobStateTimeout   JobState = "timeout"
)

func (e *JobState) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = JobState(str)
	if !e.Valid() {
		return errors.New("invalid job state")
	}

	return nil
}

func (e JobState) MarshalGQL(w io.Writer) {
	fmt.Fprintf(w, "\"%s\"", e)
}

func (e JobState) Valid() bool {
	return e == JobStateRunning ||
		e == JobStateCompleted ||
		e == JobStateFailed ||
		e == JobStateCanceled ||
		e == JobStateStopped ||
		e == JobStateTimeout
}
