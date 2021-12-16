package schema

import (
	"fmt"
	"io"
)

// Format of `data.json` files.
type JobData map[string]*JobMetric

type JobMetric struct {
	Unit     string          `json:"Unit"`
	Scope    MetricScope     `json:"Scope"`
	Timestep int             `json:"Timestep"`
	Series   []*MetricSeries `json:"Series"`
}

type MetricScope string

const (
	MetricScopeNode   MetricScope = "node"
	MetricScopeSocket MetricScope = "socket"
	MetricScopeCpu    MetricScope = "cpu"
)

func (e *MetricScope) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = MetricScope(str)
	if *e != "node" && *e != "socket" && *e != "cpu" {
		return fmt.Errorf("%s is not a valid MetricScope", str)
	}
	return nil
}

func (e MetricScope) MarshalGQL(w io.Writer) {
	fmt.Fprintf(w, "\"%s\"", e)
}

type MetricStatistics struct {
	Avg float64 `json:"Avg"`
	Min float64 `json:"Min"`
	Max float64 `json:"Max"`
}

type MetricSeries struct {
	Hostname   string            `json:"Hostname"`
	Id         int               `json:"Id"`
	Statistics *MetricStatistics `json:"Statistics"`
	Data       []Float           `json:"Data"`
}

type JobMetaStatistics struct {
	Unit string  `json:"Unit"`
	Avg  float64 `json:"Avg"`
	Min  float64 `json:"Min"`
	Max  float64 `json:"Max"`
}

type Accelerator struct {
	ID    int    `json:"Id"`
	Type  string `json:"Type"`
	Model string `json:"Model"`
}

type JobResource struct {
	Hostname     string        `json:"Hostname"`
	HWThreads    []int         `json:"HWThreads,omitempty"`
	Accelerators []Accelerator `json:"Accelerators,omitempty"`
}

// Format of `meta.json` files.
type JobMeta struct {
	JobId            int64          `json:"JobId"`
	User             string         `json:"User"`
	Project          string         `json:"Project"`
	Cluster          string         `json:"Cluster"`
	NumNodes         int            `json:"NumNodes"`
	NumHWThreads     int            `json:"NumHWThreads"`
	NumAcc           int            `json:"NumAcc"`
	Exclusive        int8           `json:"Exclusive"`
	MonitoringStatus int8           `json:"MonitoringStatus"`
	SMT              int8           `json:"SMT"`
	Partition        string         `json:"Partition"`
	ArrayJobId       int            `json:"ArrayJobId"`
	JobState         string         `json:"JobState"`
	StartTime        int64          `json:"StartTime"`
	Duration         int64          `json:"Duration"`
	Resources        []*JobResource `json:"Resources"`
	MetaData         string         `json:"MetaData"`
	Tags             []struct {
		Name string `json:"Name"`
		Type string `json:"Type"`
	} `json:"Tags"`
	Statistics map[string]*JobMetaStatistics `json:"Statistics"`
}
