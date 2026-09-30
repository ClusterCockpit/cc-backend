// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fpCluster = "fpcluster"

// fpFixture is a set of jobs on fpCluster with live metric values served by
// the internal metric store test hook. Running jobs are keyed by name.
type fpFixture struct {
	r   *JobRepository
	ids map[string]int64

	mu      sync.Mutex
	queried []string // hostnames the metric store was asked for
}

type fpJob struct {
	name, user string
	state      schema.JobState
	runFor     time.Duration
	liveFlops  float64 // live flops_any average on the job's node (running jobs)
	livePower  float64 // live cpu_power average (running jobs)
	dbFlops    float64 // persisted flops_any_avg footprint
	hasLive    bool
}

func setupFootprintQueries(t *testing.T) *fpFixture {
	t.Helper()
	r := setup(t)

	savedClusters, savedCutoff := archive.Clusters, config.Keys.ShortRunningJobsDuration
	savedHandle, savedCB := metricstore.MetricStoreHandle, metricstore.TestLoadStatsCallback
	t.Cleanup(func() {
		archive.Clusters, config.Keys.ShortRunningJobsDuration = savedClusters, savedCutoff
		metricstore.MetricStoreHandle, metricstore.TestLoadStatsCallback = savedHandle, savedCB
		metricdispatch.ResetLiveFootprintCache()
	})
	metricdispatch.ResetLiveFootprintCache()

	mc := func(name, fp, energy string) *schema.MetricConfig {
		return &schema.MetricConfig{Metric: schema.Metric{Name: name}, Footprint: fp, Energy: energy, Scope: schema.MetricScopeNode}
	}
	archive.Clusters = append(slices.Clone(archive.Clusters), &schema.Cluster{
		Name: fpCluster,
		SubClusters: []*schema.SubCluster{{
			Name:            "main",
			MetricConfig:    []*schema.MetricConfig{mc("flops_any", "avg", ""), mc("cpu_power", "", "power")},
			Footprint:       []string{"flops_any"},
			EnergyFootprint: []string{"cpu_power"},
		}},
	})
	config.Keys.ShortRunningJobsDuration = 300

	f := &fpFixture{r: r, ids: map[string]int64{}}
	jobs := []fpJob{
		{name: "R1", user: "alice", state: schema.JobStateRunning, runFor: 2 * time.Hour, liveFlops: 10, livePower: 100, dbFlops: 1, hasLive: true},
		{name: "R2", user: "alice", state: schema.JobStateRunning, runFor: 2 * time.Hour, liveFlops: 50, livePower: 500, hasLive: true},
		{name: "R3", user: "bob", state: schema.JobStateRunning, runFor: 2 * time.Hour, liveFlops: 50, livePower: 250, hasLive: true},
		{name: "R4", user: "bob", state: schema.JobStateRunning, runFor: 2 * time.Hour, liveFlops: 200, livePower: 1000, hasLive: true},
		{name: "R5", user: "bob", state: schema.JobStateRunning, runFor: time.Minute, liveFlops: 999, livePower: 999, hasLive: true},
		{name: "F1", user: "alice", state: schema.JobStateCompleted, runFor: time.Hour, dbFlops: 5},
		{name: "F2", user: "bob", state: schema.JobStateFailed, runFor: time.Hour, dbFlops: 300},
	}

	live := map[string]fpJob{}
	for i, j := range jobs {
		host := "fpn-" + j.name
		status := schema.MonitoringStatusRunningOrArchiving
		if j.state != schema.JobStateRunning {
			status = schema.MonitoringStatusArchivingSuccessful
		}
		job := &schema.Job{
			JobID: int64(880000 + i), User: j.user, Project: "p", Cluster: fpCluster, SubCluster: "main",
			Partition: "batch", NumNodes: 1, NumHWThreads: 4, Shared: "none", SMT: 1, Walltime: 86400,
			MonitoringStatus: status, State: j.state,
			StartTime: time.Now().Add(-j.runFor).Unix(), Duration: int32(j.runFor.Seconds()),
			Resources: []*schema.Resource{{Hostname: host}},
		}
		job.RawResources, _ = json.Marshal(job.Resources)
		if j.dbFlops != 0 {
			job.RawFootprint, _ = json.Marshal(map[string]float64{"flops_any_avg": j.dbFlops})
		} else {
			job.RawFootprint = []byte("null")
		}
		job.RawEnergyFootprint = []byte("null")
		job.RawMetaData = []byte("{}")
		id, err := r.InsertJobDirect(job)
		require.NoError(t, err)
		f.ids[j.name] = id
		if j.hasLive {
			live[host] = j
		}
	}

	metricstore.MetricStoreHandle = &metricstore.InternalMetricStore{}
	metricstore.TestLoadStatsCallback = func(job *schema.Job, metrics []string, _ map[string]bool, _ context.Context) (map[string]map[string]schema.MetricStatistics, error) {
		host := job.Resources[0].Hostname
		f.mu.Lock()
		f.queried = append(f.queried, host)
		f.mu.Unlock()
		j, ok := live[host]
		if !ok {
			return nil, fmt.Errorf("no data for %s", host)
		}
		return map[string]map[string]schema.MetricStatistics{
			"flops_any": {host: {Avg: j.liveFlops, Min: j.liveFlops, Max: j.liveFlops}},
			"cpu_power": {host: {Avg: j.livePower, Min: j.livePower, Max: j.livePower}},
		}, nil
	}
	return f
}

func (f *fpFixture) names(t *testing.T, jobs []*schema.Job) []string {
	t.Helper()
	byID := map[int64]string{}
	for n, id := range f.ids {
		byID[id] = n
	}
	res := make([]string, 0, len(jobs))
	for _, j := range jobs {
		res = append(res, byID[*j.ID])
	}
	return res
}

func clusterFilter() *model.JobFilter {
	c := fpCluster
	return &model.JobFilter{Cluster: &model.StringInput{Eq: &c}}
}

func runningFilter() *model.JobFilter {
	return &model.JobFilter{State: []schema.JobState{schema.JobStateRunning}}
}

func flopsFilter(from, to float64) *model.JobFilter {
	return &model.JobFilter{MetricStats: []*model.MetricStatItem{
		{MetricName: "flops_any_avg", Range: &model.FloatRange{From: from, To: to}},
	}}
}

func flopsOrder(dir model.SortDirectionEnum) *model.OrderByInput {
	return &model.OrderByInput{Field: "flops_any_avg", Type: "footprint", Order: dir}
}

func TestClassifyFootprintQuery(t *testing.T) {
	running := []schema.JobState{schema.JobStateRunning}
	mixed := []schema.JobState{schema.JobStateRunning, schema.JobStateFailed}
	finished := []schema.JobState{schema.JobStateCompleted, schema.JobStateFailed}
	metric := []*model.MetricStatItem{{MetricName: "flops_any_avg", Range: &model.FloatRange{From: 1}}}
	energy := &model.FloatRange{From: 1}
	fpOrder := &model.OrderByInput{Field: "flops_any_avg", Type: "footprint", Order: model.SortDirectionEnumDesc}
	energyOrder := &model.OrderByInput{Field: "energy", Type: "col", Order: model.SortDirectionEnumDesc}
	colOrder := &model.OrderByInput{Field: "startTime", Type: "col", Order: model.SortDirectionEnumDesc}

	tests := []struct {
		name    string
		filters []*model.JobFilter
		order   *model.OrderByInput
		histo   bool
		want    footprintMode
		wantErr bool
	}{
		{"no feature, mixed states", []*model.JobFilter{{State: mixed}}, colOrder, false, footprintPlain, false},
		{"no feature, no state", nil, nil, false, footprintPlain, false},
		{"metricStats running", []*model.JobFilter{{State: running, MetricStats: metric}}, nil, false, footprintLive, false},
		{"energy filter running", []*model.JobFilter{{State: running, Energy: energy}}, nil, false, footprintLive, false},
		{"footprint sort running", []*model.JobFilter{{State: running}}, fpOrder, false, footprintLive, false},
		{"energy sort running", []*model.JobFilter{{State: running}}, energyOrder, false, footprintLive, false},
		{"histograms running", []*model.JobFilter{{State: running}}, nil, true, footprintLive, false},
		{"footprint sort finished", []*model.JobFilter{{State: finished}}, fpOrder, false, footprintFinished, false},
		{"footprint sort no state", nil, fpOrder, false, footprintFinished, false},
		{"histograms no state", []*model.JobFilter{{}}, nil, true, footprintFinished, false},
		{"mixed with footprint sort", []*model.JobFilter{{State: mixed}}, fpOrder, false, footprintPlain, true},
		{"mixed with metricStats", []*model.JobFilter{{State: mixed, MetricStats: metric}}, nil, false, footprintPlain, true},
		{"mixed with energy sort", []*model.JobFilter{{State: mixed}}, energyOrder, false, footprintPlain, true},
		{"intersection is running", []*model.JobFilter{{State: mixed}, {State: running}}, fpOrder, false, footprintLive, false},
		{"intersection excludes running", []*model.JobFilter{{State: mixed}, {State: finished}}, fpOrder, false, footprintFinished, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classifyFootprintQuery(tt.filters, tt.order, tt.histo)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrMixedStateFootprintQuery)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFootprintQueryFinishedWithoutState(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)

	jobs, err := f.r.QueryJobs(ctx, []*model.JobFilter{clusterFilter()}, nil, flopsOrder(model.SortDirectionEnumDesc))
	require.NoError(t, err)
	assert.Equal(t, []string{"F2", "F1"}, f.names(t, jobs), "footprint sort without state returns finished jobs only")

	n, err := f.r.CountJobs(ctx, []*model.JobFilter{clusterFilter(), flopsFilter(1, 0)})
	require.NoError(t, err)
	assert.Equal(t, 2, n, "footprint filter without state counts finished jobs only")
}

func TestFootprintQueryLiveFilters(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)

	t.Run("metricStats uses live values, not the persisted column", func(t *testing.T) {
		filters := []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(5, 100)}
		jobs, err := f.r.QueryJobs(ctx, filters, nil, nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"R1", "R2", "R3"}, f.names(t, jobs))

		filters = []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(20, 100)}
		jobs, err = f.r.QueryJobs(ctx, filters, nil, nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"R2", "R3"}, f.names(t, jobs), "R1 has live 10 (persisted 1)")
		for _, j := range jobs {
			assert.Equal(t, 50.0, j.Footprint["flops_any_avg"], "live values are stored on the jobs")
		}
		n, err := f.r.CountJobs(ctx, filters)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
	})

	t.Run("short job never matches a constraining range", func(t *testing.T) {
		jobs, err := f.r.QueryJobs(ctx, []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(0, 1000000)}, nil, nil)
		require.NoError(t, err)
		assert.NotContains(t, f.names(t, jobs), "R5")
	})

	t.Run("energy filter", func(t *testing.T) {
		filters := []*model.JobFilter{clusterFilter(), runningFilter(), {Energy: &model.FloatRange{From: 0.6}}}
		jobs, err := f.r.QueryJobs(ctx, filters, nil, nil)
		require.NoError(t, err)
		// 500 W * 2 h = 1 kWh, 1000 W * 2 h = 2 kWh
		assert.ElementsMatch(t, []string{"R2", "R4"}, f.names(t, jobs))
	})

	t.Run("empty match", func(t *testing.T) {
		filters := []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(100000, 0)}
		jobs, err := f.r.QueryJobs(ctx, filters, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, jobs)
		n, err := f.r.CountJobs(ctx, filters)
		require.NoError(t, err)
		assert.Zero(t, n)
		stats, err := f.r.JobsStats(ctx, filters, map[string]bool{"totalJobs": true})
		require.NoError(t, err)
		require.Len(t, stats, 1)
		assert.Zero(t, stats[0].TotalJobs)
	})

	t.Run("caller filters are not modified", func(t *testing.T) {
		ms := flopsFilter(20, 100)
		filters := []*model.JobFilter{clusterFilter(), runningFilter(), ms}
		_, err := f.r.RouteFootprintQuery(ctx, filters, nil, false)
		require.NoError(t, err)
		assert.Len(t, filters, 3)
		assert.Len(t, ms.MetricStats, 1)
		assert.Nil(t, ms.DbID)
	})

	t.Run("mixed states are rejected", func(t *testing.T) {
		filters := []*model.JobFilter{clusterFilter(), {State: []schema.JobState{schema.JobStateRunning, schema.JobStateCompleted}}}
		_, err := f.r.QueryJobs(ctx, filters, nil, flopsOrder(model.SortDirectionEnumAsc))
		assert.ErrorIs(t, err, ErrMixedStateFootprintQuery)
		_, err = f.r.JobsStats(ctx, append(filters, flopsFilter(1, 0)), nil)
		assert.ErrorIs(t, err, ErrMixedStateFootprintQuery)
	})
}

func TestFootprintQueryLiveRespectsAccess(t *testing.T) {
	f := setupFootprintQueries(t)
	alice := context.WithValue(context.Background(), ContextUserKey, &schema.User{
		Username: "alice", Roles: []string{schema.GetRoleString(schema.RoleUser)},
	})

	jobs, err := f.r.QueryJobs(alice, []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(1, 0)}, nil, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"R1", "R2"}, f.names(t, jobs))

	f.mu.Lock()
	defer f.mu.Unlock()
	for _, host := range f.queried {
		assert.Contains(t, []string{"fpn-R1", "fpn-R2"}, host, "live values computed for a job the user cannot see")
	}
}

func TestBuildWhereClauseLargeDbIDSet(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)

	ids := make([]string, 0, 40000)
	for i := range 40000 {
		ids = append(ids, strconv.Itoa(10_000_000+i))
	}
	ids = append(ids, strconv.FormatInt(f.ids["F1"], 10), strconv.FormatInt(f.ids["R2"], 10))

	n, err := f.r.CountJobs(ctx, []*model.JobFilter{{DbID: ids}})
	require.NoError(t, err, "more ids than SQLite's variable limit")
	assert.Equal(t, 2, n)
}

func TestFootprintQueryLiveSort(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)
	filters := []*model.JobFilter{clusterFilter(), runningFilter()}

	desc, err := f.r.QueryJobs(ctx, filters, nil, flopsOrder(model.SortDirectionEnumDesc))
	require.NoError(t, err)
	// R2 and R3 tie at 50 and are ordered by id; R5 has no value.
	assert.Equal(t, []string{"R4", "R2", "R3", "R1", "R5"}, f.names(t, desc))

	asc, err := f.r.QueryJobs(ctx, filters, nil, flopsOrder(model.SortDirectionEnumAsc))
	require.NoError(t, err)
	assert.Equal(t, []string{"R5", "R1", "R2", "R3", "R4"}, f.names(t, asc))

	energy, err := f.r.QueryJobs(ctx, filters, nil, &model.OrderByInput{Field: "energy", Type: "col", Order: model.SortDirectionEnumDesc})
	require.NoError(t, err)
	assert.Equal(t, []string{"R4", "R2", "R3", "R1", "R5"}, f.names(t, energy))

	var paged []string
	for p := 1; p <= 3; p++ {
		jobs, err := f.r.QueryJobs(ctx, filters, &model.PageRequest{Page: p, ItemsPerPage: 2}, flopsOrder(model.SortDirectionEnumDesc))
		require.NoError(t, err)
		paged = append(paged, f.names(t, jobs)...)
	}
	assert.Equal(t, f.names(t, desc), paged, "pages neither overlap nor skip")

	beyond, err := f.r.QueryJobs(ctx, filters, &model.PageRequest{Page: 4, ItemsPerPage: 2}, flopsOrder(model.SortDirectionEnumDesc))
	require.NoError(t, err)
	assert.Empty(t, beyond)
}

func TestFootprintQueryLiveCountsAgree(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)
	filters := []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(20, 0)}

	count, err := f.r.CountJobs(ctx, filters)
	require.NoError(t, err)

	seen := map[string]bool{}
	for p := 1; ; p++ {
		jobs, err := f.r.QueryJobs(ctx, filters, &model.PageRequest{Page: p, ItemsPerPage: 2}, nil)
		require.NoError(t, err)
		if len(jobs) == 0 {
			break
		}
		for _, n := range f.names(t, jobs) {
			assert.False(t, seen[n], "job %s on two pages", n)
			seen[n] = true
		}
	}
	assert.Equal(t, 3, count)
	assert.Len(t, seen, count, "count equals jobs reachable by paging")

	groupBy := model.AggregateUser
	grouped, err := f.r.JobsStatsGrouped(ctx, filters, nil, nil, &groupBy, map[string]bool{"totalJobs": true})
	require.NoError(t, err)
	perUser := map[string]int{}
	for _, g := range grouped {
		perUser[g.ID] = g.TotalJobs
	}
	assert.Equal(t, map[string]int{"alice": 1, "bob": 2}, perUser)
}

func TestFootprintQueryConcurrentSharedFilters(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)
	filters := []*model.JobFilter{clusterFilter(), runningFilter(), flopsFilter(20, 0)}
	order := flopsOrder(model.SortDirectionEnumDesc)

	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for range 10 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, err := f.r.QueryJobs(ctx, filters, &model.PageRequest{Page: 1, ItemsPerPage: 2}, order)
			errs <- err
		}()
		go func() { defer wg.Done(); _, err := f.r.CountJobs(ctx, filters); errs <- err }()
		go func() {
			defer wg.Done()
			groupBy := model.AggregateUser
			_, err := f.r.JobsStatsGrouped(ctx, filters, nil, nil, &groupBy, map[string]bool{"totalJobs": true})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Len(t, filters[2].MetricStats, 1)
	assert.Nil(t, filters[2].DbID)
}

func TestRunningMetricHistogramUsesFootprintMean(t *testing.T) {
	f := setupFootprintQueries(t)
	ctx := getContext(t)

	// Cluster-level config provides the peak used for binning: 10 bins of 100.
	for _, c := range archive.Clusters {
		if c.Name == fpCluster {
			c.MetricConfig = []*schema.MetricConfig{{Metric: schema.Metric{Name: "flops_any", Peak: 1000}, Footprint: "avg"}}
		}
	}

	job := &schema.Job{
		JobID: 889999, User: "carol", Project: "p", Cluster: fpCluster, SubCluster: "main",
		Partition: "batch", NumNodes: 2, NumHWThreads: 8, Shared: "none", SMT: 1, Walltime: 86400,
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving, State: schema.JobStateRunning,
		StartTime: time.Now().Add(-time.Hour).Unix(), Duration: 3600,
		Resources: []*schema.Resource{{Hostname: "fpn-two-a"}, {Hostname: "fpn-two-b"}},
	}
	job.RawResources, _ = json.Marshal(job.Resources)
	job.RawFootprint, job.RawEnergyFootprint, job.RawMetaData = []byte("null"), []byte("null"), []byte("{}")
	_, err := f.r.InsertJobDirect(job)
	require.NoError(t, err)

	metricstore.TestLoadStatsCallback = func(j *schema.Job, _ []string, _ map[string]bool, _ context.Context) (map[string]map[string]schema.MetricStatistics, error) {
		if j.Resources[0].Hostname != "fpn-two-a" {
			return nil, fmt.Errorf("not part of this test")
		}
		return map[string]map[string]schema.MetricStatistics{
			"flops_any": {
				"fpn-two-a": {Avg: 100, Min: 100, Max: 100},
				"fpn-two-b": {Avg: 300, Min: 300, Max: 300},
			},
		}, nil
	}

	carol := "carol"
	filters := []*model.JobFilter{clusterFilter(), runningFilter(), {User: &model.StringInput{Eq: &carol}}}
	bins := 10
	stat, err := f.r.AddMetricHistograms(ctx, filters, []string{"flops_any"}, &model.JobsStatistics{}, &bins)
	require.NoError(t, err)
	require.Len(t, stat.HistMetrics, 1)

	counts := map[int]int{}
	for _, p := range stat.HistMetrics[0].Data {
		counts[*p.Min] = p.Count
	}
	assert.Equal(t, 1, counts[200], "binned by the mean over nodes (200)")
	assert.Equal(t, 0, counts[400], "not by the sum of node averages (400)")
	require.NotNil(t, stat.HistMetrics[0].Stat)
	assert.Equal(t, "avg", *stat.HistMetrics[0].Stat)
}
