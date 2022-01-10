package schema

import (
	"fmt"
	"io"
)

type JobData map[string]map[MetricScope]*JobMetric

type JobMetric struct {
	Unit             string       `json:"unit"`
	Scope            MetricScope  `json:"scope"`
	Timestep         int          `json:"timestep"`
	Series           []Series     `json:"series"`
	StatisticsSeries *StatsSeries `json:"statisticsSeries"`
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
	Mean        []Float         `json:"mean"`
	Min         []Float         `json:"min"`
	Max         []Float         `json:"max"`
	Percentiles map[int][]Float `json:"percentiles,omitempty"`
}

type MetricScope string

const (
	MetricScopeNode     MetricScope = "node"
	MetricScopeSocket   MetricScope = "socket"
	MetricScopeCpu      MetricScope = "cpu"
	MetricScopeHWThread MetricScope = "hwthread"
)

var metricScopeGranularity map[MetricScope]int = map[MetricScope]int{
	MetricScopeNode:     1,
	MetricScopeSocket:   2,
	MetricScopeCpu:      3,
	MetricScopeHWThread: 4,
}

func (e *MetricScope) MaxGranularity(other MetricScope) MetricScope {
	a := metricScopeGranularity[*e]
	b := metricScopeGranularity[other]
	if a < b {
		return *e
	}
	return other
}

func (e *MetricScope) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = MetricScope(str)
	if _, ok := metricScopeGranularity[*e]; !ok {
		return fmt.Errorf("%s is not a valid MetricScope", str)
	}
	return nil
}

func (e MetricScope) MarshalGQL(w io.Writer) {
	fmt.Fprintf(w, "\"%s\"", e)
}
