// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

// type Accelerator struct {
// 	ID    string `json:"id"`
// 	Type  string `json:"type"`
// 	Model string `json:"model"`
// }

// type Topology struct {
// 	Node         []int          `json:"node"`
// 	Socket       [][]int        `json:"socket"`
// 	MemoryDomain [][]int        `json:"memoryDomain"`
// 	Die          [][]int        `json:"die"`
// 	Core         [][]int        `json:"core"`
// 	Accelerators []*Accelerator `json:"accelerators"`
// }

type SubCluster struct {
	Name            string           `json:"name"`
	Nodes           string           `json:"nodes"`
	NumberOfNodes   int              `json:"numberOfNodes"`
	ProcessorType   string           `json:"processorType"`
	SocketsPerNode  int              `json:"socketsPerNode"`
	CoresPerSocket  int              `json:"coresPerSocket"`
	ThreadsPerCore  int              `json:"threadsPerCore"`
	FlopRateScalar  int              `json:"flopRateScalar"`
	FlopRateSimd    int              `json:"flopRateSimd"`
	MemoryBandwidth int              `json:"memoryBandwidth"`
	Topology        *schema.Topology `json:"topology"`
}

// type SubClusterConfig struct {
// 	Name    string  `json:"name"`
// 	Peak    float64 `json:"peak"`
// 	Normal  float64 `json:"normal"`
// 	Caution float64 `json:"caution"`
// 	Alert   float64 `json:"alert"`
// }

type MetricConfig struct {
	Name        string                     `json:"name"`
	Unit        string                     `json:"unit"`
	Scope       schema.MetricScope         `json:"scope"`
	Aggregation string                     `json:"aggregation"`
	Timestep    int                        `json:"timestep"`
	Peak        float64                    `json:"peak"`
	Normal      float64                    `json:"normal"`
	Caution     float64                    `json:"caution"`
	Alert       float64                    `json:"alert"`
	SubClusters []*schema.SubClusterConfig `json:"subClusters"`
}

type Cluster struct {
	Name         string          `json:"name"`
	MetricConfig []*MetricConfig `json:"metricConfig"`
	SubClusters  []*SubCluster   `json:"subClusters"`
}
