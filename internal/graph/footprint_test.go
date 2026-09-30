// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/generated"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gfCluster = "gfcluster"

type graphFixture struct {
	resolver *Resolver
	running  *schema.Job
	short    *schema.Job
	finished *schema.Job
	calls    atomic.Int64
	fail     atomic.Bool
}

// setupGraphFootprint creates a fresh database with a running job (2 h, 100
// flops, 500 W), a short running job and an archived job with persisted
// footprint, and serves live statistics through the internal metric store hook.
func setupGraphFootprint(t *testing.T) *graphFixture {
	t.Helper()
	cclog.Init("warn", true)

	require.NoError(t, repository.ResetConnection())
	dbfile := filepath.Join(t.TempDir(), "job.db")
	require.NoError(t, repository.MigrateDB(dbfile))
	repository.Connect(dbfile)

	savedClusters, savedCutoff := archive.Clusters, config.Keys.ShortRunningJobsDuration
	savedHandle, savedCB := metricstore.MetricStoreHandle, metricstore.TestLoadStatsCallback
	t.Cleanup(func() {
		archive.Clusters, config.Keys.ShortRunningJobsDuration = savedClusters, savedCutoff
		metricstore.MetricStoreHandle, metricstore.TestLoadStatsCallback = savedHandle, savedCB
		metricdispatch.ResetLiveFootprintCache()
		repository.ResetConnection()
	})
	metricdispatch.ResetLiveFootprintCache()

	mc := func(name, fp, energy string) *schema.MetricConfig {
		return &schema.MetricConfig{Metric: schema.Metric{Name: name}, Footprint: fp, Energy: energy, Scope: schema.MetricScopeNode}
	}
	archive.Clusters = append(slices.Clone(archive.Clusters), &schema.Cluster{
		Name: gfCluster,
		SubClusters: []*schema.SubCluster{{
			Name:            "main",
			MetricConfig:    []*schema.MetricConfig{mc("flops_any", "avg", ""), mc("cpu_power", "", "power")},
			Footprint:       []string{"flops_any"},
			EnergyFootprint: []string{"cpu_power"},
		}},
	})
	config.Keys.ShortRunningJobsDuration = 300

	repo := repository.GetJobRepository()
	f := &graphFixture{resolver: &Resolver{DB: repository.GetConnection().DB, Repo: repo}}

	insert := func(jobID int64, state schema.JobState, status int32, runFor time.Duration, fp, efp string, energy float64) *schema.Job {
		job := &schema.Job{
			JobID: jobID, User: "alice", Project: "p", Cluster: gfCluster, SubCluster: "main",
			Partition: "batch", NumNodes: 1, NumHWThreads: 4, Shared: "none", SMT: 1, Walltime: 86400,
			MonitoringStatus: status, State: state, Energy: energy,
			StartTime: time.Now().Add(-runFor).Unix(), Duration: int32(runFor.Seconds()),
			Resources: []*schema.Resource{{Hostname: "gfn1"}},
		}
		job.RawResources, _ = json.Marshal(job.Resources)
		job.RawFootprint, job.RawEnergyFootprint, job.RawMetaData = []byte(fp), []byte(efp), []byte("{}")
		id, err := repo.InsertJobDirect(job)
		require.NoError(t, err)
		loaded, err := repo.FindByIDDirect(id)
		require.NoError(t, err)
		return loaded
	}
	// The running job carries a stale persisted footprint that must be ignored.
	f.running = insert(770001, schema.JobStateRunning, schema.MonitoringStatusRunningOrArchiving, 2*time.Hour,
		`{"flops_any_avg": 1}`, `{"cpu_power": 0.01}`, 0.01)
	f.short = insert(770002, schema.JobStateRunning, schema.MonitoringStatusRunningOrArchiving, time.Minute,
		"null", "null", 0)
	f.finished = insert(770003, schema.JobStateCompleted, schema.MonitoringStatusArchivingSuccessful, time.Hour,
		`{"flops_any_avg": 42}`, `{"cpu_power": 3.5}`, 3.5)

	metricstore.MetricStoreHandle = &metricstore.InternalMetricStore{}
	metricstore.TestLoadStatsCallback = func(job *schema.Job, _ []string, _ map[string]bool, _ context.Context) (map[string]map[string]schema.MetricStatistics, error) {
		f.calls.Add(1)
		if f.fail.Load() {
			return nil, errors.New("metric store unreachable")
		}
		return map[string]map[string]schema.MetricStatistics{
			"flops_any": {"gfn1": {Avg: 100, Min: 100, Max: 100}},
			"cpu_power": {"gfn1": {Avg: 500, Min: 500, Max: 500}},
		}, nil
	}
	return f
}

func adminContext() context.Context {
	return context.WithValue(context.Background(), repository.ContextUserKey, &schema.User{
		Username: "admin", Roles: []string{schema.GetRoleString(schema.RoleAdmin)},
	})
}

func TestJobsResolverRoutesOnce(t *testing.T) {
	f := setupGraphFootprint(t)
	c := gfCluster
	filters := []*model.JobFilter{
		{Cluster: &model.StringInput{Eq: &c}},
		{State: []schema.JobState{schema.JobStateRunning}},
	}
	order := &model.OrderByInput{Field: "flops_any_avg", Type: "footprint", Order: model.SortDirectionEnumDesc}

	before := repository.LiveCandidateScans()
	res, err := f.resolver.Query().Jobs(adminContext(), filters, &model.PageRequest{Page: 1, ItemsPerPage: 1}, order)
	require.NoError(t, err)

	assert.Equal(t, int64(1), repository.LiveCandidateScans()-before, "page, count and next-page probe share one routing")
	require.Len(t, res.Items, 1)
	assert.Equal(t, f.running.JobID, res.Items[0].JobID, "job with the highest live value first")
	assert.Equal(t, 2, *res.Count)
	assert.True(t, *res.HasNextPage)
}

func footprintMap(values []*model.FootprintValue) map[string]float64 {
	m := map[string]float64{}
	for _, v := range values {
		m[v.Name+"_"+v.Stat] = v.Value
	}
	return m
}

func TestJobFootprintResolvers(t *testing.T) {
	f := setupGraphFootprint(t)
	jr := f.resolver.Job()
	ctx := adminContext()

	t.Run("running job gets live values", func(t *testing.T) {
		fp, err := jr.Footprint(ctx, f.running)
		require.NoError(t, err)
		assert.Equal(t, map[string]float64{"flops_any_avg": 100}, footprintMap(fp), "live value, not the persisted 1")

		efp, err := jr.EnergyFootprint(ctx, f.running)
		require.NoError(t, err)
		require.Len(t, efp, 1)
		// 500 W * 1 node * 2 h / 1000 = 1 kWh
		assert.Equal(t, 1.0, efp[0].Value)

		energy, err := f.resolver.Job().(*jobResolver).Energy(ctx, f.running)
		require.NoError(t, err)
		assert.Equal(t, 1.0, energy)
	})

	t.Run("short running job has no footprint", func(t *testing.T) {
		before := f.calls.Load()
		fp, err := jr.Footprint(ctx, f.short)
		require.NoError(t, err)
		assert.Empty(t, fp)
		assert.Equal(t, before, f.calls.Load(), "no metric store query below the cutoff")
	})

	t.Run("finished job gets persisted values", func(t *testing.T) {
		before := f.calls.Load()
		fp, err := jr.Footprint(ctx, f.finished)
		require.NoError(t, err)
		assert.Equal(t, map[string]float64{"flops_any_avg": 42}, footprintMap(fp))

		efp, err := jr.EnergyFootprint(ctx, f.finished)
		require.NoError(t, err)
		require.Len(t, efp, 1)
		assert.Equal(t, 3.5, efp[0].Value)

		energy, err := f.resolver.Job().(*jobResolver).Energy(ctx, f.finished)
		require.NoError(t, err)
		assert.Equal(t, 3.5, energy)
		assert.Equal(t, before, f.calls.Load(), "finished jobs do not query the metric store")
	})

	t.Run("metric store failure yields empty values without error", func(t *testing.T) {
		metricdispatch.ResetLiveFootprintCache()
		f.fail.Store(true)
		t.Cleanup(func() { f.fail.Store(false); metricdispatch.ResetLiveFootprintCache() })

		fp, err := jr.Footprint(ctx, f.running)
		require.NoError(t, err)
		assert.Empty(t, fp)
		efp, err := jr.EnergyFootprint(ctx, f.running)
		require.NoError(t, err)
		assert.Empty(t, efp)
	})
}

// graphqlClient serves the generated schema over HTTP with an admin user in the
// request context.
func graphqlClient(f *graphFixture) *client.Client {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: f.resolver}))
	srv.AddTransport(transport.POST{})
	withUser := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(adminContext()))
	})
	return client.New(withUser)
}

func TestGraphQLJobEnergyIsLive(t *testing.T) {
	f := setupGraphFootprint(t)
	c := graphqlClient(f)
	id := strconv.FormatInt(*f.running.ID, 10)

	var bare struct{ Job struct{ JobID int64 } }
	before := f.calls.Load()
	require.NoError(t, c.Post(`query($id: ID!) { job(id: $id) { jobId } }`, &bare, client.Var("id", id)))
	assert.Equal(t, f.running.JobID, bare.Job.JobID)
	assert.Equal(t, before, f.calls.Load(), "no live computation without footprint fields")

	var resp struct {
		Job struct {
			Energy    float64
			Footprint []struct {
				Name, Stat string
				Value      float64
			}
		}
	}
	require.NoError(t, c.Post(`query($id: ID!) { job(id: $id) { energy footprint { name stat value } } }`, &resp, client.Var("id", id)))
	assert.Equal(t, 1.0, resp.Job.Energy, "energy resolved live (persisted value is 0.01)")
	require.Len(t, resp.Job.Footprint, 1)
	assert.Equal(t, 100.0, resp.Job.Footprint[0].Value)
	assert.Equal(t, before+1, f.calls.Load(), "energy and footprint share one computation")
}
