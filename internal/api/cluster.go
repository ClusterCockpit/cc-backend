// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// GetClustersAPIResponse model
type GetClustersAPIResponse struct {
	Clusters []*schema.Cluster `json:"clusters"` // Array of clusters
}

// getClusters godoc
// @summary     Lists all cluster configs
// @tags Cluster query
// @description Get a list of all cluster configs. Specific cluster can be requested using query parameter.
// @produce     json
// @param       cluster        query    string            false "Job Cluster"
// @success     200            {object} api.GetClustersAPIResponse  "Array of clusters"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401            {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/clusters/ [get]
func (api *RestAPI) getClusters(rw http.ResponseWriter, r *http.Request) {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleApi) {

		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleApi)), http.StatusForbidden, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()

	var clusters []*schema.Cluster

	if r.URL.Query().Has("cluster") {
		name := r.URL.Query().Get("cluster")
		cluster := archive.GetCluster(name)
		if cluster == nil {
			handleError(fmt.Errorf("unknown cluster: %s", name), http.StatusBadRequest, rw)
			return
		}
		clusters = append(clusters, cluster)
	} else {
		clusters = archive.Clusters
	}

	payload := GetClustersAPIResponse{
		Clusters: clusters,
	}

	if err := json.NewEncoder(bw).Encode(payload); err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}
}
