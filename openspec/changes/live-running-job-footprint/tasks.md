# Tasks

## 1. Shared footprint builder (D1)

- [x] 1.1 Create `internal/footprint` with `Fold` (mean of node averages, min of mins, max of maxes, hosts without data skipped, 2-digit rounding) and verify with table tests, including the positive-minimum case (12.5, 8.25 → 8.25) and the average case (100, 50 → 75)
- [x] 1.2 Add `Build` resolving the statistic per footprint metric (global setting, overridden by subcluster setting) and verify a test where global `avg` and subcluster `max` yields `<metric>_max`
- [x] 1.3 Add `BuildEnergy` (power formula, `"energy"` metrics report 0, total = sum) and verify with a test against hand-computed kWh values
- [x] 1.4 Switch `repository.UpdateFootprint`, `repository.UpdateEnergy` and `importer.enrichJobMetadata` to the builders, removing both inverted `MetricIndex` checks; verify `go test ./internal/repository/... ./internal/importer/... ./internal/archiver/...` passes
- [x] 1.5 Switch `metricdispatch.LoadJobStats` to `footprint.Fold` and verify `go test ./internal/metricdispatch/...` passes with a new case asserting a non-zero min

## 2. Internal metric store statistics fast path (D3)

- [x] 2.1 Add a `json:"-"` avg-only hint to `metricstore.APIQuery` and extend `MetricDataRepository.LoadStats` with the per-metric requirement; update both implementations and all callers; verify `go build ./...` and `go vet ./...` succeed
- [x] 2.2 Implement `metricstore.FetchStats` (Stats for single-buffer selectors, Stats for avg-only aggregated selectors with fallback to Read on alignment errors, Read + AddStats otherwise, ScaleFactor applied) and use it from `InternalMetricStore.LoadStats`; verify with a test comparing FetchStats against the FetchData result for node-native, sum-aggregated and avg-aggregated metrics
- [x] 2.3 Add a test proving the aggregated min/max path still uses per-timestep semantics (sum of cores per timestep), and a test for the misalignment fallback; verify `go test ./pkg/metricstore/...` passes

## 3. Live footprint computation (D2)

- [x] 3.1 Implement `metricdispatch.LiveFootprint` with eligibility (running or RunningOrArchiving, monitoring not disabled, elapsed ≥ `short-running-jobs-duration`), one `LoadStats` call for footprint ∪ energy metrics, and the builders; verify unit tests with a fake `MetricDataRepository` for eligible, below-cutoff (no store call) and stopped-not-archived jobs
- [x] 3.2 Add optional `max-concurrent-requests` to `CCMetricStoreConfig` and the `metric-store-external` config schema (default 8, values below 1 rejected at `metricdispatch.Init`); verify config tests that an entry without the field gets 8, an explicit value is used, and 0 fails initialisation
- [x] 3.3 Give each metric-data repository its own semaphore (`GOMAXPROCS` for the internal store, the configured value per external store) and acquire it in `LiveFootprint`; verify a test with two fake repositories where saturating one does not block calls to the other, and a test that in-flight calls never exceed the limit
- [x] 3.4 Add the dedicated lrucache (constant 60 s TTL, constant 10 s negative TTL on errors); verify a test where concurrent calls for one job trigger exactly one store call, and a test where a store error yields empty values and no error
- [x] 3.5 Add a test asserting that total energy increases between two computations spaced beyond the TTL (fake clock or TTL override) under constant power

## 4. Query routing and safeguard (D4)

- [x] 4.1 Implement the classifier (features, state intersection, PLAIN/FINISHED/LIVE/`ErrMixedStateFootprintQuery`) and verify table tests covering every feature kind, intersection, no-state and mixed cases
- [x] 4.2 Implement the FINISHED rewrite (append all-states-except-running when no state is set) and verify with an in-memory SQLite test that a footprint sort without state excludes running jobs
- [x] 4.3 Implement the LIVE rewrite: candidate query with SecurityCheck and stripped filters, bounded fan-out over `LiveFootprint`, Go-side range predicates with the same From/To rules as `buildFloatCondition`/`buildFloatJSONCondition`, appended DbID filter, copies instead of mutation; verify tests for metricStats and energy filters, the empty-match short circuit, and a regular user seeing only own jobs
- [x] 4.4 Emit `job.id IN (SELECT value FROM json_each(?))` in `BuildWhereClause` for large DbID sets and verify a test with more ids than SQLite's variable limit
- [x] 4.5 Implement the Go-side footprint and energy sort (value, then id; NULL first ASC / last DESC) with page slicing and loading the page rows by id; verify tests for ordering, ties and two consecutive pages without overlap or gaps
- [x] 4.6 Apply the classifier and rewrite at `QueryJobs`, `CountJobs`, `JobsStats`, `JobsStatsGrouped`, `JobCountGrouped`, `AddJobCountGrouped`, `AddHistograms`, `AddMetricHistograms`; verify a test where count, list paging and grouped per-user counts agree for a running-only metricStats filter
- [x] 4.7 Resolve once in the `jobs` GraphQL resolver and pass the result to its QueryJobs, CountJobs and next-page calls; verify with a counting fake that one request performs one candidate scan
- [x] 4.8 Verify with `go test -race ./internal/repository/... ./internal/graph/...` that concurrent resolvers sharing one filter slice do not race

## 5. Running-job histograms (D7)

- [x] 5.1 Switch `runningJobsMetricStatisticsHistogram` to `LiveFootprint` values (`<metric>_<stat>`, jobs without value skipped) and verify a test where a two-node job is binned by its mean, not the sum of node averages

## 6. GraphQL and REST exposure (D5, D6)

- [x] 6.1 Mark `Job.energy` as resolver-backed in `gqlgen.yml`, run `make graphql`, and verify `go build ./...` succeeds
- [x] 6.2 Implement the `Footprint`, `EnergyFootprint` and `Energy` resolvers (live for eligible jobs with empty-on-error, `obj.Footprint` for finished jobs, no 24 h cache for running jobs, reuse of values stored by the LIVE path); verify resolver tests for running, finished and store-failure cases
- [x] 6.3 Enrich `getJobs`, `getJobByID` and `getCompleteJobByID` with live values for eligible jobs; verify httptest cases in `internal/api` asserting a non-empty `footprint` for a running job above the cutoff and an empty one below it
- [x] 6.4 Update the Swagger annotations for the three REST endpoints to describe live values, run `make swagger`, and verify `internal/api/docs.go` and `api/swagger.yaml` are regenerated

## 7. Worker removal and configuration (D8)

- [x] 7.1 Delete `internal/taskmanager/updateFootprintService.go`, its registration in `Start`, and `FindRunningJobs`; verify `go build ./...` and `go test ./internal/taskmanager/...` pass
- [x] 7.2 Keep `CronFrequency.FootprintWorker`, log a deprecation warning when set, and remove the key from the default config in `cmd/cc-backend/init.go`; verify a taskmanager test that `{"footprint-worker": "10m"}` decodes without error and schedules no footprint task

## 8. Documentation and final verification

- [x] 8.1 Document live footprints, the short-job cutoff, the mixed-state error, the no-state fallback, the deprecated key and `metric-store-external[].max-concurrent-requests` (default 8) in `CLAUDE.md` and `ReleaseNotes.md`; verify both files mention `footprint-worker` as deprecated and describe the new field
- [x] 8.2 Run `make test` and `go vet ./...` and verify both succeed
- [x] 8.3 Start `./cc-backend -server -dev` against demo data with running jobs and verify via GraphQL: live footprint on a running job, running-only footprint sort and filter work, a mixed-state footprint sort returns the mixed-state error, and a footprint sort without state returns only finished jobs
