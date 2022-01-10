package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Common subset of Job and JobMeta. Use one of those, not
// this type directly.
type BaseJob struct {
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
	Resources        []*Resource `json:"resources"`
	MetaData         interface{} `json:"metaData" db:"meta_data"`
}

// This type is used as the GraphQL interface and using sqlx as a table row.
type Job struct {
	ID int64 `json:"id" db:"id"`
	BaseJob
	StartTime        time.Time `json:"startTime" db:"start_time"`
	MemUsedMax       float64   `json:"-" db:"mem_used_max"`
	FlopsAnyAvg      float64   `json:"-" db:"flops_any_avg"`
	MemBwAvg         float64   `json:"-" db:"mem_bw_avg"`
	LoadAvg          float64   `json:"-" db:"load_avg"`
	NetBwAvg         float64   `json:"-" db:"net_bw_avg"`
	NetDataVolTotal  float64   `json:"-" db:"net_data_vol_total"`
	FileBwAvg        float64   `json:"-" db:"file_bw_avg"`
	FileDataVolTotal float64   `json:"-" db:"file_data_vol_total"`
}

// When reading from the database or sending data via GraphQL, the start time can be in the much more
// convenient time.Time type. In the `meta.json` files, the start time is encoded as a unix epoch timestamp.
// This is why there is this struct, which contains all fields from the regular job struct, but "overwrites"
// the StartTime field with one of type int64.
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
	"job.id", "job.job_id", "job.user", "job.project", "job.cluster", "job.start_time", "job.partition", "job.array_job_id", "job.num_nodes",
	"job.num_hwthreads", "job.num_acc", "job.exclusive", "job.monitoring_status", "job.smt", "job.job_state",
	"job.duration", "job.resources", "job.meta_data",
}

type Scannable interface {
	StructScan(dest interface{}) error
}

// Helper function for scanning jobs with the `jobTableCols` columns selected.
func ScanJob(row Scannable) (*Job, error) {
	job := &Job{BaseJob: JobDefaults}
	if err := row.StructScan(job); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(job.RawResources, &job.Resources); err != nil {
		return nil, err
	}

	if job.Duration == 0 && job.State == JobStateRunning {
		job.Duration = int32(time.Since(job.StartTime).Seconds())
	}

	job.RawResources = nil
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
