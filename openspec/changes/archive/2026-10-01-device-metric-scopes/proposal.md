# Proposal

## Why

Filesystem and network metrics can only be shown per node today. Nothing can tell `/home` from `/scratch`, or `ib0` from `eth0`. The metric store already keeps per-instance data under the host when it arrives as `type=filesystem,type-id=/scratch` (the accelerator convention), but no query can reach it. cc-lib v2.15 adds `filesystem` and `network` as device scopes, declares the instances in the subcluster topology, and removes the unused metric-group ("filesystems" array) API. cc-backend has to adopt it, or it stops compiling against v2.15.

## What Changes

- Bump `github.com/ClusterCockpit/cc-lib/v2` to v2.15.0.
- Support the `filesystem` and `network` scopes as native metric scopes, handled like `accelerator`:
  - one query per declared instance when the device scope is requested
  - a single aggregated query per host when node scope is requested
  - no data for any other requested scope
- Take the instance ids from `topology.filesystems` / `topology.networks` of the job's subcluster.
  - A subcluster that declares none gets no data for the scope, and no error.
  - Series carry the instance id in `Series.ID`, the same way accelerator series carry the device id.
- Handle all three device scopes (accelerator, filesystem, network) in one branch of the shared scope-query builder, used by both the internal metric store and the external cc-metric-store client. It replaces the accelerator-only branches.
- **BREAKING (archive contents)**: requesting a device scope for a metric whose native scope is not that device scope now returns no data.
  - Before, `Max()` silently changed the request to the metric's native CPU scope. As a result, jobs with more than 8 nodes that use GPUs had every hardware-thread metric archived at hardware-thread resolution.
  - Newly archived GPU jobs with more than 8 nodes are therefore smaller. Already archived jobs are unchanged.
  - Two views also got hardware-thread series this way and plotted them as the finest scope returned: the node list (requests `core, socket, accelerator`) and single-node GPU job rows (request `core, accelerator`). They now show CPU metrics at core scope. If hardware-thread plots are wanted there, the separate UI change can request `hwthread` explicitly.
- The archiver also loads the `filesystem` and `network` scopes when the job's subcluster declares them. `meta.json` statistics and footprints keep coming from the node total.
- **BREAKING (Go API, internal)**: remove the metric-group plumbing that v2.15 no longer provides, in the archive decoders, the Parquet converter and the job-data deep copy. Archives containing a top-level `filesystems` array no longer decode. None are known to exist: nothing in cc-backend ever wrote one.
- Regenerate the Swagger docs, which still list the removed `StatsGroup` types. Document device scopes and the topology lists in `CLAUDE.md`.

Out of scope:
- the frontend UI (a separate change)
- collector tag normalization (cc-metric-collector)
- per-device footprints, sorting and filters

## Capabilities

### New Capabilities
- `device-metric-scopes`: querying, aggregating and archiving metrics whose native scope is a node-attached device (accelerator, filesystem, network), with instances declared in the subcluster topology.

### Modified Capabilities
<!-- none: openspec/specs/ has no capabilities yet -->

## Impact

- **Code**:
  - `pkg/metricstore/scopequery.go` (device branch, signature)
  - `pkg/metricstore/query.go` and `internal/metricstoreclient/cc-metric-store-queries.go` (callers: job, node and node-list query builders)
  - `internal/archiver/archiver.go` (scopes)
  - `pkg/archive/json.go`, `pkg/archive/parquet/convert.go`, `internal/metricdispatch/dataLoader.go` (Groups removal)
  - the tests next to each
- **APIs**:
  - GraphQL `MetricScope` is a scalar bound to cc-lib, so `filesystem` and `network` become valid values without a schema change.
  - REST job data can contain the new scopes.
  - Swagger is regenerated.
- **Configuration**: cluster.json may declare `topology.filesystems` / `topology.networks`. Device metrics need `aggregation` set to `sum` or `avg`, or node-scope aggregation fails in the metric store.
- **Dependencies**: requires the cc-lib v2.15.0 tag (branch `feat/device-metric-scopes`).
- **Data**: GPU jobs with more than 8 nodes archive less (see BREAKING above). No migration.
