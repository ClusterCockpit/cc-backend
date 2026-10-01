# Design

## Context

See proposal.md for the motivation, and specs/device-metric-scopes/spec.md for the required behavior.

Current state that shapes the approach:

- **Shared builder.** `pkg/metricstore/scopequery.go` `BuildScopeQueries` is the single scope-transformation algorithm. It is shared by four query builders:
  - `buildQueries` and `buildNodeQueries` in `pkg/metricstore/query.go`
  - their twins in `internal/metricstoreclient/cc-metric-store-queries.go`

  It returns `(results, ok)`. An empty result is an expected exception; `ok=false` makes the caller fail the whole request.
- **Accelerator branches.** The builder handles accelerators in two dedicated branches (accelerator→accelerator, accelerator→node) that take an `accelerators []string` argument. Every other native scope is a CPU scope resolved through the topology.
- **The effective scope is `native.Max(requested)`.** cc-lib v2.15 gives all device scopes granularity 1, below `hwthread`. So for a CPU metric, `Max(cpuScope, anyDevice)` is the CPU scope, and the request falls through to a CPU branch that returns native-scope data. That is the fallback the spec removes.
- **De-duplication by effective scope.** Each builder loop skips repeated effective scopes (`handledScopes`), keyed by `native.Max(requested)`, before calling the builder. Callers also pre-skip accelerator metrics: `job.NumAcc == 0` in the job builders, and no topology accelerators in the node builders.
- **Store layout.** The metric store keeps device data as a child level of the host, keyed `type+id` (for example `filesystem/scratch`). A query selector is built the same way from `Type` and each `TypeIds` entry. Node aggregation of a selector group uses the metric's configured aggregation (`sum`/`avg`); any other aggregation is an error (`metricstore.go:665-673`).
- **What cc-lib v2.15 provides.** `MetricScope.IsDevice()`, `Topology.GetDeviceIDs(scope)`, the topology lists, and the removal of `JobData.Groups`, `ScopedJobStats.Groups` and `JobStatisticsSet.Groups`.

## Goals / Non-Goals

**Goals:**
- One code path for all device scopes in the shared builder, so that adding a fourth device type needs only a cc-lib scope constant and topology list.
- Callers stay unaware of which device type they handle.
- No behavior change for CPU-native metrics requested at CPU or node scopes.

**Non-Goals:**
- Discovering device instances from the metric store. Instances are declared in the topology; mount points are assumed static.
- Changing how the store stores or aggregates device levels, or tag handling at line-protocol ingestion.
- Any GraphQL schema change. `MetricScope` is a scalar bound to cc-lib.

## Decisions

### D1: One device branch in `BuildScopeQueries`, driven by `IsDevice()`

Replace the two accelerator branches with one branch taken when `nativeScope.IsDevice()`:

```
native.IsDevice():
  len(deviceIDs) == 0              -> empty, ok        (nothing declared/allocated)
  requested == native              -> Type=native, TypeIds=deviceIDs, Aggregate=false, Scope=native
  requested == node                -> Type=native, TypeIds=deviceIDs, Aggregate=true,  Scope=node
  otherwise                        -> empty, ok        (CPU scope or other device)
!native.IsDevice() && requested.IsDevice()
                                   -> empty, ok        (no fallback to native CPU scope)
```

The branch compares `requested` directly and does not use `Max()`. This avoids the equal-granularity ambiguity between device scopes and the fall-through into CPU branches. `Type` is `string(nativeScope)`, which matches the store's level-key prefix and the `type`/`type-id` ingestion convention that accelerators already use. The existing `AcceleratorString` variable is kept as the `Type` for accelerators, and matching variables are added for the new scopes, keeping the pre-converted strings that avoid allocations.

**Alternative considered:** separate branches per device type, copying the accelerator pair. Rejected: the three types are behaviorally identical, and copies would drift apart.

### D2: Callers resolve device ids, and the builder parameter becomes `deviceIDs`

`accelerators []string` is renamed to `deviceIDs []string`. It means "the instance ids of the native device scope for this host". Callers pass:

- **Job builders:** `host.Accelerators` when the native scope is accelerator (allocation-specific), otherwise `topology.GetDeviceIDs(native)`.
- **Node builders:** `topology.GetDeviceIDs(native)`. For accelerators this equals today's `topology.GetAcceleratorIDs()`.

A small helper in `scopequery.go`, `DeviceIDs(native, topology, allocatedAccelerators)`, keeps the four callers identical. The node builders already resolve the topology per node when no subcluster is given, so per-node subcluster lookup (spec: "Node views use the node's subcluster") needs no new code.

**Alternative considered:** have the builder read ids from the topology itself. Rejected: the job path needs allocated accelerators rather than all of them, and the topology is already passed alongside.

### D3: Skip non-matching device requests before de-duplication

With strict semantics, a device request on a CPU metric yields nothing. But its `Max()` effective scope is the CPU native scope, which the `handledScopes` de-duplication would then record as handled. A later explicit request for that CPU scope in the same call, for example `[filesystem, hwthread]`, would then be dropped.

All four loops therefore skip a requested scope before de-duplication when `requested.IsDevice() && requested != native`. The same check replaces the `NumAcc == 0` and empty-accelerator pre-skips with one rule: `native.IsDevice() && len(DeviceIDs(...)) == 0`. The builder keeps its own empty-result guards (D1), so correctness does not depend on the callers' pre-skips; they only avoid building empty query slices.

**Alternative considered:** key de-duplication by the builder's returned scope. Rejected: it needs the builder called before the de-duplication check, which reorders all four loops for no gain.

### D4: The archiver requests device scopes from the declaration

`ArchiveJob` already appends `accelerator` when `job.NumAcc > 0`. The scope list moves into a pure helper, `archiveScopes`, so it can be tested without loading data (D6). It additionally appends `filesystem` and `network` when the job's subcluster declares instances of that type (`archive.GetSubCluster`). This is independent of the node-count limit that gates `core`. Statistics keep coming from `jobData.Metrics[m][node]` (`archiver.go:68`), and node scope is always requested, so device metrics get statistics and footprints from the node total with no further change.

A subcluster lookup failure is logged, and the job is archived without device scopes rather than failing, matching how a missing cluster config is tolerated elsewhere in the archiver.

**Alternative considered:** always request both device scopes. It would be harmless (D1 returns empty for undeclared types), but it adds a loop pass per metric for every archived job on clusters without devices.

### D5: Remove Groups plumbing without replacement

- `pkg/archive/json.go` `DecodeJobStats` and `pkg/archive/parquet/convert.go` lose their group handling.
- `internal/metricdispatch/dataLoader.go` `deepCopy` becomes `copyScopedMetrics` on `Metrics` alone, and the filter no longer copies `Groups`.

Device data is ordinary `Metrics` content, so resampling, filtering, `AddStatisticsSeries` and `RoundMetricStats` cover it without changes. That fixes the gaps where group data was not resampled or filtered.

### D6: Testing strategy

- **`BuildScopeQueries` table tests.** Extend the table with every device-scope case from D1, with a topology fixture declaring accelerators, filesystems and networks. This includes the no-fallback cases: `hwthread`, `core` and `node` native with each device requested, which must return empty with `ok=true`. Extend `TestBuildScopeQueries_UnhandledCase` to iterate over all scopes, including the device ones.
- **Builder-loop tests** for `buildQueries` and `buildNodeQueries` on both stores. Neither has tests today, and both read topology through the global archive. The tests therefore initialize a temporary file archive (`archive.Init` on a temp dir holding `version.txt` and a `cluster.json`) with two subclusters that declare different filesystems, and one that declares none. The external client resolves topology through its own `getTopology`/`getTopologyByNode`, backed by the same archive. Cases:
  - `[filesystem, hwthread]` on an `hwthread` metric still yields the hardware-thread queries (D3)
  - per-node subclusters with different filesystem lists
  - a subcluster without filesystems yields no queries and no error
- **Archiver test.** `ArchiveJob` has no tests and pulls data through `metricdispatch`. The scope selection is therefore extracted into a pure function, `archiveScopes(job, subCluster)`, and tested:
  - a subcluster declaring filesystems gets `filesystem` for a job with more than 8 nodes
  - a GPU job keeps `accelerator`
  - a job on a subcluster without devices gets only `node` and `core`

  The strict builder test covers the absence of hardware-thread series for large GPU jobs.
- **Archive decoding.** A top-level `filesystems` array is a decode error. Device-scoped job data round-trips through the fs, SQLite and Parquet paths.

## Risks / Trade-offs

- [Node list and single-node GPU job rows switch from hardware-thread to core plots for CPU metrics] → Accepted (strict rule chosen). The frontend UI change can request `hwthread` explicitly if that view is wanted. Called out in the release notes.
- [Device metrics configured without `aggregation: sum|avg` fail node-scope queries with "invalid aggregation"] → Document in `CLAUDE.md` next to the topology lists. Validation at config load stays out of scope, since the store already rejects it at query time.
- [Collectors must tag device data `type=filesystem|network,type-id=<id>`. The line decoder keeps `stype`/`stype-id` only below a non-node `type`, so `type=node,stype=filesystem,…` (the originally planned convention), `device=` or `filesystem=` tags produce no device-level data: they write into the host buffer, where several devices overwrite each other. Verified in-process: `type`/`type-id` lines read back per mount and sum at node scope] → Collector-side normalization is out of scope here. Documented in `CLAUDE.md` as a deployment prerequisite. `type`/`type-id` was chosen over teaching the decoder the `stype` form, so the ingest hot path stays unchanged.
- [The external cc-metric-store serves device scopes only once it is upgraded] → It embeds cc-backend's `pkg/metricstore` (currently v1.5.3) for ingestion and queries, which treat `type` as an opaque level-key prefix. Once cc-metric-store is upgraded to a cc-backend release containing this change, it behaves like the internal store. The upgrade and its verification are out of scope here; the cc-backend client side is covered by the builder tests.
- [Device series multiply the series count (nodes × instances) for large jobs] → Bounded by the declared instance count, typically 2–5, and comparable to accelerator scope, which is also unlimited. No node-count limit is added.
- [cc-lib v2.15 is not tagged yet] → Develop against `../cc-lib` with an uncommitted `go.work`. The go.mod bump to v2.15.0 is the last task, done once the tag exists.

## Migration Plan

1. Tag cc-lib v2.15.0 (maintainer).
2. Merge this change with `go.mod` pointing at v2.15.0.
3. Sites add `topology.filesystems` / `topology.networks` and device metrics (`scope: filesystem|network`, `aggregation: sum|avg`) to cluster.json, and normalize collector tags. Without that, nothing changes, apart from the strict device-scope rule.

Rollback: revert the merge. Archives written in the meantime hold device scopes as ordinary scope entries, which the previous cc-backend and cc-lib v2.14 decode as unknown scopes of a flat metric, without error.
