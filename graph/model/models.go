package model

import (
	"time"
)

type Job struct {
	ID          string    `json:"id"`
	JobID       string    `json:"jobId" db:"job_id"`
	UserID      string    `json:"userId" db:"user_id"`
	ProjectID   string    `json:"projectId" db:"project_id"`
	ClusterID   string    `json:"clusterId" db:"cluster_id"`
	StartTime   time.Time `json:"startTime" db:"start_time"`
	Duration    int       `json:"duration" db:"duration"`
	Walltime    *int      `json:"walltime" db:"walltime"`
	Jobstate    *string   `json:"jobstate" db:"job_state"`
	NumNodes    int       `json:"numNodes" db:"num_nodes"`
	NodeList    string    `json:"nodelist" db:"node_list"`
	HasProfile  bool      `json:"hasProfile" db:"has_profile"`
	MemUsedMax  *float64  `json:"memUsedMax" db:"mem_used_max"`
	FlopsAnyAvg *float64  `json:"flopsAnyAvg" db:"flops_any_avg"`
	MemBwAvg    *float64  `json:"memBwAvg" db:"mem_bw_avg"`
	NetBwAvg    *float64  `json:"netBwAvg" db:"net_bw_avg"`
	FileBwAvg   *float64  `json:"fileBwAvg" db:"file_bw_avg"`
	LoadAvg     *float64  `json:"loadAvg" db:"load_avg"`
	Tags        []JobTag  `json:"tags"`
}

type JobTag struct {
	ID      string `db:"id"`
	TagType string `db:"tag_type"`
	TagName string `db:"tag_name"`
}

type Cluster struct {
	ClusterID       string         `json:"clusterID"`
	ProcessorType   string         `json:"processorType"`
	SocketsPerNode  int            `json:"socketsPerNode"`
	CoresPerSocket  int            `json:"coresPerSocket"`
	ThreadsPerCore  int            `json:"threadsPerCore"`
	FlopRateScalar  int            `json:"flopRateScalar"`
	FlopRateSimd    int            `json:"flopRateSimd"`
	MemoryBandwidth int            `json:"memoryBandwidth"`
	MetricConfig    []MetricConfig `json:"metricConfig"`
}
