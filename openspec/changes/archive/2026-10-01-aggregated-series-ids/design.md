# Design

## Context

See proposal.md for the motivation and specs/scope-aggregation/spec.md for the required behavior.

Current state:

- **Where the id comes from.** Each of the six result loops sets `Series.ID` with `ExtractTypeID(query.Type, query.TypeIds, ndx, …)`, which returns `TypeIds[ndx]`. The loops are in `LoadData`, `LoadScopedStats` and `LoadNodeListData`, in both `pkg/metricstore/query.go` and `internal/metricstoreclient/cc-metric-store.go`. An aggregated query returns exactly one result per query, so `ndx` is 0, and the id becomes the first source id.
- **Who knows the right id.** `BuildScopeQueries` in `pkg/metricstore/scopequery.go` builds one aggregated result per target instance, and its loop variable is the target: `core` in hwthread→core, `socket` in hwthread→socket, core→socket and memoryDomain→socket. A result for node scope has no target id. The id is dropped when the result is converted to an `APIQuery`.
- **How results are mapped back.** The four builders (`buildQueries`, `buildNodeQueries` in each store) return `queries` plus a parallel slice `assignedScope []schema.MetricScope`. The loops index it with the query index, and `LoadData` and `LoadNodeListData` truncate it together with the queries when the response is short.
- **The core→socket branch** calls `GetSocketsFromCores(hwthreads)`, which treats hardware-thread ids as core ids. It then queries `TypeIds: topology.Socket[socket]`, which are hardware-thread ids, with `Type: core`. The cc-lib helpers it needs already exist: `GetCoresFromHWThreads` maps hardware threads to cores, and the topology maps are initialized when the archive loads the cluster config (`pkg/archive/clusterConfig.go`).
- **Consumers of the id.** Nothing in the backend reads `Series.ID`. The frontend uses it for the accelerator legend (unaggregated series), and the job statistics table shows and sorts by it (`StatsTableEntry.svelte`, `StatsTable.svelte`).

## Goals / Non-Goals

**Goals:**
- The id of a series is decided once, in the shared builder, for both stores and all three result loops of each.
- No per-series allocation added to the result loops.

**Non-Goals:**
- Changing which sources an aggregate covers, except for core→socket. For example, hwthread→core keeps aggregating all hardware threads of a core the job touches.
- The order of series within a scope. It already follows map iteration in some branches.
- Rewriting ids in existing archives.

## Decisions

### D1: The builder returns the target id with each aggregated result

`ScopeQueryResult` gains `ID *string`. `BuildScopeQueries` sets it for every aggregated result that represents a scope instance: the core id for core scope, and the socket id for socket scope, including memoryDomain→socket, where the map key is the socket. Node-scope results, and every unaggregated result, leave it nil. For unaggregated results the loops keep deriving the id per series from `TypeIds`, which is already correct.

**Alternative considered:** derive the id in the result loops from `scope` and `TypeIds` (for example, look up the core of the first hardware thread). Rejected: it repeats topology logic in six places for both stores, and the builder already has the value.

### D2: Replace `assignedScope` with a slice of query targets

A shared type in `scopequery.go`, `QueryTarget{Scope schema.MetricScope; ID *string}`, replaces `[]schema.MetricScope` as the builders' second return value. The loops read `target.Scope` where they read `scope` today, and the existing truncation keeps applying to the one slice.

**Alternatives considered:**
- A third parallel slice for ids. Rejected: one more slice that has to stay in step through truncation.
- A field on `APIQuery` tagged `json:"-"`. Rejected: `APIQuery` is the wire type of both stores' query APIs. The internal one is also the request type of the store's REST query endpoint, so the field would carry response labelling into a request type.

### D3: Loops take the target id for aggregated queries

In each loop, the id is `target.ID` when `query.Aggregate`, and `ExtractTypeID(…)` otherwise. `Aggregate` is the right switch, because it is what makes the store return one combined result instead of one per `TypeIds` entry. Node-native metrics at node scope have `Aggregate=false` and `Type=nil`, so `ExtractTypeID` already returns nil for them.

### D4: Core→socket from core ids

The branch computes the job's cores with `GetCoresFromHWThreads(hwthreads)`, and their sockets with `GetSocketsFromCores(cores)`. For each socket, it queries the cores of that socket, `GetCoresFromHWThreads(topology.Socket[socket])`, as `Type: core`, and sets the socket id. Querying all cores of the socket matches hwthread→socket, which queries all hardware threads of the socket (`topology.Socket[socket]`).

**Alternative considered:** query only the job's cores on that socket. Rejected: it would make core→socket differ from hwthread→socket for the same job, which is a behavior choice beyond fixing the id mix-up.

### D5: Pre-converted ids, allocated once per result

The ids are formatted with `strconv.Itoa` once per aggregated result in the builder. That is one allocation per query, next to the `TypeIds` slice the builder already allocates per query. The result loops add none.

### D6: Testing

- **`TestBuildScopeQueries`:** add the expected id to every case. Aggregated cases are compared as a set of `{ID, TypeIds}`, because some branches iterate maps.
  - hwthread→core gives ids `0`–`3`, and hwthread→socket gives `0`, `1`.
  - memoryDomain→socket gives socket ids.
  - core→socket on the SMT fixture gives `{0: [0 1]}` and `{1: [2 3]}`.
  - Every →node case and every unaggregated case has a nil id.
- **Core→socket for a job on socket 0 only:** one result with id `0` and cores `[0 1]`.
- **Builder-loop tests, both stores:** the returned targets carry the ids, and node-scope device queries carry none.
- **Result-loop test (internal store):** extend the line-protocol test to run the real `LoadData` path, or `LoadNodeListData` if a job fixture is simpler. Check that the node series has no id and the filesystem series have their mount points.
- **`run-cc-backend` smoke:** assert that the node series has no id.

## Risks / Trade-offs

- [External consumers that relied on the old ids, for example scripts reading `Series.id` from GraphQL] → Called out as **BREAKING (series ids)** in the proposal and the commit. The old ids were not meaningful (first source), so no correct consumer can depend on them.
- [Archives now mix old-rule and new-rule ids across jobs] → Accepted (no migration). The UI shows the id only in the statistics table, where both forms render; new jobs show the correct values.
- [Core→socket now returns different data on SMT nodes] → It was wrong before: it aggregated the cores whose ids equal the socket's hardware-thread ids. The change is a fix; no cluster config in `configs/` or the test archives under `pkg/archive/testdata` declares a `core`-native metric, so the effect is limited to sites that do.
- [The external cc-metric-store receives different queries for core→socket] → The queries stay well-formed (`type: core`, core ids), and the store treats `type` as an opaque prefix. No server change is needed.
