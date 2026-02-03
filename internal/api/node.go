// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type UpdateNodeStatesRequest struct {
	Nodes   []schema.NodePayload `json:"nodes"`
	Cluster string               `json:"cluster" example:"fritz"`
}

// metricListToNames converts a map of metric configurations to a list of metric names
func metricListToNames(metricList map[string]*schema.Metric) []string {
	names := make([]string, 0, len(metricList))
	for name := range metricList {
		names = append(names, name)
	}
	return names
}

// this routine assumes that only one of them exists per node
func determineState(states []string) schema.SchedulerState {
	for _, state := range states {
		switch strings.ToLower(state) {
		case "allocated":
			return schema.NodeStateAllocated
		case "reserved":
			return schema.NodeStateReserved
		case "idle":
			return schema.NodeStateIdle
		case "down":
			return schema.NodeStateDown
		case "mixed":
			return schema.NodeStateMixed
		}
	}

	return schema.NodeStateUnknown
}

// updateNodeStates godoc
// @summary     Deliver updated Slurm node states
// @tags Nodestates
// @description Returns a JSON-encoded list of users.
// @description Required query-parameter defines if all users or only users with additional special roles are returned.
// @produce     json
// @param       request body UpdateNodeStatesRequest true "Request body containing nodes and their states"
// @success     200     {object} api.DefaultAPIResponse "Success message"
// @failure     400     {object} api.ErrorResponse      "Bad Request"
// @failure     401     {object} api.ErrorResponse      "Unauthorized"
// @failure     403     {object} api.ErrorResponse      "Forbidden"
// @failure     500     {object} api.ErrorResponse      "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/nodestats/ [post]
func (api *RestAPI) updateNodeStates(rw http.ResponseWriter, r *http.Request) {
	// Parse request body
	req := UpdateNodeStatesRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err),
			http.StatusBadRequest, rw)
		return
	}
	requestReceived := time.Now().Unix()
	repo := repository.GetNodeRepository()
	ms := metricstore.GetMemoryStore()

	m := make(map[string][]string)
	healthStates := make(map[string]metricstore.NodeHealthState)

	for _, node := range req.Nodes {
		if sc, err := archive.GetSubClusterByNode(req.Cluster, node.Hostname); err == nil {
			m[sc] = append(m[sc], node.Hostname)
		}
	}

	for sc, nl := range m {
		if sc != "" {
			metricList := archive.GetMetricConfigSubCluster(req.Cluster, sc)
			metricNames := metricListToNames(metricList)
			if states, err := ms.HealthCheckAlt(req.Cluster, nl, metricNames); err == nil {
				maps.Copy(healthStates, states)
			}
		}
	}

	for _, node := range req.Nodes {
		state := determineState(node.States)
		healthState := schema.MonitoringStateFull
		if hs, ok := healthStates[node.Hostname]; ok {
			healthState = hs.Status
		}
		nodeState := schema.NodeStateDB{
			TimeStamp:       requestReceived,
			NodeState:       state,
			CpusAllocated:   node.CpusAllocated,
			MemoryAllocated: node.MemoryAllocated,
			GpusAllocated:   node.GpusAllocated,
			HealthState:     healthState,
			JobsRunning:     node.JobsRunning,
		}

		repo.UpdateNodeState(node.Hostname, req.Cluster, &nodeState)
	}
}
