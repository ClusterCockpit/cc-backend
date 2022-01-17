package model

type Cluster struct {
	Name         string          `json:"name"`
	MetricConfig []*MetricConfig `json:"metricConfig"`
	FilterRanges *FilterRanges   `json:"filterRanges"`
	Partitions   []*Partition    `json:"partitions"`

	// NOT part of the API:
	MetricDataRepository *MetricDataRepository `json:"metricDataRepository"`
}

type MetricDataRepository struct {
	Kind  string `json:"kind"`
	Url   string `json:"url"`
	Token string `json:"token"`
}

// Return a list of socket IDs given a list of hwthread IDs.
// Even if just one hwthread is in that socket, add it to the list.
// If no hwthreads other than those in the argument list are assigned to
// one of the sockets in the first return value, return true as the second value.
// TODO: Optimize this, there must be a more efficient way/algorithm.
func (topo *Topology) GetSocketsFromHWThreads(hwthreads []int) (sockets []int, exclusive bool) {
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

// Return a list of core IDs given a list of hwthread IDs.
// Even if just one hwthread is in that core, add it to the list.
// If no hwthreads other than those in the argument list are assigned to
// one of the cores in the first return value, return true as the second value.
// TODO: Optimize this, there must be a more efficient way/algorithm.
func (topo *Topology) GetCoresFromHWThreads(hwthreads []int) (cores []int, exclusive bool) {
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
