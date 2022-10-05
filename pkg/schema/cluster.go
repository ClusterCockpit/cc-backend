// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import "strconv"

type Accelerator struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Model string `json:"model"`
}

type Topology struct {
	Node         []int          `json:"node"`
	Socket       [][]int        `json:"socket"`
	MemoryDomain [][]int        `json:"memoryDomain"`
	Die          [][]int        `json:"die"`
	Core         [][]int        `json:"core"`
	Accelerators []*Accelerator `json:"accelerators"`
}

type SubCluster struct {
	Name            string    `json:"name"`
	Nodes           string    `json:"nodes"`
	NumberOfNodes   int       `json:"numberOfNodes"`
	ProcessorType   string    `json:"processorType"`
	SocketsPerNode  int       `json:"socketsPerNode"`
	CoresPerSocket  int       `json:"coresPerSocket"`
	ThreadsPerCore  int       `json:"threadsPerCore"`
	FlopRateScalar  int       `json:"flopRateScalar"`
	FlopRateSimd    int       `json:"flopRateSimd"`
	MemoryBandwidth int       `json:"memoryBandwidth"`
	Topology        *Topology `json:"topology"`
}

type SubClusterConfig struct {
	Name    string  `json:"name"`
	Peak    float64 `json:"peak"`
	Normal  float64 `json:"normal"`
	Caution float64 `json:"caution"`
	Alert   float64 `json:"alert"`
}

type MetricConfig struct {
	Name        string              `json:"name"`
	Unit        Unit                `json:"unit"`
	Scope       MetricScope         `json:"scope"`
	Aggregation *string             `json:"aggregation"`
	Timestep    int                 `json:"timestep"`
	Peak        *float64            `json:"peak"`
	Normal      *float64            `json:"normal"`
	Caution     *float64            `json:"caution"`
	Alert       *float64            `json:"alert"`
	SubClusters []*SubClusterConfig `json:"subClusters"`
}

type Cluster struct {
	Name         string          `json:"name"`
	MetricConfig []*MetricConfig `json:"metricConfig"`
	SubClusters  []*SubCluster   `json:"subClusters"`
}

// Return a list of socket IDs given a list of hwthread IDs.  Even if just one
// hwthread is in that socket, add it to the list.  If no hwthreads other than
// those in the argument list are assigned to one of the sockets in the first
// return value, return true as the second value.  TODO: Optimize this, there
// must be a more efficient way/algorithm.
func (topo *Topology) GetSocketsFromHWThreads(
	hwthreads []int) (sockets []int, exclusive bool) {

	socketsMap := map[int]int{}
	for _, hwthread := range hwthreads {
		for socket, hwthreadsInSocket := range topo.Socket {
			for _, hwthreadInSocket := range hwthreadsInSocket {
				if hwthread == hwthreadInSocket {
					socketsMap[socket] += 1
				}
			}
		}
	}

	exclusive = true
	hwthreadsPerSocket := len(topo.Node) / len(topo.Socket)
	sockets = make([]int, 0, len(socketsMap))
	for socket, count := range socketsMap {
		sockets = append(sockets, socket)
		exclusive = exclusive && count == hwthreadsPerSocket
	}

	return sockets, exclusive
}

// Return a list of core IDs given a list of hwthread IDs.  Even if just one
// hwthread is in that core, add it to the list.  If no hwthreads other than
// those in the argument list are assigned to one of the cores in the first
// return value, return true as the second value.  TODO: Optimize this, there
// must be a more efficient way/algorithm.
func (topo *Topology) GetCoresFromHWThreads(
	hwthreads []int) (cores []int, exclusive bool) {

	coresMap := map[int]int{}
	for _, hwthread := range hwthreads {
		for core, hwthreadsInCore := range topo.Core {
			for _, hwthreadInCore := range hwthreadsInCore {
				if hwthread == hwthreadInCore {
					coresMap[core] += 1
				}
			}
		}
	}

	exclusive = true
	hwthreadsPerCore := len(topo.Node) / len(topo.Core)
	cores = make([]int, 0, len(coresMap))
	for core, count := range coresMap {
		cores = append(cores, core)
		exclusive = exclusive && count == hwthreadsPerCore
	}

	return cores, exclusive
}

// Return a list of memory domain IDs given a list of hwthread IDs.  Even if
// just one hwthread is in that memory domain, add it to the list.  If no
// hwthreads other than those in the argument list are assigned to one of the
// memory domains in the first return value, return true as the second value.
// TODO: Optimize this, there must be a more efficient way/algorithm.
func (topo *Topology) GetMemoryDomainsFromHWThreads(
	hwthreads []int) (memDoms []int, exclusive bool) {

	memDomsMap := map[int]int{}
	for _, hwthread := range hwthreads {
		for memDom, hwthreadsInmemDom := range topo.MemoryDomain {
			for _, hwthreadInmemDom := range hwthreadsInmemDom {
				if hwthread == hwthreadInmemDom {
					memDomsMap[memDom] += 1
				}
			}
		}
	}

	exclusive = true
	hwthreadsPermemDom := len(topo.Node) / len(topo.MemoryDomain)
	memDoms = make([]int, 0, len(memDomsMap))
	for memDom, count := range memDomsMap {
		memDoms = append(memDoms, memDom)
		exclusive = exclusive && count == hwthreadsPermemDom
	}

	return memDoms, exclusive
}

func (topo *Topology) GetAcceleratorIDs() ([]int, error) {
	accels := make([]int, 0)
	for _, accel := range topo.Accelerators {
		id, err := strconv.Atoi(accel.ID)
		if err != nil {
			return nil, err
		}
		accels = append(accels, id)
	}
	return accels, nil
}

func (topo *Topology) GetAcceleratorIndex(id string) (int, bool) {
	for idx, accel := range topo.Accelerators {
		if accel.ID == id {
			return idx, true
		}
	}
	return -1, false
}
