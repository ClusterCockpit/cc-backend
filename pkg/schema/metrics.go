// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"fmt"
	"io"
	"math"
	"sort"
	"unsafe"
)

type JobData map[string]map[MetricScope]*JobMetric

type JobMetric struct {
	Unit             Unit         `json:"unit"`
	Timestep         int          `json:"timestep"`
	Series           []Series     `json:"series"`
	StatisticsSeries *StatsSeries `json:"statisticsSeries,omitempty"`
}

type Series struct {
	Hostname   string           `json:"hostname"`
	Id         *string          `json:"id,omitempty"`
	Statistics MetricStatistics `json:"statistics"`
	Data       []Float          `json:"data"`
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
	MetricScopeInvalid MetricScope = "invalid_scope"

	MetricScopeNode         MetricScope = "node"
	MetricScopeSocket       MetricScope = "socket"
	MetricScopeMemoryDomain MetricScope = "memoryDomain"
	MetricScopeCore         MetricScope = "core"
	MetricScopeHWThread     MetricScope = "hwthread"

	MetricScopeAccelerator MetricScope = "accelerator"
)

var metricScopeGranularity map[MetricScope]int = map[MetricScope]int{
	MetricScopeNode:         10,
	MetricScopeSocket:       5,
	MetricScopeMemoryDomain: 3,
	MetricScopeCore:         2,
	MetricScopeHWThread:     1,

	MetricScopeAccelerator: 5, // Special/Randomly choosen

	MetricScopeInvalid: -1,
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
		return fmt.Errorf("SCHEMA/METRICS > enums must be strings")
	}

	*e = MetricScope(str)
	if !e.Valid() {
		return fmt.Errorf("SCHEMA/METRICS > %s is not a valid MetricScope", str)
	}
	return nil
}

func (e MetricScope) MarshalGQL(w io.Writer) {
	fmt.Fprintf(w, "\"%s\"", e)
}

func (e MetricScope) Valid() bool {
	gran, ok := metricScopeGranularity[e]
	return ok && gran > 0
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

const smooth bool = false

func (jm *JobMetric) AddStatisticsSeries() {
	if jm.StatisticsSeries != nil || len(jm.Series) < 4 {
		return
	}

	n, m := 0, len(jm.Series[0].Data)
	for _, series := range jm.Series {
		if len(series.Data) > n {
			n = len(series.Data)
		}
		if len(series.Data) < m {
			m = len(series.Data)
		}
	}

	min, mean, max := make([]Float, n), make([]Float, n), make([]Float, n)
	i := 0
	for ; i < m; i++ {
		smin, ssum, smax := math.MaxFloat32, 0.0, -math.MaxFloat32
		notnan := 0
		for j := 0; j < len(jm.Series); j++ {
			x := float64(jm.Series[j].Data[i])
			if math.IsNaN(x) {
				continue
			}

			notnan += 1
			ssum += x
			smin = math.Min(smin, x)
			smax = math.Max(smax, x)
		}

		if notnan < 3 {
			min[i] = NaN
			mean[i] = NaN
			max[i] = NaN
		} else {
			min[i] = Float(smin)
			mean[i] = Float(ssum / float64(notnan))
			max[i] = Float(smax)
		}
	}

	for ; i < n; i++ {
		min[i] = NaN
		mean[i] = NaN
		max[i] = NaN
	}

	if smooth {
		for i := 2; i < len(mean)-2; i++ {
			if min[i].IsNaN() {
				continue
			}

			min[i] = (min[i-2] + min[i-1] + min[i] + min[i+1] + min[i+2]) / 5
			max[i] = (max[i-2] + max[i-1] + max[i] + max[i+1] + max[i+2]) / 5
			mean[i] = (mean[i-2] + mean[i-1] + mean[i] + mean[i+1] + mean[i+2]) / 5
		}
	}

	jm.StatisticsSeries = &StatsSeries{Mean: mean, Min: min, Max: max}
}

func (jd *JobData) AddNodeScope(metric string) bool {
	scopes, ok := (*jd)[metric]
	if !ok {
		return false
	}

	var maxScope MetricScope = MetricScopeInvalid
	for scope := range scopes {
		maxScope = maxScope.Max(scope)
	}

	if maxScope == MetricScopeInvalid || maxScope == MetricScopeNode {
		return false
	}

	jm := scopes[maxScope]
	hosts := make(map[string][]Series, 32)
	for _, series := range jm.Series {
		hosts[series.Hostname] = append(hosts[series.Hostname], series)
	}

	nodeJm := &JobMetric{
		Unit:     jm.Unit,
		Timestep: jm.Timestep,
		Series:   make([]Series, 0, len(hosts)),
	}
	for hostname, series := range hosts {
		min, sum, max := math.MaxFloat32, 0.0, -math.MaxFloat32
		for _, series := range series {
			sum += series.Statistics.Avg
			min = math.Min(min, series.Statistics.Min)
			max = math.Max(max, series.Statistics.Max)
		}

		n, m := 0, len(jm.Series[0].Data)
		for _, series := range jm.Series {
			if len(series.Data) > n {
				n = len(series.Data)
			}
			if len(series.Data) < m {
				m = len(series.Data)
			}
		}

		i, data := 0, make([]Float, len(series[0].Data))
		for ; i < m; i++ {
			x := Float(0.0)
			for _, series := range jm.Series {
				x += series.Data[i]
			}
			data[i] = x
		}

		for ; i < n; i++ {
			data[i] = NaN
		}

		nodeJm.Series = append(nodeJm.Series, Series{
			Hostname:   hostname,
			Statistics: MetricStatistics{Min: min, Avg: sum / float64(len(series)), Max: max},
			Data:       data,
		})
	}

	scopes[MetricScopeNode] = nodeJm
	return true
}

func (jm *JobMetric) AddPercentiles(ps []int) bool {
	if jm.StatisticsSeries == nil {
		jm.AddStatisticsSeries()
	}

	if len(jm.Series) < 3 {
		return false
	}

	if jm.StatisticsSeries.Percentiles == nil {
		jm.StatisticsSeries.Percentiles = make(map[int][]Float, len(ps))
	}

	n := 0
	for _, series := range jm.Series {
		if len(series.Data) > n {
			n = len(series.Data)
		}
	}

	data := make([][]float64, n)
	for i := 0; i < n; i++ {
		vals := make([]float64, 0, len(jm.Series))
		for _, series := range jm.Series {
			if i < len(series.Data) {
				vals = append(vals, float64(series.Data[i]))
			}
		}

		sort.Float64s(vals)
		data[i] = vals
	}

	for _, p := range ps {
		if p < 1 || p > 99 {
			panic("SCHEMA/METRICS > invalid percentile")
		}

		if _, ok := jm.StatisticsSeries.Percentiles[p]; ok {
			continue
		}

		percentiles := make([]Float, n)
		for i := 0; i < n; i++ {
			sorted := data[i]
			percentiles[i] = Float(sorted[(len(sorted)*p)/100])
		}

		jm.StatisticsSeries.Percentiles[p] = percentiles
	}

	return true
}
