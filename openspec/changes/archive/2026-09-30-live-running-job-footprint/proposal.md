# Proposal

## Why

The footprint (and energy footprint) of a running job is currently persisted by a
recurring cron service (`footprint-worker`, default 10 minutes) that queries the
metric store for every running job and rewrites the `footprint` column in one
transaction per cluster. Values shown to users are up to one interval stale, the
worker spends metric-store and database write capacity on jobs nobody looks at,
the energy footprint of running jobs is never computed at all, and the worker
carries latent bugs (the min statistic always folds to `<= 0`, the
subcluster-specific footprint statistic override is never applied). Computing
footprints on demand from the metric store removes the staleness and the
background write load, and makes running and finished jobs follow one clear rule:
running jobs are live, finished jobs are final and persisted by the archiver.

## What Changes

- Remove the recurring footprint update worker. Footprints of running jobs are no
  longer written to the database.
- Compute the footprint, energy footprint and total energy of a running job on
  demand from the metric store (internal and external cc-metric-store), shared by
  GraphQL and REST through a single computation path with a short-lived cache.
- Jobs shorter than `main.short-running-jobs-duration` get no live footprint.
  This bounds the work on clusters with many short jobs.
- The GraphQL `Job.footprint`, `Job.energyFootprint` and `Job.energy` fields return
  live values for running jobs. `energy` becomes a resolver-backed field.
- REST job endpoints (`GET /api/jobs/`, `GET/POST /api/jobs/{id}`) return live
  footprint, energy footprint and energy for running jobs.
- Queries that use a footprint feature (footprint sort, `metricStats` filter,
  `energy` filter, `energy` sort, metric histograms) are routed by job state:
  - state set exactly `{running}`: filtered and sorted in memory on live values,
  - state set without `running`: served from the persisted columns in SQL,
  - no state filter: treated as finished jobs only,
  - state set mixing `running` with other states: rejected with an error.
  **BREAKING** for API clients that combine footprint sort/filter/histograms
  with running and finished jobs in one query.
- Metric histograms for running jobs use the same live footprint values as
  filtering, instead of summed node averages.
- The internal metric store's statistics query uses the precomputed per-buffer
  aggregates (`MemoryStore.Stats`) instead of reading full time series.
- Fix the min-statistic fold and the inverted `MetricIndex` check in the footprint
  computation.
- The `cron.footprint-worker` config key is still accepted but ignored, with a
  deprecation warning (the cron config is decoded with unknown fields disallowed).

Out of scope: the UI adaptation that separates the running-jobs view from the
all-jobs view (tracked separately); energy for metrics configured as `"energy"`
(not `"power"`), which remains unimplemented.

## Capabilities

### New Capabilities
- `running-job-footprint`: on-demand computation of footprint, energy footprint
  and total energy for running jobs, its short-job cutoff, caching, failure
  behaviour, and exposure through GraphQL and REST.
- `footprint-queries`: how queries that filter, sort or aggregate on footprint or
  energy values are routed by job state, including the in-memory path for
  running jobs and the safeguard that rejects mixed-state queries.

### Modified Capabilities
<!-- None: no specs exist yet under openspec/specs/. -->

## Impact

- **Code**: `internal/taskmanager` (worker removal, config deprecation),
  `internal/metricdispatch` (live footprint computation and cache),
  `pkg/metricstore/query.go` and `api.go` (stats fast path),
  `internal/metricstoreclient` (shared stats folding), `internal/repository`
  (`jobQuery.go`, `stats.go`, `job.go`: query routing, safeguard, footprint fixes),
  `internal/graph` (resolvers, `gqlgen.yml` for `energy`), `internal/api/job.go`
  (REST enrichment).
- **APIs**: GraphQL and REST return live values for running jobs. Mixed-state
  footprint queries now return an error. Queries without a state filter that use
  footprint features no longer include running jobs.
- **Config**: `cron.footprint-worker` deprecated. `main.short-running-jobs-duration`
  now also acts as the live-footprint cutoff. New optional
  `metric-store-external[].max-concurrent-requests` (default 8) limits
  concurrent live-footprint requests per external metric store.
- **Load**: metric-store load moves from a fixed interval to request time,
  bounded by the cache, the short-job cutoff and per-store concurrency limits.
- **Docs**: `CLAUDE.md`/config documentation, `ReleaseNotes.md`, Swagger text for
  the REST job endpoints (`make swagger`), `make graphql` after `gqlgen.yml` change.
