package schema

import (
	"fmt"
	"io"
)

type JobData map[string]map[string]*JobMetric

type JobMetric struct {
	Unit        string       `json:"unit"`
	Scope       MetricScope  `json:"scope"`
	Timestep    int          `json:"timestep"`
	Series      []Series     `json:"series"`
	StatsSeries *StatsSeries `json:"statisticsSeries,omitempty"`
}

type Series struct {
	Hostname   string            `json:"hostname"`
	Id         *int              `json:"id,omitempty"`
	Statistics *MetricStatistics `json:"statistics"`
	Data       []Float           `json:"data"`
}

type MetricStatistics struct {
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type StatsSeries struct {
	Mean        []Float         `json:"mean,omitempty"`
	Min         []Float         `json:"min,omitempty"`
	Max         []Float         `json:"max,omitempty"`
	Percentiles map[int][]Float `json:"percentiles,omitempty"`
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
