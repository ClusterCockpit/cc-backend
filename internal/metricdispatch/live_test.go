// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-lib/v2/lrucache"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

var testNow = time.Unix(1_800_000_000, 0)

// setupLive registers a test cluster with the given names, resets the live
// cache, fixes the clock and sets the short-job cutoff to 5 minutes.
func setupLive(t *testing.T, clusters ...string) {
	t.Helper()
	savedClusters, savedCutoff, savedNow, savedCache := archive.Clusters, config.Keys.ShortRunningJobsDuration, now, liveCache
	t.Cleanup(func() {
		archive.Clusters, config.Keys.ShortRunningJobsDuration, now, liveCache = savedClusters, savedCutoff, savedNow, savedCache
	})

	mc := func(name, fp, energy string) *schema.MetricConfig {
		return &schema.MetricConfig{Metric: schema.Metric{Name: name}, Footprint: fp, Energy: energy, Scope: schema.MetricScopeNode}
	}
	for _, name := range clusters {
		archive.Clusters = append(archive.Clusters, &schema.Cluster{
			Name: name,
			SubClusters: []*schema.SubCluster{{
				Name: "main",
				MetricConfig: []*schema.MetricConfig{
					mc("flops_any", "avg", ""),
					mc("mem_bw", "max", ""),
					mc("cpu_power", "", "power"),
				},
				Footprint:       []string{"flops_any", "mem_bw"},
				EnergyFootprint: []string{"cpu_power"},
			}},
		})
	}
	config.Keys.ShortRunningJobsDuration = 300
	now = func() time.Time { return testNow }
	liveCache = lrucache.New(1024 * 1024)
}

func liveJob(cluster string, id int64, runFor time.Duration) *schema.Job {
	return &schema.Job{
		ID:               &id,
		JobID:            id,
		Cluster:          cluster,
		SubCluster:       "main",
		State:            schema.JobStateRunning,
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
		StartTime:        testNow.Add(-runFor).Unix(),
		Duration:         1, // stale persisted value
		NumNodes:         2,
		Resources:        []*schema.Resource{{Hostname: "n1"}, {Hostname: "n2"}},
	}
}

func twoNodeStats() map[string]map[string]schema.MetricStatistics {
	return map[string]map[string]schema.MetricStatistics{
		"flops_any": {"n1": {Avg: 100, Min: 10, Max: 200}, "n2": {Avg: 50, Min: 5, Max: 90}},
		"mem_bw":    {"n1": {Avg: 30, Min: 1, Max: 40}, "n2": {Avg: 20, Min: 2, Max: 70}},
		"cpu_power": {"n1": {Avg: 300, Min: 250, Max: 350}, "n2": {Avg: 200, Min: 150, Max: 250}},
	}
}

func TestLiveFootprintRunningJob(t *testing.T) {
	setupLive(t, "live-a")
	repo := &fakeRepo{stats: twoNodeStats()}
	useRepo(t, "live-a", repo)

	lv, ok := LiveFootprint(context.Background(), liveJob("live-a", 1, 2*time.Hour))
	if !ok {
		t.Fatal("running job not treated as live")
	}
	if got := lv.Footprint["flops_any_avg"]; got != 75 {
		t.Errorf("flops_any_avg = %v, want 75", got)
	}
	if got := lv.Footprint["mem_bw_max"]; got != 70 {
		t.Errorf("mem_bw_max = %v, want 70", got)
	}
	// mean power 250 W * 2 nodes * 2 h / 1000 = 1 kWh
	if lv.EnergyFootprint["cpu_power"] != 1 || lv.Energy != 1 {
		t.Errorf("energy footprint %v, total %v; want 1 kWh", lv.EnergyFootprint, lv.Energy)
	}
	if repo.calls.Load() != 1 {
		t.Errorf("store calls = %d, want 1 (footprint and energy share one query)", repo.calls.Load())
	}
	if repo.lastJob.Duration != int32((2 * time.Hour).Seconds()) {
		t.Errorf("queried duration = %d, want elapsed run time %d", repo.lastJob.Duration, int32((2 * time.Hour).Seconds()))
	}
}

func TestLiveFootprintBelowCutoff(t *testing.T) {
	setupLive(t, "live-b")
	repo := &fakeRepo{stats: twoNodeStats()}
	useRepo(t, "live-b", repo)

	lv, ok := LiveFootprint(context.Background(), liveJob("live-b", 2, 2*time.Minute))
	if !ok {
		t.Fatal("short running job must still be live (with empty values)")
	}
	if len(lv.Footprint) != 0 || len(lv.EnergyFootprint) != 0 || lv.Energy != 0 {
		t.Errorf("short job got values %+v", lv)
	}
	if repo.calls.Load() != 0 {
		t.Errorf("store queried %d times for a job below the cutoff", repo.calls.Load())
	}
}

func TestLiveFootprintEligibility(t *testing.T) {
	setupLive(t, "live-c")
	useRepo(t, "live-c", &fakeRepo{stats: twoNodeStats()})

	stopped := liveJob("live-c", 3, time.Hour)
	stopped.State = schema.JobStateCompleted // archiving still in progress
	if lv, ok := LiveFootprint(context.Background(), stopped); !ok || lv.Footprint["flops_any_avg"] != 75 {
		t.Errorf("stopped, not archived job: ok=%v values=%v; want live values", ok, lv.Footprint)
	}

	archived := liveJob("live-c", 4, time.Hour)
	archived.State = schema.JobStateCompleted
	archived.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful
	if _, ok := LiveFootprint(context.Background(), archived); ok {
		t.Error("archived job treated as live")
	}

	disabled := liveJob("live-c", 5, time.Hour)
	disabled.MonitoringStatus = schema.MonitoringStatusDisabled
	if _, ok := LiveFootprint(context.Background(), disabled); ok {
		t.Error("job with disabled monitoring treated as live")
	}
}

func TestStoreConcurrencyConfig(t *testing.T) {
	n, err := storeConcurrency(CCMetricStoreConfig{Scope: "*"})
	if err != nil || n != DefaultMaxConcurrentRequests {
		t.Errorf("default: got %d, %v; want %d", n, err, DefaultMaxConcurrentRequests)
	}

	var cfgs []CCMetricStoreConfig
	if err := json.Unmarshal([]byte(`[{"scope":"a","url":"u","max-concurrent-requests":3},{"scope":"b","url":"u","max-concurrent-requests":0}]`), &cfgs); err != nil {
		t.Fatal(err)
	}
	if n, err := storeConcurrency(cfgs[0]); err != nil || n != 3 {
		t.Errorf("explicit: got %d, %v; want 3", n, err)
	}
	if _, err := storeConcurrency(cfgs[1]); err == nil {
		t.Error("0 must be rejected")
	}

	sch, err := jsonschema.CompileString("schema.json", configSchema)
	if err != nil {
		t.Fatal(err)
	}
	for raw, valid := range map[string]bool{
		`[{"scope":"*","url":"u","max-concurrent-requests":4}]`: true,
		`[{"scope":"*","url":"u","max-concurrent-requests":0}]`: false,
	} {
		var v any
		_ = json.Unmarshal([]byte(raw), &v)
		if err := sch.Validate(v); (err == nil) != valid {
			t.Errorf("schema validation of %s: err=%v, want valid=%v", raw, err, valid)
		}
	}
}

func TestLiveFootprintPerRepoConcurrency(t *testing.T) {
	setupLive(t, "live-busy", "live-idle", "live-limit")

	t.Run("saturated store does not block another store", func(t *testing.T) {
		busy := &fakeRepo{stats: twoNodeStats(), block: make(chan struct{})}
		idle := &fakeRepo{stats: twoNodeStats()}
		useRepo(t, "live-busy", busy)
		useRepo(t, "live-idle", idle)
		setRepoConcurrency(busy, 1)
		setRepoConcurrency(idle, 1)

		done := make(chan struct{})
		go func() {
			LiveFootprint(context.Background(), liveJob("live-busy", 10, time.Hour))
			close(done)
		}()
		for busy.inFlight.Load() == 0 {
			time.Sleep(time.Millisecond)
		}

		finished := make(chan struct{})
		go func() {
			LiveFootprint(context.Background(), liveJob("live-idle", 11, time.Hour))
			close(finished)
		}()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Fatal("call to idle store blocked by saturated store")
		}
		close(busy.block)
		<-done
	})

	t.Run("in-flight calls never exceed the limit", func(t *testing.T) {
		limited := &fakeRepo{stats: twoNodeStats(), delay: 20 * time.Millisecond}
		useRepo(t, "live-limit", limited)
		setRepoConcurrency(limited, 2)

		var wg sync.WaitGroup
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				LiveFootprint(context.Background(), liveJob("live-limit", int64(100+i), time.Hour))
			}()
		}
		wg.Wait()
		if got := limited.maxSeen.Load(); got > 2 {
			t.Errorf("max in-flight = %d, want <= 2", got)
		}
		if got := limited.calls.Load(); got != 8 {
			t.Errorf("calls = %d, want 8 distinct jobs", got)
		}
	})
}

func TestLiveFootprintSharedComputation(t *testing.T) {
	setupLive(t, "live-d")
	repo := &fakeRepo{stats: twoNodeStats(), delay: 50 * time.Millisecond}
	useRepo(t, "live-d", repo)

	var wg sync.WaitGroup
	results := make([]LiveValues, 10)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = LiveFootprint(context.Background(), liveJob("live-d", 20, time.Hour))
		}()
	}
	wg.Wait()

	if got := repo.calls.Load(); got != 1 {
		t.Errorf("store calls = %d, want 1", got)
	}
	for i, r := range results {
		if fmt.Sprint(r) != fmt.Sprint(results[0]) {
			t.Errorf("result %d = %v differs from %v", i, r, results[0])
		}
	}
}

func TestLiveFootprintStoreError(t *testing.T) {
	setupLive(t, "live-e")
	useRepo(t, "live-e", &fakeRepo{err: errors.New("store unreachable")})

	lv, ok := LiveFootprint(context.Background(), liveJob("live-e", 30, time.Hour))
	if !ok {
		t.Fatal("running job not treated as live")
	}
	if lv.Footprint == nil || len(lv.Footprint) != 0 || len(lv.EnergyFootprint) != 0 || lv.Energy != 0 {
		t.Errorf("store failure: got %+v, want empty values", lv)
	}
}

func TestLiveEnergyGrowsWithRunTime(t *testing.T) {
	setupLive(t, "live-f")
	useRepo(t, "live-f", &fakeRepo{stats: twoNodeStats()})
	job := liveJob("live-f", 40, time.Hour)

	first, _ := LiveFootprint(context.Background(), job)

	// Advance past the freshness interval and let the cache entry expire.
	now = func() time.Time { return testNow.Add(2 * liveFootprintTTL) }
	liveCache.Del(fmt.Sprintf("livefp:%d", *job.ID))

	second, _ := LiveFootprint(context.Background(), job)
	if second.Energy <= first.Energy {
		t.Errorf("energy did not grow: first %v, second %v", first.Energy, second.Energy)
	}
}
