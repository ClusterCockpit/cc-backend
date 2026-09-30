// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestApiLiveFootprint(t *testing.T) {
	restapi := setup(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		metricstore.TestLoadStatsCallback = nil
		metricdispatch.ResetLiveFootprintCache()
	})
	metricdispatch.ResetLiveFootprintCache()
	config.Keys.ShortRunningJobsDuration = 300

	// Make load_one a footprint metric of testcluster/sc1.
	sc, err := archive.GetSubCluster("testcluster", "sc1")
	require.NoError(t, err)
	for _, mc := range sc.MetricConfig {
		if mc.Name == "load_one" {
			mc.Footprint = "avg"
		}
	}
	sc.Footprint = []string{"load_one"}

	metricstore.TestLoadStatsCallback = func(job *schema.Job, _ []string, _ map[string]bool, _ context.Context) (map[string]map[string]schema.MetricStatistics, error) {
		return map[string]map[string]schema.MetricStatistics{
			"load_one": {job.Resources[0].Hostname: {Avg: 3.5, Min: 1, Max: 6}},
		}, nil
	}

	repo := repository.GetJobRepository()
	insert := func(jobID int64, host string, runFor time.Duration) int64 {
		job := &schema.Job{
			JobID: jobID, User: "testuser", Project: "testproj", Cluster: "testcluster", SubCluster: "sc1",
			Partition: "default", NumNodes: 1, NumHWThreads: 8, Shared: "none", SMT: 1, Walltime: 86400,
			MonitoringStatus: schema.MonitoringStatusRunningOrArchiving, State: schema.JobStateRunning,
			StartTime: time.Now().Add(-runFor).Unix(), Duration: 1,
			Resources: []*schema.Resource{{Hostname: host, HWThreads: []int{0, 1, 2, 3, 4, 5, 6, 7}}},
		}
		job.RawResources, _ = json.Marshal(job.Resources)
		job.RawFootprint, job.RawEnergyFootprint, job.RawMetaData = []byte("null"), []byte("null"), []byte("{}")
		id, err := repo.InsertJobDirect(job)
		require.NoError(t, err)
		return id
	}
	longID := insert(660001, "host123", time.Hour)
	shortID := insert(660002, "host124", time.Minute)

	r := chi.NewRouter()
	restapi.MountAPIRoutes(r)
	user := &schema.User{Username: "testuser", Roles: []string{"user"}, Projects: []string{}}
	do := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, req.WithContext(context.WithValue(req.Context(), repository.ContextUserKey, user)))
		return recorder
	}

	t.Run("job list", func(t *testing.T) {
		res := do(http.MethodGet, "/jobs/?state=running&cluster=testcluster")
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())

		var payload struct {
			Jobs []*schema.Job `json:"jobs"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		byID := map[int64]*schema.Job{}
		for _, j := range payload.Jobs {
			byID[*j.ID] = j
		}
		require.Contains(t, byID, longID)
		require.Contains(t, byID, shortID)
		assert.Equal(t, map[string]float64{"load_one_avg": 3.5}, byID[longID].Footprint, "live footprint above the cutoff")
		assert.Empty(t, byID[shortID].Footprint, "no footprint below the cutoff")
	})

	t.Run("single job", func(t *testing.T) {
		res := do(http.MethodGet, fmt.Sprintf("/jobs/%d", longID))
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())

		var payload struct {
			Meta *schema.Job `json:"meta"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		require.NotNil(t, payload.Meta)
		assert.Equal(t, 3.5, payload.Meta.Footprint["load_one_avg"])
	})
}
