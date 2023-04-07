// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// Non-Swaggered Comment: BaseJob
// Non-Swaggered Comment: Common subset of Job and JobMeta. Use one of those, not this type directly.

type BaseJob struct {
	// The unique identifier of a job
	JobID            int64             `json:"jobId" db:"job_id" example:"123000"`
	User             string            `json:"user" db:"user" example:"abcd100h"`                                           // The unique identifier of a user
	Project          string            `json:"project" db:"project" example:"abcd200"`                                      // The unique identifier of a project
	Cluster          string            `json:"cluster" db:"cluster" example:"fritz"`                                        // The unique identifier of a cluster
	SubCluster       string            `json:"subCluster" db:"subcluster" example:"main"`                                   // The unique identifier of a sub cluster
	Partition        string            `json:"partition" db:"partition" example:"main"`                                     // The Slurm partition to which the job was submitted
	ArrayJobId       int64             `json:"arrayJobId" db:"array_job_id" example:"123000"`                               // The unique identifier of an array job
	NumNodes         int32             `json:"numNodes" db:"num_nodes" example:"2" minimum:"1"`                             // Number of nodes used (Min > 0)
	NumHWThreads     int32             `json:"numHwthreads" db:"num_hwthreads" example:"20" minimum:"1"`                    // Number of HWThreads used (Min > 0)
	NumAcc           int32             `json:"numAcc" db:"num_acc" example:"2" minimum:"1"`                                 // Number of accelerators used (Min > 0)
	Exclusive        int32             `json:"exclusive" db:"exclusive" example:"1" minimum:"0" maximum:"2"`                // Specifies how nodes are shared: 0 - Shared among multiple jobs of multiple users, 1 - Job exclusive (Default), 2 - Shared among multiple jobs of same user
	MonitoringStatus int32             `json:"monitoringStatus" db:"monitoring_status" example:"1" minimum:"0" maximum:"3"` // State of monitoring system during job run: 0 - Disabled, 1 - Running or Archiving (Default), 2 - Archiving Failed, 3 - Archiving Successfull
	SMT              int32             `json:"smt" db:"smt" example:"4"`                                                    // SMT threads used by job
	State            JobState          `json:"jobState" db:"job_state" example:"completed"`                                 // Final state of job
	Duration         int32             `json:"duration" db:"duration" example:"43200" minimum:"1"`                          // Duration of job in seconds (Min > 0)
	Walltime         int64             `json:"walltime" db:"walltime" example:"86400" minimum:"1"`                          // Requested walltime of job in seconds (Min > 0)
	Tags             []*Tag            `json:"tags"`                                                                        // List of tags
	RawResources     []byte            `json:"-" db:"resources"`                                                            // Resources used by job [As Bytes]
	Resources        []*Resource       `json:"resources"`                                                                   // Resources used by job
	RawMetaData      []byte            `json:"-" db:"meta_data"`                                                            // Additional information about the job [As Bytes]
	MetaData         map[string]string `json:"metaData"`                                                                    // Additional information about the job
}

// Non-Swaggered Comment: Job
// Non-Swaggered Comment: This type is used as the GraphQL interface and using sqlx as a table row.

// Job model
// @Description Information of a HPC job.
type Job struct {
	// The unique identifier of a job in the database
	ID int64 `json:"id" db:"id"`
	BaseJob
	StartTimeUnix    int64     `json:"-" db:"start_time" example:"1649723812"` // Start epoch time stamp in seconds
	StartTime        time.Time `json:"startTime"`                              // Start time as 'time.Time' data type
	MemUsedMax       float64   `json:"-" db:"mem_used_max"`                    // MemUsedMax as Float64
	FlopsAnyAvg      float64   `json:"-" db:"flops_any_avg"`                   // FlopsAnyAvg as Float64
	MemBwAvg         float64   `json:"-" db:"mem_bw_avg"`                      // MemBwAvg as Float64
	LoadAvg          float64   `json:"-" db:"load_avg"`                        // LoadAvg as Float64
	NetBwAvg         float64   `json:"-" db:"net_bw_avg"`                      // NetBwAvg as Float64
	NetDataVolTotal  float64   `json:"-" db:"net_data_vol_total"`              // NetDataVolTotal as Float64
	FileBwAvg        float64   `json:"-" db:"file_bw_avg"`                     // FileBwAvg as Float64
	FileDataVolTotal float64   `json:"-" db:"file_data_vol_total"`             // FileDataVolTotal as Float64
}

// Non-Swaggered Comment: JobMeta
// Non-Swaggered Comment: When reading from the database or sending data via GraphQL, the start time can be in the much more
// Non-Swaggered Comment: convenient time.Time type. In the `meta.json` files, the start time is encoded as a unix epoch timestamp.
// Non-Swaggered Comment: This is why there is this struct, which contains all fields from the regular job struct, but "overwrites"
// Non-Swaggered Comment: the StartTime field with one of type int64.
// Non-Swaggered Comment: ID *int64 `json:"id,omitempty"` >> never used in the job-archive, only available via REST-API

// JobMeta model
// @Description Meta data information of a HPC job.
type JobMeta struct {
	// The unique identifier of a job in the database
	ID *int64 `json:"id,omitempty"`
	BaseJob
	StartTime  int64                    `json:"startTime" db:"start_time" example:"1649723812" minimum:"1"` // Start epoch time stamp in seconds (Min > 0)
	Statistics map[string]JobStatistics `json:"statistics,omitempty"`                                       // Metric statistics of job
}

const (
	MonitoringStatusDisabled            int32 = 0
	MonitoringStatusRunningOrArchiving  int32 = 1
	MonitoringStatusArchivingFailed     int32 = 2
	MonitoringStatusArchivingSuccessful int32 = 3
)

var JobDefaults BaseJob = BaseJob{
	Exclusive:        1,
	MonitoringStatus: MonitoringStatusRunningOrArchiving,
}

// JobStatistics model
// @Description Specification for job metric statistics.
type JobStatistics struct {
	// Metric unit (see schema/unit.schema.json)
	Unit string  `json:"unit" example:"GHz"`
	Avg  float64 `json:"avg" example:"2500" minimum:"0"` // Job metric average
	Min  float64 `json:"min" example:"2000" minimum:"0"` // Job metric minimum
	Max  float64 `json:"max" example:"3000" minimum:"0"` // Job metric maximum
}

// Tag model
// @Description Defines a tag using name and type.
type Tag struct {
	// The unique DB identifier of a tag
	ID   int64  `json:"id" db:"id"`
	Type string `json:"type" db:"tag_type" example:"Debug"`   // Tag Type
	Name string `json:"name" db:"tag_name" example:"Testjob"` // Tag Name
}

// Resource model
// @Description A resource used by a job
type Resource struct {
	Hostname      string   `json:"hostname"`                // Name of the host (= node)
	HWThreads     []int    `json:"hwthreads,omitempty"`     // List of OS processor ids
	Accelerators  []string `json:"accelerators,omitempty"`  // List of of accelerator device ids
	Configuration string   `json:"configuration,omitempty"` // The configuration options of the node
}

type JobState string

const (
	JobStateRunning     JobState = "running"
	JobStateCompleted   JobState = "completed"
	JobStateFailed      JobState = "failed"
	JobStateCancelled   JobState = "cancelled"
	JobStateStopped     JobState = "stopped"
	JobStateTimeout     JobState = "timeout"
	JobStatePreempted   JobState = "preempted"
	JobStateOutOfMemory JobState = "out_of_memory"
)

func (e *JobState) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("SCHEMA/JOB > enums must be strings")
	}

	*e = JobState(str)
	if !e.Valid() {
		return errors.New("SCHEMA/JOB > invalid job state")
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
		e == JobStateCancelled ||
		e == JobStateStopped ||
		e == JobStateTimeout ||
		e == JobStatePreempted ||
		e == JobStateOutOfMemory
}
