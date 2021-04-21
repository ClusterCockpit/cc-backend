package model

import (
	"time"
)

type Job struct {
	ID           string    `json:"id"`
	JobID        string    `json:"jobId" db:"job_id"`
	UserID       string    `json:"userId" db:"user_id"`
	ProjectID    string    `json:"projectId" db:"project_id"`
	ClusterID    string    `json:"clusterId" db:"cluster_id"`
	StartTime    time.Time `json:"startTime" db:"start_time"`
	Duration     int       `json:"duration" db:"duration"`
	Walltime     *int      `json:"walltime" db:"walltime"`
	Jobstate     *string   `json:"jobstate" db:"job_state"`
	NumNodes     int       `json:"numNodes" db:"num_nodes"`
	NodeList     string    `json:"nodelist" db:"node_list"`
	HasProfile   bool      `json:"hasProfile" db:"has_profile"`
	MemUsed_max  *float64  `json:"memUsedMax" db:"mem_used_max"`
	FlopsAny_avg *float64  `json:"flopsAnyAvg" db:"flops_any_avg"`
	MemBw_avg    *float64  `json:"memBwAvg" db:"mem_bw_avg"`
	NetBw_avg    *float64  `json:"netBwAvg" db:"net_bw_avg"`
	FileBw_avg   *float64  `json:"fileBwAvg" db:"file_bw_avg"`
	Tags         []JobTag  `json:"tags"`
}

type JobTag struct {
	ID      string `db:"id"`
	TagType string `db:"tag_type"`
	TagName string `db:"tag_name"`
}
