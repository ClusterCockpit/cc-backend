# Tasks

## 1. Build against cc-lib v2.15 (pre-release)

- [x] 1.1 Create a `go.work` outside the repo (it is not git-ignored) that uses `.` and `../cc-lib` on branch `feat/device-metric-scopes`, and export `GOWORK` for the session; verify `go build ./...` fails only in `pkg/archive/json.go`, `pkg/archive/parquet/convert.go` and `internal/metricdispatch/dataLoader.go`
  - Superseded: cc-lib v2.15.0 was already tagged (merge of `feat/device-metric-scopes`, identical tree), so the build uses the tagged module directly and no `go.work` was created. Build failed only in `pkg/archive` and `pkg/archive/parquet`; `metricdispatch` was blocked behind them.

## 2. Remove metric-group plumbing (D5)

- [x] 2.1 Drop the group loop in `archive.DecodeJobStats` and the `Statistics.Groups` check in `parquet.JobToParquetRow`; verify `go build ./pkg/...` succeeds
- [x] 2.2 Reduce `deepCopy` in `internal/metricdispatch/dataLoader.go` to copying `Metrics`, and stop carrying `Groups` in the metric/scope filter; verify `go build ./...` and `go vet ./...` succeed
- [x] 2.3 Add an archive test that job data with a top-level `"filesystems": [...]` fails to decode, and one that `fs_read_bw` with scopes `node` and `filesystem` round-trips through `DecodeJobData`/`DecodeJobStats` and the Parquet row conversion; verify `go test ./pkg/archive/...` passes

## 3. Device branch in the shared scope builder (D1, D2)

- [x] 3.1 Add pre-converted `FilesystemString`/`NetworkString` next to `AcceleratorString`, plus the `DeviceIDs(native, topology, allocatedAccelerators)` helper, in `pkg/metricstore/scopequery.go`; verify with a unit test that accelerators return the allocated ids and filesystems/networks return the topology ids in declaration order
- [x] 3.2 Replace the two accelerator branches of `BuildScopeQueries` with the single `IsDevice()` branch, add the early return for device scopes requested on non-device metrics, and rename the `accelerators` parameter to `deviceIDs`; verify `go build ./...` succeeds
- [x] 3.3 Extend the `TestBuildScopeQueries` topology fixture with filesystems and networks, and add table cases for every D1 row, for all three device scopes; verify `go test ./pkg/metricstore/ -run BuildScopeQueries` passes. The cases:
  - native → native
  - native → node (aggregated)
  - native → each CPU scope (empty)
  - native → a different device scope (empty)
  - no ids (empty)
  - `hwthread`/`core`/`node` native → each device scope (empty, `ok=true`)
- [x] 3.4 Extend `TestBuildScopeQueries_UnhandledCase` to iterate over all native × requested scopes, including the device ones, and assert `ok=true` with a non-nil result; verify the test passes

## 4. Query builder loops (D3)

- [x] 4.1 In `pkg/metricstore/query.go` `buildQueries` and `buildNodeQueries`:
  - skip a requested device scope that differs from the native scope before `handledScopes` de-duplication
  - replace the `NumAcc`/empty-accelerator pre-skips with `native.IsDevice() && len(DeviceIDs(...)) == 0`
  - pass `DeviceIDs(...)` to the builder

  Verify `go build ./...` succeeds.
- [x] 4.2 Apply the same three edits to `buildQueries` and `buildNodeQueries` in `internal/metricstoreclient/cc-metric-store-queries.go`; verify `go build ./...` succeeds
- [x] 4.3 Add a test helper that initializes a temporary file archive (`version.txt` plus a `cluster.json` with subclusters `a` declaring `/home` and `/scratch`, `b` declaring `/work`, and `c` declaring none). Verify `archive.GetSubCluster` returns the declared filesystems in a smoke test.
- [x] 4.4 Add internal-store builder tests on that archive; verify `go test ./pkg/metricstore/...` passes:
  - a job requesting `[filesystem, hwthread]` for an `hwthread` metric still gets its hardware-thread queries
  - a job on `a` requesting `fs_read_bw` at `filesystem` gets `Type=filesystem`, `TypeIds=[/home, /scratch]`
  - a job on `c` gets no queries and no error
  - a node query without a given subcluster, over nodes of `a` and `b`, gets each node's own ids
- [x] 4.5 Add the same four builder tests for the external client (its `getTopology`/`getTopologyByNode` read the same archive); verify `go test ./internal/metricstoreclient/...` passes
- [x] 4.6 Add a line-protocol test pinning the collector convention: `type=filesystem,type-id=<id>` lines are read per instance and summed at node scope by the queries `BuildScopeQueries` builds, while `type=node,stype=filesystem,stype-id=<id>` creates no device level; verify `go test ./pkg/metricstore/...` passes

## 5. Archiver (D4)

- [x] 5.1 Extract the scope selection of `ArchiveJob` into `archiveScopes(job, subCluster)`, keeping `node`, the ≤8-node `core` rule and `accelerator` for `NumAcc > 0`, and adding `filesystem`/`network` when the subcluster declares them. If the subcluster lookup fails, log it and archive without device scopes. Verify `go build ./...` succeeds.
- [x] 5.2 Add table tests for `archiveScopes`; verify `go test ./internal/archiver/...` passes:
  - a 64-node job on a subcluster with filesystems gets `[node, filesystem]`
  - a 4-node GPU job gets `[node, core, accelerator]`
  - a 4-node job on a subcluster with filesystems and networks gets `[node, core, filesystem, network]`
  - a job on a subcluster without devices gets `[node, core]`

## 6. Documentation and generated code

- [x] 6.1 Run `make swagger` and verify `internal/api/docs.go` and `api/swagger.yaml` no longer mention `StatsGroup`/`StatsGroupInstance` and that `MetricScope` lists `filesystem` and `network`
- [x] 6.2 Update `CLAUDE.md` and verify the text is in place:
  - "Scopes": add the device scopes (accelerator, filesystem, network), no conversion to CPU scopes, the strict rule for device requests on CPU metrics
  - Configuration: `topology.filesystems`/`topology.networks` with `{id, type}`, and the requirement `aggregation: sum|avg` for device metrics
  - Collector prerequisite: `type=filesystem|network`, `type-id=<id>`

## 7. Verification and release bump

- [x] 7.1 Run `go vet ./...` and `go test ./...` with `GOWORK` set and verify both pass
- [x] 7.2 Manually verify against a running internal metric store: ingest `fs_read_bw,hostname=h1,cluster=c,type=filesystem,type-id=/scratch value=1` and the same for `/home`. Query a job on `h1` via GraphQL `jobMetrics(scopes: [node, filesystem])`, and verify the two per-mount series (ids `/home`, `/scratch`) and a node series equal to their sum.
  - Done 2026-10-01 on an isolated server (REST `/api/write`, `start_job`, GraphQL `/query`): over 14 one-minute samples, `/home` read 1, `/scratch` read 2 and the node series 3, with all points present.
- [x] 7.3 After cc-lib v2.15.0 is tagged, bump `github.com/ClusterCockpit/cc-lib/v2` to v2.15.0 in `go.mod` and run `go mod tidy`. Unset `GOWORK` and delete the external `go.work` (none was created, see 1.1). Verify `go build ./...` and `go test ./...` pass without the workspace.
