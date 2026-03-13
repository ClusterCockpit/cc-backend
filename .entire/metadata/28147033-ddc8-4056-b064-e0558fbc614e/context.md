# Session Context

Session ID: 28147033-ddc8-4056-b064-e0558fbc614e
Commit Message: There are memory leaks in production on queries that trigger a full scan

## Prompts

### Prompt 1

There are memory leaks in production on queries that trigger a full scan. Review all queries for missing close and other bugs that could cause memory leaks in the sqlite-3 driver. Example for a query that triggers a massive allocation is a specific jobstate list with sorting on start_time on all jobs.

## Summary

All 31 `.Query()` calls now have proper `rows.Close()`. Build passes, tests pass, vet clean.

### Summary of changes

**`internal/repository/stats.go`** (7 fixes + 1 rewrite):
- Added `defer rows.Close()` + `rows.Err()` to `JobsStatsGrouped`, `JobCountGrouped`, `AddJobCountGrouped`, `jobsStatisticsHistogram`, `jobsDurationStatisticsHistogram`, `jobsMetricStatisticsHistogram`
- Rewrote `AddJobCount` to use `QueryRow()` instead of `Query()` (single-row result)

**`internal/repository/tags.go`** (5 fixes):
- Added `defer xrows.Close()` + `xrows.Err()` for `Queryx` in `CountTags`
- Added `defer rows.Close()` for second query in `CountTags`
- Added `defer rows.Close()` + `rows.Err()` to `GetTags`, `GetTagsDirect`, `getArchiveTags`

**`internal/repository/node.go`** (4 fixes + 1 rewrite):
- Added `defer rows.Close()` + `rows.Err()`, removed manual `rows.Close()` in `QueryNodes`, `QueryNodesWithMeta`, `CountStatesTimed`
- Rewrote `CountStates` to use `COUNT(*) ... GROUP BY` in SQL instead of fetching every row and counting in Go
