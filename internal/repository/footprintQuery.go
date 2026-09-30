// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file routes job queries that filter, sort or aggregate on footprint and
// energy values. Running jobs have live values computed from the metric store
// (metricdispatch.LiveFootprint), finished jobs have values persisted by the
// archiver, and a query must not mix the two:
//
//   - no footprint feature:           answered as is (PLAIN)
//   - effective states exactly running: filtered and sorted on live values (LIVE)
//   - effective states without running: SQL on the persisted columns (FINISHED)
//   - running together with others:   ErrMixedStateFootprintQuery
//
// A query without any state filter is treated as FINISHED. The LIVE rewrite
// resolves the footprint and energy filters to a set of job ids, so all SQL
// query builders keep working unchanged on the rewritten filters.

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
)

// ErrMixedStateFootprintQuery is returned for queries that use a footprint
// feature on running and finished jobs at the same time.
var ErrMixedStateFootprintQuery = errors.New("footprint queries cannot mix running and finished jobs: filter for state 'running' only, or exclude it")

// liveCandidateScans counts the candidate scans of LIVE footprint queries.
var liveCandidateScans atomic.Int64

// LiveCandidateScans returns the number of candidate scans LIVE footprint
// queries have performed. Tests use it to verify that callers route a request
// once instead of once per query.
func LiveCandidateScans() int64 {
	return liveCandidateScans.Load()
}

type footprintMode int

const (
	footprintPlain footprintMode = iota
	footprintFinished
	footprintLive
)

// finishedStates are all job states except running, used when a footprint
// query has no state filter.
var finishedStates = []schema.JobState{
	schema.JobStateBootFail, schema.JobStateCancelled, schema.JobStateCompleted,
	schema.JobStateDeadline, schema.JobStateFailed, schema.JobStateNodeFail,
	schema.JobStateOutOfMemory, schema.JobStatePending, schema.JobStatePreempted,
	schema.JobStateSuspended, schema.JobStateTimeout,
}

// FootprintQuery is a filter list routed by classifyFootprintQuery. Its
// Filters can be passed to every query builder; for LIVE queries with a
// footprint or energy sort, SortedIDs holds the complete ordered result.
type FootprintQuery struct {
	mode    footprintMode
	Filters []*model.JobFilter
	// Order is the order to apply in SQL; nil when SortedIDs is set.
	Order *model.OrderByInput
	// SortedIDs is the ordered list of matching job ids for LIVE footprint sorts.
	SortedIDs []int64
	// Live holds the live values of the matching jobs of a LIVE query.
	Live map[int64]metricdispatch.LiveValues
	// Empty is set when a LIVE query matched no job.
	Empty bool
}

// isFootprintOrder reports whether order sorts on a footprint or energy value.
func isFootprintOrder(order *model.OrderByInput) bool {
	return order != nil && (order.Type != "col" || toSnakeCase(order.Field) == "energy")
}

func hasFootprintFilter(filters []*model.JobFilter) bool {
	for _, f := range filters {
		if f != nil && (len(f.MetricStats) > 0 || f.Energy != nil) {
			return true
		}
	}
	return false
}

// effectiveStates returns the intersection of the state filters of all
// entries. restricted is false when no entry has a state filter.
func effectiveStates(filters []*model.JobFilter) (states map[schema.JobState]bool, restricted bool) {
	for _, f := range filters {
		if f == nil || f.State == nil {
			continue
		}
		cur := make(map[schema.JobState]bool, len(f.State))
		for _, s := range f.State {
			if !restricted || states[s] {
				cur[s] = true
			}
		}
		states, restricted = cur, true
	}
	return states, restricted
}

// classifyFootprintQuery decides how a query is answered, see the file comment.
func classifyFootprintQuery(filters []*model.JobFilter, order *model.OrderByInput, metricHistograms bool) (footprintMode, error) {
	if !metricHistograms && !isFootprintOrder(order) && !hasFootprintFilter(filters) {
		return footprintPlain, nil
	}

	states, restricted := effectiveStates(filters)
	if !restricted {
		return footprintFinished, nil
	}
	if !states[schema.JobStateRunning] {
		return footprintFinished, nil
	}
	if len(states) == 1 {
		return footprintLive, nil
	}
	return footprintPlain, ErrMixedStateFootprintQuery
}

// RouteFootprintQuery classifies and rewrites a filter list. The returned
// filters are copies; the caller's filters are never modified. metricHistograms
// marks queries that request metric histograms.
func (r *JobRepository) RouteFootprintQuery(
	ctx context.Context,
	filters []*model.JobFilter,
	order *model.OrderByInput,
	metricHistograms bool,
) (*FootprintQuery, error) {
	mode, err := classifyFootprintQuery(filters, order, metricHistograms)
	if err != nil {
		return nil, err
	}

	switch mode {
	case footprintFinished:
		fq := &FootprintQuery{mode: mode, Filters: filters, Order: order}
		if _, restricted := effectiveStates(filters); !restricted {
			fq.Filters = append(slices.Clone(filters), &model.JobFilter{State: finishedStates})
		}
		return fq, nil
	case footprintLive:
		return r.routeLive(ctx, filters, order)
	default:
		return &FootprintQuery{mode: mode, Filters: filters, Order: order}, nil
	}
}

// routeLive computes live values for all running jobs matching the filters
// without their footprint and energy parts, applies those parts in Go and
// replaces them by the set of matching job ids.
func (r *JobRepository) routeLive(ctx context.Context, filters []*model.JobFilter, order *model.OrderByInput) (*FootprintQuery, error) {
	base := make([]*model.JobFilter, 0, len(filters)+1)
	var metricStats []*model.MetricStatItem
	var energy []*model.FloatRange
	for _, f := range filters {
		if f == nil {
			continue
		}
		c := *f
		metricStats = append(metricStats, c.MetricStats...)
		if c.Energy != nil {
			energy = append(energy, c.Energy)
		}
		c.MetricStats, c.Energy = nil, nil
		base = append(base, &c)
	}

	candidates, err := r.liveCandidates(ctx, base)
	if err != nil {
		return nil, err
	}
	live := metricdispatch.LiveFootprints(ctx, candidates)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	matching := make([]int64, 0, len(candidates))
	for _, job := range candidates {
		lv := live[*job.ID]
		if matchesLive(lv, metricStats, energy) {
			matching = append(matching, *job.ID)
		} else {
			delete(live, *job.ID)
		}
	}

	fq := &FootprintQuery{mode: footprintLive, Live: live, Order: order, Empty: len(matching) == 0}
	if isFootprintOrder(order) {
		sortLive(matching, live, order)
		fq.SortedIDs = matching
		fq.Order = nil
	}

	ids := make([]string, len(matching))
	for i, id := range matching {
		ids[i] = strconv.FormatInt(id, 10)
	}
	fq.Filters = append(base, &model.JobFilter{DbID: ids})
	return fq, nil
}

// liveCandidates loads the columns LiveFootprint needs for all jobs matching
// filters that the requesting user may see.
func (r *JobRepository) liveCandidates(ctx context.Context, filters []*model.JobFilter) ([]*schema.Job, error) {
	liveCandidateScans.Add(1)
	query, err := SecurityCheck(ctx, sq.Select(
		"job.id", "job.job_id", "job.cluster", "job.subcluster", "job.start_time",
		"job.num_nodes", "job.num_acc", "job.job_state", "job.monitoring_status", "job.resources",
	).From("job"))
	if err != nil {
		return nil, err
	}
	for _, f := range filters {
		query = BuildWhereClause(f, query)
	}

	rows, err := query.RunWith(r.stmtCache).QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("live footprint candidates: %w", err)
	}
	defer rows.Close()

	jobs := make([]*schema.Job, 0, defaultJobsCapacity)
	for rows.Next() {
		job := &schema.Job{}
		var id int64
		var rawResources []byte
		if err := rows.Scan(&id, &job.JobID, &job.Cluster, &job.SubCluster, &job.StartTime,
			&job.NumNodes, &job.NumAcc, &job.State, &job.MonitoringStatus, &rawResources); err != nil {
			return nil, fmt.Errorf("live footprint candidates: %w", err)
		}
		job.ID = &id
		if err := json.Unmarshal(rawResources, &job.Resources); err != nil {
			cclog.Warnf("live footprint candidates: resources of job %d: %v", id, err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// inFloatRange mirrors buildFloatCondition and buildFloatJSONCondition: a zero
// bound is open, and a range with both bounds zero does not constrain.
func inFloatRange(v float64, rng *model.FloatRange) bool {
	switch {
	case rng.From > 0 && rng.To > 0:
		return v >= rng.From && v <= rng.To
	case rng.From > 0:
		return v >= rng.From
	case rng.To > 0:
		return v <= rng.To
	default:
		return true
	}
}

// liveEnergy returns the total energy of lv; ok is false when no energy could
// be computed (short job, no data or store failure).
func liveEnergy(lv metricdispatch.LiveValues) (float64, bool) {
	return lv.Energy, len(lv.EnergyFootprint) > 0
}

// matchesLive applies footprint and energy filters to live values. A job
// without a value for a filtered metric does not match a constraining range.
func matchesLive(lv metricdispatch.LiveValues, metricStats []*model.MetricStatItem, energy []*model.FloatRange) bool {
	for _, ms := range metricStats {
		if ms == nil || ms.Range == nil {
			continue
		}
		if !validMetricName.MatchString(ms.MetricName) {
			return false
		}
		unconstrained := ms.Range.From <= 0 && ms.Range.To <= 0
		v, ok := lv.Footprint[ms.MetricName]
		if !unconstrained && (!ok || !inFloatRange(v, ms.Range)) {
			return false
		}
	}
	for _, rng := range energy {
		unconstrained := rng.From <= 0 && rng.To <= 0
		v, ok := liveEnergy(lv)
		if !unconstrained && (!ok || !inFloatRange(v, rng)) {
			return false
		}
	}
	return true
}

// liveSortValue returns the value a footprint or energy order sorts on.
func liveSortValue(lv metricdispatch.LiveValues, order *model.OrderByInput) (float64, bool) {
	if order.Type == "col" {
		return liveEnergy(lv)
	}
	v, ok := lv.Footprint[toSnakeCase(order.Field)]
	return v, ok
}

// sortLive orders ids by their live sort value, ties broken by id. Jobs without
// a value come first in ascending and last in descending order, as NULLs do
// in the SQL path.
func sortLive(ids []int64, live map[int64]metricdispatch.LiveValues, order *model.OrderByInput) {
	desc := order.Order == model.SortDirectionEnumDesc
	sort.Slice(ids, func(i, j int) bool {
		vi, oki := liveSortValue(live[ids[i]], order)
		vj, okj := liveSortValue(live[ids[j]], order)
		switch {
		case oki != okj:
			// Missing values: first in ascending, last in descending order.
			return oki == desc
		case oki && vi != vj:
			if desc {
				return vi > vj
			}
			return vi < vj
		default:
			return ids[i] < ids[j]
		}
	})
}

// applyLive stores the live values of a LIVE query on the job objects, so that
// resolvers and API handlers do not compute them again.
func (fq *FootprintQuery) applyLive(jobs []*schema.Job) {
	if fq == nil || fq.Live == nil {
		return
	}
	for _, job := range jobs {
		if job.ID == nil {
			continue
		}
		if lv, ok := fq.Live[*job.ID]; ok {
			job.Footprint = lv.Footprint
			job.EnergyFootprint = lv.EnergyFootprint
			job.Energy = lv.Energy
		}
	}
}
