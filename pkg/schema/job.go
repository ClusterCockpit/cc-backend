// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"errors"
	"fmt"
	"io"
)

// Job struct type
//
// This type is used as the GraphQL interface and using sqlx as a table row.
//
// Job model
// @Description Information of a HPC job.
type Job struct {
	Cluster            string                   `json:"cluster" db:"cluster" example:"fritz"`
	SubCluster         string                   `json:"subCluster" db:"subcluster" example:"main"`
	Partition          string                   `json:"partition,omitempty" db:"cluster_partition" example:"main"`
	Project            string                   `json:"project" db:"project" example:"abcd200"`
	User               string                   `json:"user" db:"hpc_user" example:"abcd100h"`
	State              JobState                 `json:"jobState" db:"job_state" example:"completed" enums:"completed,failed,cancelled,stopped,timeout,out_of_memory"`
	Tags               []*Tag                   `json:"tags,omitempty"`
	RawEnergyFootprint []byte                   `json:"-" db:"energy_footprint"`
	RawFootprint       []byte                   `json:"-" db:"footprint"`
	RawMetaData        []byte                   `json:"-" db:"meta_data"`
	RawResources       []byte                   `json:"-" db:"resources"`
	Resources          []*Resource              `json:"resources"`
	EnergyFootprint    map[string]float64       `json:"energyFootprint"`
	Footprint          map[string]float64       `json:"footprint"`
	MetaData           map[string]string        `json:"metaData"`
	ConcurrentJobs     JobLinkResultList        `json:"concurrentJobs"`
	Energy             float64                  `json:"energy" db:"energy"`
	ArrayJobId         int64                    `json:"arrayJobId,omitempty" db:"array_job_id" example:"123000"`
	Walltime           int64                    `json:"walltime,omitempty" db:"walltime" example:"86400" minimum:"1"`
	JobID              int64                    `json:"jobId" db:"job_id" example:"123000"`
	Duration           int32                    `json:"duration" db:"duration" example:"43200" minimum:"1"`
	SMT                int32                    `json:"smt,omitempty" db:"smt" example:"4"`
	MonitoringStatus   int32                    `json:"monitoringStatus,omitempty" db:"monitoring_status" example:"1" minimum:"0" maximum:"3"`
	Exclusive          int32                    `json:"exclusive" db:"exclusive" example:"1" minimum:"0" maximum:"2"`
	NumAcc             int32                    `json:"numAcc,omitempty" db:"num_acc" example:"2" minimum:"1"`
	NumHWThreads       int32                    `json:"numHwthreads,omitempty" db:"num_hwthreads" example:"20" minimum:"1"`
	NumNodes           int32                    `json:"numNodes" db:"num_nodes" example:"2" minimum:"1"`
	Statistics         map[string]JobStatistics `json:"statistics"`
	ID                 *int64                   `json:"id,omitempty" db:"id"`
	StartTime          int64                    `json:"startTime" db:"start_time" example:"1649723812"`
}

//	JobMeta struct type
//
//	When reading from the database or sending data via GraphQL, the start time
//	can be in the much more convenient time.Time type. In the `meta.json`
//	files, the start time is encoded as a unix epoch timestamp. This is why
//	there is this struct, which contains all fields from the regular job
//	struct, but "overwrites" the StartTime field with one of type int64. ID
//	*int64 `json:"id,omitempty"` >> never used in the job-archive, only
//	available via REST-API
//
// JobMeta model
// @Description Meta data information of a HPC job.
// type JobMeta struct {
// 	ID         *int64                   `json:"id,omitempty"`
// 	BaseJob
// 	Statistics map[string]JobStatistics `json:"statistics"`
// 	StartTime int64 `json:"startTime" db:"start_time" example:"1649723812" minimum:"1"`
// }

type JobLink struct {
	ID    int64 `json:"id"`
	JobID int64 `json:"jobId"`
}

type JobLinkResultList struct {
	Items []*JobLink `json:"items"`
	Count int        `json:"count"`
}

const (
	MonitoringStatusDisabled            int32 = 0
	MonitoringStatusRunningOrArchiving  int32 = 1
	MonitoringStatusArchivingFailed     int32 = 2
	MonitoringStatusArchivingSuccessful int32 = 3
)

// var JobDefaults Job = Job{
// 	Exclusive:        1,
// 	MonitoringStatus: MonitoringStatusRunningOrArchiving,
// }

type Unit struct {
	Base   string `json:"base"`
	Prefix string `json:"prefix,omitempty"`
}

// JobStatistics model
// @Description Specification for job metric statistics.
type JobStatistics struct {
	Unit Unit    `json:"unit"`
	Avg  float64 `json:"avg" example:"2500" minimum:"0"` // Job metric average
	Min  float64 `json:"min" example:"2000" minimum:"0"` // Job metric minimum
	Max  float64 `json:"max" example:"3000" minimum:"0"` // Job metric maximum
}

// Tag model
// @Description Defines a tag using name and type.
type Tag struct {
	Type  string `json:"type" db:"tag_type" example:"Debug"`
	Name  string `json:"name" db:"tag_name" example:"Testjob"`
	Scope string `json:"scope" db:"tag_scope" example:"global"`
	ID    int64  `json:"id" db:"id"`
}

// Resource model
// @Description A resource used by a job
type Resource struct {
	Hostname      string   `json:"hostname"`
	Configuration string   `json:"configuration,omitempty"`
	HWThreads     []int    `json:"hwthreads,omitempty"`
	Accelerators  []string `json:"accelerators,omitempty"`
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

func (j Job) GoString() string {
	return fmt.Sprintf("Job{ID:%d, StartTime:%d, JobID:%v, BaseJob:%v}",
		j.ID, j.StartTime, j.JobID, j)
}

func (e *JobState) UnmarshalGQL(v any) error {
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
