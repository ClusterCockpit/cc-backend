# Design

## Context

See `proposal.md` for motivation and `specs/` for the required behaviour. The
current state that shapes the approach:

- `internal/taskmanager/updateFootprintService.go` loops over
  `FindRunningJobs` per cluster, calls `MetricDataRepository.LoadStats` per job,
  folds per-host statistics (the min fold starts at `0.0`), and writes only the
  `footprint` column through `JobRepository.UpdateFootprint`. Energy is never
  written for running jobs. It is the only caller of `FindRunningJobs`.
- Footprint maps are built in three copies: `repository.UpdateFootprint`
  (`job.go`), `repository.UpdateEnergy`, and `importer.enrichJobMetadata`
  (`initDB.go`). Both footprint copies invert the `archive.MetricIndex` error
  check, so the subcluster-specific statistic is never applied.
- The footprint filter reaches SQL in `BuildWhereClause` (`jobQuery.go`), which is
  called from `QueryJobs`, `CountJobs` and five query builders in `stats.go`.
  Footprint sort is `ORDER BY json_extract(footprint, ...)` in `QueryJobs`.
  Energy filter and sort use the plain `job.energy` column.
- Running-job metric histograms (`runningJobsMetricStatisticsHistogram`) already
  load live data via `metricdispatch.LoadAverages`, which sums node averages.
- Internal store: `InternalMetricStore.LoadStats` → `metricstore.FetchData`, which
  forces `WithData = true`, reads full series with `MemoryStore.Read`, then runs
  `AddStats`. `MemoryStore.Stats` uses per-buffer cached aggregates but has no
  callers. It is not a drop-in replacement:
  - `Read` sums (or averages) child buffers **per timestep** and only warns on
    misalignment.
  - `Stats` combines the children's own min/max (min of mins, max of maxes) and
    **errors** on misalignment. Its avg is exact; its min/max across aggregated
    children is not.
- `repository` imports `metricdispatch`, so `metricdispatch` cannot import
  `repository`. `importer` imports `repository`.
- gqlgen resolves resolver-backed fields of list elements concurrently. The same
  filter argument slice is visible to the `jobs` resolver's `QueryJobs`,
  `CountJobs` and next-page calls.
- `taskmanager.Start` decodes the cron config with `DisallowUnknownFields`.
- `FetchEnergyFootprint` caches energy footprints for 24 h per job id.
- REST `GET /api/jobs/` accepts any `items-per-page >= 1`.

## Goals / Non-Goals

**Goals:**
- One computation path for live values, used by GraphQL, REST, running-job
  queries and histograms.
- No SQL changes beyond one id-set filter. The existing query builders stay as
  they are.
- Bounded metric-store load for both store kinds.

**Non-Goals:**
- Changing the analysis-view `jobsFootprints` query (it uses archive or
  live averages independently of the footprint column).
- A batch statistics endpoint in the external cc-metric-store.
- Sum statistics for energy metrics configured as `"energy"`.
- Stable ordering of footprint sorts for finished jobs (SQL, unchanged).

## Decisions

### D1: Shared footprint builder package

Create `internal/footprint` (depends only on `pkg/archive` and cc-lib `schema`):

- `Fold(job, perHostStats) map[metric]JobStatistics`: correct fold across the
  job's hosts (mean of averages, min of mins, max of maxes, hosts without data
  skipped, rounded to 2 digits).
- `Build(job, stats) map[string]float64`: resolves the statistic per footprint
  metric (global setting, overridden by subcluster setting) and returns the
  `<metric>_<stat>` map.
- `BuildEnergy(job, stats) (map[string]float64, float64)`: energy footprint and
  total, using the existing power formula.

`repository.UpdateFootprint`/`UpdateEnergy`, `importer.enrichJobMetadata`,
`metricdispatch.LoadJobStats` and the live path all call it. This fixes both bugs
in one place.

*Alternative:* put the builders in `metricdispatch`. Rejected because the
importer and the archiver would then depend on metric-store dispatch code only to
build a map.

### D2: `metricdispatch.LiveFootprint`

```
LiveFootprint(ctx, job) -> LiveValues{Footprint, EnergyFootprint, Energy}, ok

  eligible = (state == running || monitoringStatus == RunningOrArchiving)
             && monitoringStatus != Disabled
             && now - startTime >= config.Keys.ShortRunningJobsDuration
  not eligible            -> empty values, no store access
  cache.Get("livefp:<dbid>", compute)       (own lrucache, TTL 60s)
     compute:
       acquire the semaphore of the job's metric-data repository
       metrics = subcluster.Footprint  U  subcluster.EnergyFootprint
       stats   = repo.LoadStats(job with Duration = now - start, metrics, node scope)
       values  = footprint.Build / BuildEnergy (footprint.Fold(stats))
       on error: log, cache EMPTY values with TTL 10s (negative cache)
```

- `cc-lib/lrucache.Get` already makes concurrent callers for one key wait for a
  single computation. That provides the freshness requirement and prevents
  stampedes without extra code.
- One `LoadStats` call per job serves footprint and energy.
- Each metric-data repository has its own semaphore, shared by all requests
  that reach it:
  - internal store: capacity `GOMAXPROCS`. The work is CPU-bound in the same
    process, so the limit only prevents a large LIVE request from starting
    thousands of goroutines. Not configurable.
  - each external store: capacity from the new optional field
    `metric-store-external[].max-concurrent-requests` (default 8, values below 1
    are rejected at startup). Each job is one HTTP call, and the right limit
    depends on the remote store's capacity and network latency.
- The eligibility rule applies the short-job cutoff before any store access.
- The freshness interval (60 s) and the negative-cache interval (10 s) are
  constants, not configuration. The short-job cutoff is the operator's lever for
  load. The freshness interval is part of the spec and the paging guarantee, and
  going below the usual 30–60 s sampling interval gains nothing.

*Alternative:* cache per `(job, metric)`. Rejected: the per-job call already
fetches every metric, and a per-job key keeps footprint and energy consistent.

*Alternative:* one global semaphore. Rejected: with several external stores, a
busy large cluster would use up the budget and slow down live footprints of
clusters served by other stores. Per-store limits match the existing per-entry
configuration (scope, url, token).

### D3: Statistics fast path in the internal store, only where exact

Add `metricstore.FetchStats(req)` used by `InternalMetricStore.LoadStats`
(`WithData == false`). Per selector:

- **Single buffer** (query has no `Type`, i.e. the metric is node-native): use
  `MemoryStore.Stats`. Avg, min and max are exact.
- **Aggregated** selector with only `avg` required: use `MemoryStore.Stats`. On an
  alignment error, fall back to `Read` + `AddStats`.
- **Aggregated** selector with `min` or `max` required: `Read` + `AddStats`
  (per-timestep semantics, unchanged).
- Apply `ScaleFactor` to avg/min/max.

The per-metric "avg only" hint travels on `APIQuery` in a field tagged
`json:"-"`, so it is never serialized to an external store or exposed on the
metric store's own HTTP API. `LoadStats` gains a per-metric requirement parameter
(`MetricDataRepository` interface change). The external client ignores it. The
live path sets `avg only` for metrics whose footprint statistic is `avg` and that
are used for energy.

*Alternative:* always use `Stats`. Rejected: min/max of node-aggregated metrics
(e.g. core-level `flops_any` summed per node) would change meaning silently, and
alignment errors would drop metrics that `Read` currently tolerates.

### D4: Query routing in the repository

A pure classifier plus one rewrite step, applied at every repository entry point
that accepts `[]*model.JobFilter`: `QueryJobs`, `CountJobs`, `JobsStats`,
`JobsStatsGrouped`, `JobCountGrouped`, `AddJobCountGrouped`, `AddHistograms`,
`AddMetricHistograms`.

```
classify(filters, order, metricHistograms) -> PLAIN | FINISHED | LIVE | error
  feature = any MetricStats || any Energy filter
            || order.Type != "col" || order.Field == "energy"
            || metricHistograms
  !feature                          -> PLAIN
  eff = intersection of State over entries (absent = all states)
  eff == {running}                  -> LIVE
  running not in eff                -> FINISHED
  otherwise                         -> ErrMixedStateFootprintQuery

rewrite:
  PLAIN    -> filters unchanged
  FINISHED -> if no entry has State: append {State: all states except running}
  LIVE     -> 1. candidates: SecurityCheck + filters with MetricStats/Energy
                 stripped, minimal columns
              2. LiveFootprint per candidate (bounded fan-out over D2)
              3. apply MetricStats/Energy ranges in Go, with the exact
                 From/To semantics of buildFloatCondition/buildFloatJSONCondition
              4. append {DbID: matching ids}; drop MetricStats/Energy from copies
              5. footprint/energy sort: sort (value, id) in Go, NULL first ASC /
                 last DESC; page is sliced in Go and rows are loaded by id
```

- Rewrites **copy** the slice and the entries. Callers' filters are never
  mutated, which avoids races between concurrent gqlgen resolvers.
- An empty id set short-circuits to an empty result or zero counts.
- Step 4 reuses `BuildWhereClause`. All seven SQL call sites stay unchanged, so
  list, count and aggregates agree by construction.
- For large id sets `BuildWhereClause` emits
  `job.id IN (SELECT value FROM json_each(?))` (one bind parameter) instead of
  one placeholder per id. This avoids SQLite's variable limit without changing
  `model.JobFilter`.
- The `jobs` GraphQL resolver resolves once and passes the rewritten filters and
  the Go-side order to its `QueryJobs`, `CountJobs` and next-page calls. This
  avoids three candidate scans per request.
- The REST job list has no footprint features, so it is always PLAIN.

*Alternative considered in exploration:* overlay live values into SQL through a
`MATERIALIZED` CTE. It supports mixed-state queries, but needs changes at all
seven sites and is 1000× slower if the CTE is not materialized. Rejected in
favour of separating running and finished queries completely.

### D5: GraphQL resolvers

- `energy` becomes resolver-backed (`gqlgen.yml`, `make graphql`).
- `Footprint`, `EnergyFootprint`, `Energy`:
  - live-eligible job → `LiveFootprint`, errors become empty values;
  - otherwise → persisted values. The footprint comes from `obj.Footprint`
    (already filled by `scanJob`), which removes today's extra DB query per
    row. The energy footprint keeps `FetchEnergyFootprint`.
- Running jobs never go through the 24 h energy-footprint cache.
- When the LIVE query path has already computed values, it stores them on the job
  objects, and the resolvers reuse them.

### D6: REST enrichment

`getJobs`, `getJobByID` and `getCompleteJobByID` fill `Footprint`,
`EnergyFootprint` and `Energy` from `LiveFootprint` for live-eligible jobs before
encoding. List enrichment runs in parallel, limited by the per-store D2
semaphores.
`items-per-page` stays uncapped; the cache and the semaphore bound the cost.

### D7: Running-job metric histograms

`runningJobsMetricStatisticsHistogram` bins `LiveFootprint` values
(`<metric>_<stat>`) instead of `LoadAverages` sums. Jobs without a value are
skipped. The existing 5000-job limit and the peak-based binning stay.

### D8: Worker removal and configuration

- Delete `updateFootprintService.go` and its registration. Delete
  `FindRunningJobs` (no remaining callers) and its tests.
- Keep `CronFrequency.FootprintWorker`. When it's set, log a deprecation warning
  in `Start`.
- Remove `footprint-worker` from the generated default config (`init.go`) and
  document the deprecation in `ReleaseNotes.md` and `CLAUDE.md`.

## Risks / Trade-offs

- **Cold cache on a large running set with an external store**: first sort or
  filter can take seconds (one HTTP call per job, `max-concurrent-requests` in
  flight, default 8). → Short-job cutoff, 60 s cache shared by all users,
  per-store tunable concurrency, timing log per LIVE resolution, request context
  cancellation stops pending computations.
- **Aggregated-metric fast path inexact for min/max** → D3 uses it only where the
  result is exact; a test compares fast path and full read on fixtures for each
  aggregation kind.
- **Paging drift across the freshness interval**: energy grows continuously, so
  pages fetched more than 60 s apart can overlap. → Documented. Tie-break by id
  keeps pages exact within the interval.
- **Breaking change for mixed-state footprint queries** → Clear error message;
  release note; the UI no longer sends such queries.
- **No state filter now excludes running jobs from footprint features**
  (analysis, user and list views before the UI change) → Matches the new model;
  release note.
- **Stale persisted footprints of jobs running at upgrade time**: the LIVE path
  ignores the column, and the archiver overwrites it at stop. Jobs whose
  archiving fails keep their last cron value (same as today).
- **Stop-to-archive gap in FINISHED queries**: jobs between stop and archiving
  have no persisted value and briefly don't match footprint filters. → Accepted;
  they are still displayed with live values.
- **Memory per LIVE request** for thousands of candidates → Minimal column set
  for candidates; full rows only for the page.
- **Interface change of `MetricDataRepository.LoadStats`** → Both implementations
  and all callers (`LoadJobStats`, `LoadAverages`) updated in the same change.

## Migration Plan

1. Deploy the new binary. No database migration.
2. Existing configs with `cron.footprint-worker` keep loading (warning only).
   Existing `metric-store-external` entries without `max-concurrent-requests`
   get the default of 8.
3. Rollback: deploy the previous binary. The cron worker resumes and refills
   the `footprint` column of running jobs on its first run. Finished jobs are
   unaffected in both directions.
