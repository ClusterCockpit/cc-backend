// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// fakeRepo is a MetricDataRepository that serves canned node statistics and
// records how often and how concurrently LoadStats is called.
type fakeRepo struct {
	mu       sync.Mutex
	stats    map[string]map[string]schema.MetricStatistics
	err      error
	delay    time.Duration
	block    chan struct{} // if non-nil, LoadStats waits until it is closed
	calls    atomic.Int64
	inFlight atomic.Int64
	maxSeen  atomic.Int64
	lastJob  *schema.Job
}

func (f *fakeRepo) LoadStats(job *schema.Job, metrics []string, _ map[string]bool, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {
	f.calls.Add(1)
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxSeen.Load()
		if n <= m || f.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	if f.block != nil {
		<-f.block
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastJob = job
	if f.err != nil {
		return nil, f.err
	}
	res := make(map[string]map[string]schema.MetricStatistics, len(metrics))
	for _, m := range metrics {
		if hosts, ok := f.stats[m]; ok {
			res[m] = hosts
		}
	}
	return res, nil
}

func (f *fakeRepo) LoadData(*schema.Job, []string, []schema.MetricScope, context.Context, int, string) (schema.JobData, error) {
	return schema.JobData{}, nil
}

func (f *fakeRepo) LoadScopedStats(*schema.Job, []string, []schema.MetricScope, context.Context) (schema.ScopedJobStats, error) {
	return schema.ScopedJobStats{}, nil
}

func (f *fakeRepo) LoadNodeData(string, []string, []string, []schema.MetricScope, time.Time, time.Time, context.Context) (map[string]map[string][]*schema.JobMetric, error) {
	return nil, nil
}

func (f *fakeRepo) LoadNodeListData(string, string, []string, []string, []schema.MetricScope, int, time.Time, time.Time, context.Context, string) (map[string]schema.JobData, error) {
	return nil, nil
}

func (f *fakeRepo) HealthCheck(string, []string, []string) (map[string]metricstore.HealthCheckResult, error) {
	return nil, nil
}

// useRepo registers repo for cluster for the duration of the test.
func useRepo(t interface{ Cleanup(func()) }, cluster string, repo MetricDataRepository) {
	metricDataRepos[cluster] = repo
	t.Cleanup(func() { delete(metricDataRepos, cluster) })
}
