# Tasks

## 1. Target ids in the shared builder (D1, D4, D5)

- [x] 1.1 Add `ID *string` to `ScopeQueryResult` and the shared `QueryTarget{Scope, ID}` type in `pkg/metricstore/scopequery.go`; verify `go build ./...` succeeds
- [x] 1.2 Set `ID` in every aggregated branch of `BuildScopeQueries` that targets a core or socket: hwthread→core (core id), hwthread→socket, core→socket and memoryDomain→socket (socket id). Leave it nil for node-scope and unaggregated results. Verify `go build ./...` succeeds
- [x] 1.3 Rewrite the core→socket branch on core ids: the job's cores from `GetCoresFromHWThreads(hwthreads)`, their sockets from `GetSocketsFromCores(cores)`, and per socket `TypeIds` = the socket's cores from `GetCoresFromHWThreads(topology.Socket[socket])`. Verify `go build ./...` succeeds
- [x] 1.4 Extend `TestBuildScopeQueries` with the expected id of every case, comparing aggregated results as a set of `{ID, TypeIds}`:
  - hwthread→core gives ids `0`–`3`, each combining that core's hardware threads
  - hwthread→socket gives `0`, `1`; memoryDomain→socket gives socket ids
  - core→socket on the SMT fixture gives `{0: [0 1]}` and `{1: [2 3]}`
  - every →node case and every unaggregated case has a nil id

  Verify `go test ./pkg/metricstore/ -run BuildScopeQueries` passes.
- [x] 1.5 Add a core→socket case for a job on socket 0 only (hardware threads `0`–`3`), expecting one result with id `0` and `TypeIds` `[0 1]`; verify it passes, and that it fails against the old branch (temporarily revert 1.3)

## 2. Carry targets through the builders (D2)

- [x] 2.1 Change `buildQueries` and `buildNodeQueries` in `pkg/metricstore/query.go` to return `[]QueryTarget` built from each result's `Scope` and `ID`, and update `LoadData`, `LoadStats`, `LoadScopedStats` and `LoadNodeListData`, including the length checks and truncation; verify `go build ./...` succeeds
- [x] 2.2 Apply the same change to the two builders and their callers in `internal/metricstoreclient/`; verify `go build ./...` and `go vet ./...` succeed
- [x] 2.3 Update the builder-loop tests of both stores (`pkg/metricstore/query_test.go`, `internal/metricstoreclient/cc-metric-store-queries_test.go`) to the new return type, and assert that the node-scope device queries carry no target id and hwthread queries carry none; verify `go test ./pkg/metricstore/... ./internal/metricstoreclient/...` passes

## 3. Result loops (D3)

- [x] 3.1 In the three result loops of `pkg/metricstore/query.go` (`LoadData`, `LoadScopedStats`, `LoadNodeListData`), take the id from `target.ID` when `query.Aggregate` and from `ExtractTypeID` otherwise; verify `go build ./...` succeeds
- [x] 3.2 Apply the same rule to the three result loops in `internal/metricstoreclient/cc-metric-store.go`; verify `go build ./...` succeeds
- [x] 3.3 Add an internal-store `LoadData` test on the device archive fixture: swap `msInstance` for a store fed with `type=filesystem,type-id=` lines for `/home` and `/scratch` on `a01` (restore with `t.Cleanup`), load `fs_read_bw` at `[node, filesystem]`, and assert the node series has a nil id while the filesystem series carry `/home` and `/scratch`; verify `go test ./pkg/metricstore/ -run LoadData` passes and fails with 3.1 reverted

## 4. Live check and docs

- [x] 4.1 Make the `run-cc-backend` smoke test (`.claude/skills/run-cc-backend/driver.sh`) assert that the node series carries no id; verify `driver.sh smoke` prints `SMOKE OK` and shows `id=None` for the node series
- [x] 4.2 Add one line to the "Scopes" section of `CLAUDE.md`: aggregated series carry the id of the scope they represent (none at node scope); verify the line is in place

## 5. Verification

- [x] 5.1 Run `go vet ./...` and `go test ./...` and verify both pass; restore `internal/repository/testdata/job.db` if the run modified it
