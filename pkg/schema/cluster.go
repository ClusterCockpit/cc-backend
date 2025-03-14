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

	// Cache maps for faster lookups
	hwthreadToSocket       map[int][]int
	hwthreadToCore         map[int][]int
	hwthreadToMemoryDomain map[int][]int
	coreToSocket           map[int][]int
	memoryDomainToSocket   map[int]int // New: Direct mapping from memory domain to socket
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

// InitTopologyMaps initializes the topology mapping caches
func (topo *Topology) InitTopologyMaps() {
	// Initialize maps
	topo.hwthreadToSocket = make(map[int][]int)
	topo.hwthreadToCore = make(map[int][]int)
	topo.hwthreadToMemoryDomain = make(map[int][]int)
	topo.coreToSocket = make(map[int][]int)
	topo.memoryDomainToSocket = make(map[int]int)

	// Build hwthread to socket mapping
	for socketID, hwthreads := range topo.Socket {
		for _, hwthread := range hwthreads {
			topo.hwthreadToSocket[hwthread] = append(topo.hwthreadToSocket[hwthread], socketID)
		}
	}

	// Build hwthread to core mapping
	for coreID, hwthreads := range topo.Core {
		for _, hwthread := range hwthreads {
			topo.hwthreadToCore[hwthread] = append(topo.hwthreadToCore[hwthread], coreID)
		}
	}

	// Build hwthread to memory domain mapping
	for memDomID, hwthreads := range topo.MemoryDomain {
		for _, hwthread := range hwthreads {
			topo.hwthreadToMemoryDomain[hwthread] = append(topo.hwthreadToMemoryDomain[hwthread], memDomID)
		}
	}

	// Build core to socket mapping
	for coreID, hwthreads := range topo.Core {
		socketSet := make(map[int]struct{})
		for _, hwthread := range hwthreads {
			for socketID := range topo.hwthreadToSocket[hwthread] {
				socketSet[socketID] = struct{}{}
			}
		}
		topo.coreToSocket[coreID] = make([]int, 0, len(socketSet))
		for socketID := range socketSet {
			topo.coreToSocket[coreID] = append(topo.coreToSocket[coreID], socketID)
		}
	}

	// Build memory domain to socket mapping
	for memDomID, hwthreads := range topo.MemoryDomain {
		if len(hwthreads) > 0 {
			// Use the first hwthread to determine the socket
			if socketIDs, ok := topo.hwthreadToSocket[hwthreads[0]]; ok && len(socketIDs) > 0 {
				topo.memoryDomainToSocket[memDomID] = socketIDs[0]
			}
		}
	}
}

// EnsureTopologyMaps ensures that the topology maps are initialized
func (topo *Topology) EnsureTopologyMaps() {
	if topo.hwthreadToSocket == nil {
		topo.InitTopologyMaps()
	}
}

func (topo *Topology) GetSocketsFromHWThreads(
	hwthreads []int,
) (sockets []int, exclusive bool) {
	topo.EnsureTopologyMaps()

	socketsMap := make(map[int]int)
	for _, hwthread := range hwthreads {
		for _, socketID := range topo.hwthreadToSocket[hwthread] {
			socketsMap[socketID]++
		}
	}

	exclusive = true
	sockets = make([]int, 0, len(socketsMap))
	for socket, count := range socketsMap {
		sockets = append(sockets, socket)
		// Check if all hwthreads in this socket are in our input list
		exclusive = exclusive && count == len(topo.Socket[socket])
	}

	return sockets, exclusive
}

func (topo *Topology) GetSocketsFromCores(
	cores []int,
) (sockets []int, exclusive bool) {
	topo.EnsureTopologyMaps()

	socketsMap := make(map[int]int)
	for _, core := range cores {
		for _, socketID := range topo.coreToSocket[core] {
			socketsMap[socketID]++
		}
	}

	exclusive = true
	sockets = make([]int, 0, len(socketsMap))
	for socket, count := range socketsMap {
		sockets = append(sockets, socket)
		// Count total cores in this socket
		totalCoresInSocket := 0
		for _, hwthreads := range topo.Core {
			for _, hwthread := range hwthreads {
				for _, sID := range topo.hwthreadToSocket[hwthread] {
					if sID == socket {
						totalCoresInSocket++
						break
					}
				}
			}
		}
		exclusive = exclusive && count == totalCoresInSocket
	}

	return sockets, exclusive
}

func (topo *Topology) GetCoresFromHWThreads(
	hwthreads []int,
) (cores []int, exclusive bool) {
	topo.EnsureTopologyMaps()

	coresMap := make(map[int]int)
	for _, hwthread := range hwthreads {
		for _, coreID := range topo.hwthreadToCore[hwthread] {
			coresMap[coreID]++
		}
	}

	exclusive = true
	cores = make([]int, 0, len(coresMap))
	for core, count := range coresMap {
		cores = append(cores, core)
		// Check if all hwthreads in this core are in our input list
		exclusive = exclusive && count == len(topo.Core[core])
	}

	return cores, exclusive
}

func (topo *Topology) GetMemoryDomainsFromHWThreads(
	hwthreads []int,
) (memDoms []int, exclusive bool) {
	topo.EnsureTopologyMaps()

	memDomsMap := make(map[int]int)
	for _, hwthread := range hwthreads {
		for _, memDomID := range topo.hwthreadToMemoryDomain[hwthread] {
			memDomsMap[memDomID]++
		}
	}

	exclusive = true
	memDoms = make([]int, 0, len(memDomsMap))
	for memDom, count := range memDomsMap {
		memDoms = append(memDoms, memDom)
		// Check if all hwthreads in this memory domain are in our input list
		exclusive = exclusive && count == len(topo.MemoryDomain[memDom])
	}

	return memDoms, exclusive
}

// GetMemoryDomainsBySocket can now use the direct mapping
func (topo *Topology) GetMemoryDomainsBySocket(domainIDs []int) (map[int][]int, error) {
	socketToDomains := make(map[int][]int)
	for _, domainID := range domainIDs {
		if domainID < 0 || domainID >= len(topo.MemoryDomain) || len(topo.MemoryDomain[domainID]) == 0 {
			return nil, fmt.Errorf("MemoryDomain %d is invalid or empty", domainID)
		}

		socketID, ok := topo.memoryDomainToSocket[domainID]
		if !ok {
			return nil, fmt.Errorf("MemoryDomain %d could not be assigned to any socket", domainID)
		}

		socketToDomains[socketID] = append(socketToDomains[socketID], domainID)
	}

	return socketToDomains, nil
}

// GetAcceleratorID converts a numeric ID to the corresponding Accelerator ID as a string.
// This is useful when accelerators are stored in arrays and accessed by index.
func (topo *Topology) GetAcceleratorID(id int) (string, error) {
	if id < 0 {
		return "", fmt.Errorf("accelerator ID %d is negative", id)
	}

	if id >= len(topo.Accelerators) {
		return "", fmt.Errorf("accelerator index %d out of valid range (max: %d)",
			id, len(topo.Accelerators)-1)
	}

	return topo.Accelerators[id].ID, nil
}

// GetAcceleratorIDs returns a list of all Accelerator IDs (as strings).
// Capacity is pre-allocated to improve efficiency.
func (topo *Topology) GetAcceleratorIDs() []string {
	if len(topo.Accelerators) == 0 {
		return []string{}
	}

	accels := make([]string, 0, len(topo.Accelerators))
	for _, accel := range topo.Accelerators {
		accels = append(accels, accel.ID)
	}
	return accels
}

// GetAcceleratorIDsAsInt converts all Accelerator IDs to integer values.
// This function can fail if the IDs cannot be interpreted as numbers.
// Capacity is pre-allocated to improve efficiency.
func (topo *Topology) GetAcceleratorIDsAsInt() ([]int, error) {
	if len(topo.Accelerators) == 0 {
		return []int{}, nil
	}

	accels := make([]int, 0, len(topo.Accelerators))
	for i, accel := range topo.Accelerators {
		id, err := strconv.Atoi(accel.ID)
		if err != nil {
			return nil, fmt.Errorf("accelerator ID at position %d (%s) cannot be converted to a number: %w",
				i, accel.ID, err)
		}
		accels = append(accels, id)
	}
	return accels, nil
}
