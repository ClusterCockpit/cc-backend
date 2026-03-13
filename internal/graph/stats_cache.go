// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package graph

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
)

// statsGroupCache is a per-request cache for grouped JobsStatistics results.
// It deduplicates identical (filter+groupBy) SQL queries that arise when the
// frontend requests multiple sort/page slices of the same underlying data
// (e.g. topUserJobs, topUserNodes, topUserAccs all group by USER).
type statsGroupCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	once   sync.Once
	result []*model.JobsStatistics
	err    error
}

type ctxKey int

const statsGroupCacheKey ctxKey = iota

// newStatsGroupCache creates a new empty cache.
func newStatsGroupCache() *statsGroupCache {
	return &statsGroupCache{
		entries: make(map[string]*cacheEntry),
	}
}

// WithStatsGroupCache injects a new cache into the context.
func WithStatsGroupCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, statsGroupCacheKey, newStatsGroupCache())
}

// getStatsGroupCache retrieves the cache from context, or nil if absent.
func getStatsGroupCache(ctx context.Context) *statsGroupCache {
	if c, ok := ctx.Value(statsGroupCacheKey).(*statsGroupCache); ok {
		return c
	}
	return nil
}

// cacheKey builds a deterministic string key from filter + groupBy.
func statsCacheKey(filter []*model.JobFilter, groupBy *model.Aggregate) string {
	return fmt.Sprintf("%v|%v", filter, *groupBy)
}

// getOrCompute returns cached results for the given key, computing them on
// first access via the provided function.
func (c *statsGroupCache) getOrCompute(
	key string,
	compute func() ([]*model.JobsStatistics, error),
) ([]*model.JobsStatistics, error) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &cacheEntry{}
		c.entries[key] = entry
	}
	c.mu.Unlock()

	entry.once.Do(func() {
		entry.result, entry.err = compute()
	})
	return entry.result, entry.err
}

// sortAndPageStats sorts a copy of allStats by the given sortBy field (descending)
// and returns the requested page slice.
func sortAndPageStats(allStats []*model.JobsStatistics, sortBy *model.SortByAggregate, page *model.PageRequest) []*model.JobsStatistics {
	// Work on a shallow copy so the cached slice order is not mutated.
	sorted := make([]*model.JobsStatistics, len(allStats))
	copy(sorted, allStats)

	if sortBy != nil {
		getter := statsFieldGetter(*sortBy)
		slices.SortFunc(sorted, func(a, b *model.JobsStatistics) int {
			return getter(b) - getter(a) // descending
		})
	}

	if page != nil && page.ItemsPerPage != -1 {
		start := (page.Page - 1) * page.ItemsPerPage
		if start >= len(sorted) {
			return nil
		}
		end := start + page.ItemsPerPage
		if end > len(sorted) {
			end = len(sorted)
		}
		sorted = sorted[start:end]
	}

	return sorted
}

// statsFieldGetter returns a function that extracts the sortable int field
// from a JobsStatistics struct for the given sort key.
func statsFieldGetter(sortBy model.SortByAggregate) func(*model.JobsStatistics) int {
	switch sortBy {
	case model.SortByAggregateTotaljobs:
		return func(s *model.JobsStatistics) int { return s.TotalJobs }
	case model.SortByAggregateTotalusers:
		return func(s *model.JobsStatistics) int { return s.TotalUsers }
	case model.SortByAggregateTotalwalltime:
		return func(s *model.JobsStatistics) int { return s.TotalWalltime }
	case model.SortByAggregateTotalnodes:
		return func(s *model.JobsStatistics) int { return s.TotalNodes }
	case model.SortByAggregateTotalnodehours:
		return func(s *model.JobsStatistics) int { return s.TotalNodeHours }
	case model.SortByAggregateTotalcores:
		return func(s *model.JobsStatistics) int { return s.TotalCores }
	case model.SortByAggregateTotalcorehours:
		return func(s *model.JobsStatistics) int { return s.TotalCoreHours }
	case model.SortByAggregateTotalaccs:
		return func(s *model.JobsStatistics) int { return s.TotalAccs }
	case model.SortByAggregateTotalacchours:
		return func(s *model.JobsStatistics) int { return s.TotalAccHours }
	default:
		return func(s *model.JobsStatistics) int { return s.TotalJobs }
	}
}
