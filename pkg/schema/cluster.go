// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"fmt"
	"strconv"
)

type Accelerator struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Model string `json:"model"`
}

type Topology struct {
	Node         []int          `json:"node"`
	Socket       [][]int        `json:"socket"`
	MemoryDomain [][]int        `json:"memoryDomain"`
	Die          [][]*int       `json:"die,omitempty"`
	Core         [][]int        `json:"core"`
	Accelerators []*Accelerator `json:"accelerators,omitempty"`
}

type MetricValue struct {
	Unit  Unit    `json:"unit"`
	Value float64 `json:"value"`
}

type SubCluster struct {
	Name            string         `json:"name"`
	Nodes           string         `json:"nodes"`
	ProcessorType   string         `json:"processorType"`
	Topology        Topology       `json:"topology"`
	FlopRateScalar  MetricValue    `json:"flopRateScalar"`
	FlopRateSimd    MetricValue    `json:"flopRateSimd"`
	MemoryBandwidth MetricValue    `json:"memoryBandwidth"`
	MetricConfig    []MetricConfig `json:"metricConfig,omitempty"`
	Footprint       []string       `json:"footprint,omitempty"`
	EnergyFootprint []string       `json:"energyFootprint,omitempty"`
	SocketsPerNode  int            `json:"socketsPerNode"`
	CoresPerSocket  int            `json:"coresPerSocket"`
	ThreadsPerCore  int            `json:"threadsPerCore"`
}

type SubClusterConfig struct {
	Name          string  `json:"name"`
	Footprint     string  `json:"footprint,omitempty"`
	Energy        string  `json:"energy"`
	Peak          float64 `json:"peak"`
	Normal        float64 `json:"normal"`
	Caution       float64 `json:"caution"`
	Alert         float64 `json:"alert"`
	Remove        bool    `json:"remove"`
	LowerIsBetter bool    `json:"lowerIsBetter"`
}

type MetricConfig struct {
	Unit          Unit                `json:"unit"`
	Energy        string              `json:"energy"`
	Name          string              `json:"name"`
	Scope         MetricScope         `json:"scope"`
	Aggregation   string              `json:"aggregation"`
	Footprint     string              `json:"footprint,omitempty"`
	SubClusters   []*SubClusterConfig `json:"subClusters,omitempty"`
	Peak          float64             `json:"peak"`
	Caution       float64             `json:"caution"`
	Alert         float64             `json:"alert"`
	Timestep      int                 `json:"timestep"`
	Normal        float64             `json:"normal"`
	LowerIsBetter bool                `json:"lowerIsBetter"`
}

type Cluster struct {
	Name         string          `json:"name"`
	MetricConfig []*MetricConfig `json:"metricConfig"`
	SubClusters  []*SubCluster   `json:"subClusters"`
}

type ClusterSupport struct {
	Cluster     string   `json:"cluster"`
	SubClusters []string `json:"subclusters"`
}

type GlobalMetricListItem struct {
	Name         string           `json:"name"`
	Unit         Unit             `json:"unit"`
	Scope        MetricScope      `json:"scope"`
	Footprint    string           `json:"footprint,omitempty"`
	Availability []ClusterSupport `json:"availability"`
}

// Return a list of socket IDs given a list of hwthread IDs.  Even if just one
// hwthread is in that socket, add it to the list.  If no hwthreads other than
// those in the argument list are assigned to one of the sockets in the first
// return value, return true as the second value.  TODO: Optimize this, there
// must be a more efficient way/algorithm.
func (topo *Topology) GetSocketsFromHWThreads(
	hwthreads []int,
) (sockets []int, exclusive bool) {
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
	hwthreads []int,
) (cores []int, exclusive bool) {
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
	hwthreads []int,
) (memDoms []int, exclusive bool) {
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

// Temporary fix to convert back from int id to string id for accelerators
func (topo *Topology) GetAcceleratorID(id int) (string, error) {
	if id < 0 {
		fmt.Printf("ID smaller than 0!\n")
		return topo.Accelerators[0].ID, nil
	} else if id < len(topo.Accelerators) {
		return topo.Accelerators[id].ID, nil
	} else {
		return "", fmt.Errorf("index %d out of range", id)
	}
}

// Return list of hardware (string) accelerator IDs
func (topo *Topology) GetAcceleratorIDs() []string {
	accels := make([]string, 0)
	for _, accel := range topo.Accelerators {
		accels = append(accels, accel.ID)
	}
	return accels
}

// Outdated? Or: Return indices of accelerators in parent array?
func (topo *Topology) GetAcceleratorIDsAsInt() ([]int, error) {
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
