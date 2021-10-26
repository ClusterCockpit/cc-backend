package schema

import (
	"fmt"
	"io"
)

// Format of `data.json` files.
type JobData map[string]*JobMetric

type JobMetric struct {
	Unit     string          `json:"unit"`
	Scope    MetricScope     `json:"scope"`
	Timestep int             `json:"timestep"`
	Series   []*MetricSeries `json:"series"`
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
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type MetricSeries struct {
	NodeID     string            `json:"node_id"`
	Statistics *MetricStatistics `json:"statistics"`
	Data       []Float           `json:"data"`
}

// Format of `meta.json` files.
type JobMeta struct {
	JobId     string   `json:"job_id"`
	UserId    string   `json:"user_id"`
	ProjectId string   `json:"project_id"`
	ClusterId string   `json:"cluster_id"`
	NumNodes  int      `json:"num_nodes"`
	JobState  string   `json:"job_state"`
	StartTime int64    `json:"start_time"`
	Duration  int64    `json:"duration"`
	Nodes     []string `json:"nodes"`
	Tags      []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"tags"`
	Statistics map[string]struct {
		Unit string  `json:"unit"`
		Avg  float64 `json:"avg"`
		Min  float64 `json:"min"`
		Max  float64 `json:"max"`
	} `json:"statistics"`
}
