// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-lib/schema"
)

type Node struct {
	Name            string   `json:"hostname"`
	States          []string `json:"states"`
	CpusAllocated   int      `json:"cpusAllocated"`
	CpusTotal       int      `json:"cpusTotal"`
	MemoryAllocated int      `json:"memoryAllocated"`
	MemoryTotal     int      `json:"memoryTotal"`
	GpusAllocated   int      `json:"gpusAllocated"`
	GpusTotal       int      `json:"gpusTotal"`
}

type UpdateNodeStatesRequest struct {
	Nodes   []Node `json:"nodes"`
	Cluster string `json:"cluster" example:"fritz"`
}

// this routine assumes that only one of them exists per node
func determineState(states []string) schema.NodeState {
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
// @success     200     {object} api.DefaultApiResponse "Success message"
// @failure     400     {object} api.ErrorResponse      "Bad Request"
// @failure     401     {object} api.ErrorResponse      "Unauthorized"
// @failure     403     {object} api.ErrorResponse      "Forbidden"
// @failure     500     {object} api.ErrorResponse      "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/nodestats/ [post]
func (api *RestApi) updateNodeStates(rw http.ResponseWriter, r *http.Request) {
	// Parse request body
	req := UpdateNodeStatesRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err),
			http.StatusBadRequest, rw)
		return
	}
	repo := repository.GetNodeRepository()

	for _, node := range req.Nodes {
		state := determineState(node.States)
		nodeState := schema.Node{
			TimeStamp: time.Now().Unix(), NodeState: state,
			Hostname: node.Name, Cluster: req.Cluster,
			CpusAllocated: node.CpusAllocated, CpusTotal: node.CpusTotal,
			MemoryAllocated: node.MemoryAllocated, MemoryTotal: node.MemoryTotal,
			GpusAllocated: node.GpusAllocated, GpusTotal: node.GpusTotal,
			HealthState: schema.MonitoringStateFull,
		}

		repo.InsertNodeState(&nodeState)
	}
}
