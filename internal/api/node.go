// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"fmt"
	"net/http"
)

type Node struct {
	Name   string   `json:"hostname"`
	States []string `json:"states"`
}

// updateNodeStatesRequest model
type UpdateNodeStatesRequest struct {
	Nodes   []Node `json:"nodes"`
	Cluster string `json:"cluster" example:"fritz"`
}

func (api *RestApi) updateNodeStates(rw http.ResponseWriter, r *http.Request) {
	// Parse request body
	req := UpdateNodeStatesRequest{}
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("parsing request body failed: %w", err), http.StatusBadRequest, rw)
		return
	}
}
