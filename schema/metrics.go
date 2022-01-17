package schema

import (
	"fmt"
	"io"
	"unsafe"
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
	MetricScopeCore     MetricScope = "core"
	MetricScopeHWThread MetricScope = "hwthread"

	MetricScopeAccelerator MetricScope = "accelerator"
)

var metricScopeGranularity map[MetricScope]int = map[MetricScope]int{
	MetricScopeNode:     10,
	MetricScopeSocket:   5,
	MetricScopeCore:     2,
	MetricScopeHWThread: 1,

	MetricScopeAccelerator: 5, // Special/Randomly choosen
}

func (e *MetricScope) LT(other MetricScope) bool {
	a := metricScopeGranularity[*e]
	b := metricScopeGranularity[other]
	return a < b
}

func (e *MetricScope) LTE(other MetricScope) bool {
	a := metricScopeGranularity[*e]
	b := metricScopeGranularity[other]
	return a <= b
}

func (e *MetricScope) Max(other MetricScope) MetricScope {
	a := metricScopeGranularity[*e]
	b := metricScopeGranularity[other]
	if a > b {
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

func (jd *JobData) Size() int {
	n := 128
	for _, scopes := range *jd {
		for _, metric := range scopes {
			if metric.StatisticsSeries != nil {
				n += len(metric.StatisticsSeries.Max)
				n += len(metric.StatisticsSeries.Mean)
				n += len(metric.StatisticsSeries.Min)
			}

			for _, series := range metric.Series {
				n += len(series.Data)
			}
		}
	}
	return n * int(unsafe.Sizeof(Float(0)))
}
