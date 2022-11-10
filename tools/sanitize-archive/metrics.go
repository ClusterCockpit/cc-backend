// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type JobData map[string]map[schema.MetricScope]*JobMetric

type JobMetric struct {
	Unit             string             `json:"unit"`
	Scope            schema.MetricScope `json:"scope"`
	Timestep         int                `json:"timestep"`
	Series           []Series           `json:"series"`
	StatisticsSeries *StatsSeries       `json:"statisticsSeries"`
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

// type MetricScope string

// const (
// 	MetricScopeInvalid MetricScope = "invalid_scope"

// 	MetricScopeNode         MetricScope = "node"
// 	MetricScopeSocket       MetricScope = "socket"
// 	MetricScopeMemoryDomain MetricScope = "memoryDomain"
// 	MetricScopeCore         MetricScope = "core"
// 	MetricScopeHWThread     MetricScope = "hwthread"

// 	MetricScopeAccelerator MetricScope = "accelerator"
// )

// var metricScopeGranularity map[MetricScope]int = map[MetricScope]int{
// 	MetricScopeNode:         10,
// 	MetricScopeSocket:       5,
// 	MetricScopeMemoryDomain: 3,
// 	MetricScopeCore:         2,
// 	MetricScopeHWThread:     1,

// 	MetricScopeAccelerator: 5, // Special/Randomly choosen

// 	MetricScopeInvalid: -1,
// }
