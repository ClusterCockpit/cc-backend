// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/footprint"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/lrucache"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

const (
	// liveFootprintTTL bounds how often the footprint of one job is recomputed.
	// All requests within this interval observe the same values.
	liveFootprintTTL = 60 * time.Second
	// liveFootprintErrTTL is how long a failed computation is remembered before
	// the metric store is asked again.
	liveFootprintErrTTL = 10 * time.Second
	// liveFootprintTimeout bounds one metric store query. The computation is
	// shared by all waiting requests, so it does not use a request context.
	liveFootprintTimeout = 30 * time.Second
	// DefaultMaxConcurrentRequests is the default limit of concurrent live
	// footprint requests to one external metric store.
	DefaultMaxConcurrentRequests = 8
)

// LiveValues holds the footprint, energy footprint and total energy of a job
// computed from the metric store. The maps are shared between all readers of
// the cache entry and must not be modified.
type LiveValues struct {
	Footprint       map[string]float64
	EnergyFootprint map[string]float64
	Energy          float64
}

var (
	liveCache = lrucache.New(32 * 1024 * 1024)

	// now is the clock used for elapsed run times; replaced in tests.
	now = time.Now

	repoSemMu sync.Mutex
	repoSems  = map[MetricDataRepository]chan struct{}{}
)

// ResetLiveFootprintCache drops all cached live values. Tests use it because
// fresh test databases reuse job ids.
func ResetLiveFootprintCache() {
	liveCache = lrucache.New(32 * 1024 * 1024)
}

// IsLive reports whether the footprint of job is computed from the metric store
// rather than read from the database: the job is running or stopped but not yet
// archived, and monitoring is enabled.
func IsLive(job *schema.Job) bool {
	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return false
	}
	return job.State == schema.JobStateRunning ||
		job.MonitoringStatus == schema.MonitoringStatusRunningOrArchiving
}

// elapsed returns the run time of job up to now, in seconds.
func elapsed(job *schema.Job) int64 {
	return now().Unix() - job.StartTime
}

// LiveFootprint returns the live footprint, energy footprint and total energy of
// job. ok is false when the job is not live (see IsLive) and its values have to
// be read from the database instead.
//
// Jobs shorter than the configured short-running-jobs-duration get empty values
// without a metric store query. Values are cached per job for liveFootprintTTL,
// and concurrent callers for the same job share one computation. When the
// metric store fails, the error is logged and empty values are returned.
func LiveFootprint(ctx context.Context, job *schema.Job) (values LiveValues, ok bool) {
	if !IsLive(job) {
		return LiveValues{}, false
	}
	if elapsed(job) < int64(config.Keys.ShortRunningJobsDuration) {
		return emptyLiveValues(), true
	}
	if job.ID == nil {
		return computeLiveFootprint(ctx, job), true
	}

	v := liveCache.Get(fmt.Sprintf("livefp:%d", *job.ID), func() (any, time.Duration, int) {
		lv, err := loadLiveFootprint(ctx, job)
		if err != nil {
			cclog.Warnf("live footprint for job %d (dbid %d, cluster %s): %v", job.JobID, *job.ID, job.Cluster, err)
			return emptyLiveValues(), liveFootprintErrTTL, 64
		}
		return lv, liveFootprintTTL, 64 + 32*(len(lv.Footprint)+len(lv.EnergyFootprint))
	})
	return v.(LiveValues), true
}

func emptyLiveValues() LiveValues {
	return LiveValues{Footprint: map[string]float64{}, EnergyFootprint: map[string]float64{}}
}

func computeLiveFootprint(ctx context.Context, job *schema.Job) LiveValues {
	lv, err := loadLiveFootprint(ctx, job)
	if err != nil {
		cclog.Warnf("live footprint for job %d (cluster %s): %v", job.JobID, job.Cluster, err)
		return emptyLiveValues()
	}
	return lv
}

// loadLiveFootprint queries the metric store once for all footprint and energy
// metrics of the job's subcluster and builds the live values from the result.
func loadLiveFootprint(ctx context.Context, job *schema.Job) (LiveValues, error) {
	sc, err := archive.GetSubCluster(job.Cluster, job.SubCluster)
	if err != nil {
		return LiveValues{}, err
	}

	metrics := make([]string, 0, len(sc.Footprint)+len(sc.EnergyFootprint))
	avgOnly := make(map[string]bool, cap(metrics))
	for _, m := range sc.Footprint {
		statType, err := footprint.StatType(sc, m)
		if err != nil {
			return LiveValues{}, err
		}
		metrics = append(metrics, m)
		avgOnly[m] = statType == "avg"
	}
	for _, m := range sc.EnergyFootprint {
		if _, seen := avgOnly[m]; !seen {
			metrics = append(metrics, m)
			avgOnly[m] = true // energy only uses the average power
		}
	}
	if len(metrics) == 0 {
		return emptyLiveValues(), nil
	}

	repo, err := GetMetricDataRepo(job.Cluster, job.SubCluster)
	if err != nil {
		return LiveValues{}, err
	}

	// The computation is shared by every request waiting on the cache entry, so
	// one caller's cancellation must not fail it for the others.
	qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), liveFootprintTimeout)
	defer cancel()

	release, err := acquireRepo(qctx, repo)
	if err != nil {
		return LiveValues{}, err
	}
	defer release()

	// Query up to now: the persisted duration of a running job may be stale.
	q := *job
	q.Duration = int32(elapsed(job))

	stats, err := repo.LoadStats(&q, metrics, avgOnly, qctx)
	if err != nil {
		return LiveValues{}, err
	}

	folded := footprint.Fold(&q, stats)
	fp, err := footprint.Build(sc, folded, false)
	if err != nil {
		return LiveValues{}, err
	}
	efp, energy := footprint.BuildEnergy(sc, folded, q.NumNodes, q.Duration)

	return LiveValues{Footprint: fp, EnergyFootprint: efp, Energy: energy}, nil
}

// setRepoConcurrency sets the number of concurrent live footprint requests
// allowed against repo.
func setRepoConcurrency(repo MetricDataRepository, n int) {
	repoSemMu.Lock()
	defer repoSemMu.Unlock()
	repoSems[repo] = make(chan struct{}, n)
}

// acquireRepo takes one slot of repo's concurrency limit. Repositories without
// a configured limit (the internal metric store) get GOMAXPROCS slots.
func acquireRepo(ctx context.Context, repo MetricDataRepository) (release func(), err error) {
	repoSemMu.Lock()
	sem, ok := repoSems[repo]
	if !ok {
		sem = make(chan struct{}, runtime.GOMAXPROCS(0))
		repoSems[repo] = sem
	}
	repoSemMu.Unlock()

	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// liveFanOut bounds the goroutines computing live footprints for one batch of
// jobs. The metric store load itself is bounded per store by acquireRepo.
const liveFanOut = 64

// LiveFootprints computes LiveFootprint for all live jobs with bounded
// parallelism and returns the values keyed by database id. Jobs that are not
// live or have no id are left out. Once ctx is cancelled no further jobs are
// scheduled.
func LiveFootprints(ctx context.Context, jobs []*schema.Job) map[int64]LiveValues {
	res := make(map[int64]LiveValues, len(jobs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, liveFanOut)
	for _, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		if job.ID == nil || !IsLive(job) {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			lv, ok := LiveFootprint(ctx, job)
			if !ok {
				return
			}
			mu.Lock()
			res[*job.ID] = lv
			mu.Unlock()
		}()
	}
	wg.Wait()
	return res
}

// ApplyLiveFootprints replaces the persisted footprint, energy footprint and
// total energy of all live jobs by their live values.
func ApplyLiveFootprints(ctx context.Context, jobs ...*schema.Job) {
	live := LiveFootprints(ctx, jobs)
	for _, job := range jobs {
		if job.ID == nil {
			continue
		}
		if lv, ok := live[*job.ID]; ok {
			job.Footprint, job.EnergyFootprint, job.Energy = lv.Footprint, lv.EnergyFootprint, lv.Energy
		}
	}
}
