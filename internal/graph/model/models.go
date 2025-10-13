// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"github.com/ClusterCockpit/cc-lib/schema"
)

type Node struct {
	ID              int64
	Hostname        string                 `json:"hostname"`
	Cluster         string                 `json:"cluster"`
	SubCluster      string                 `json:"subCluster"`
	RunningJobs     int                    `json:"jobsRunning"`
	CpusAllocated   int                    `json:"cpusAllocated"`
	MemoryAllocated int                    `json:"memoryAllocated"`
	GpusAllocated   int                    `json:"gpusAllocated"`
	NodeState       schema.NodeState       `json:"nodeState"`
	HealthState     schema.MonitoringState `json:"healthState"`
}
