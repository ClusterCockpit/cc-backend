# Proposal

## Why

When several sources are combined into one series, that series is labelled with the id of its first source instead of the id of the scope it represents. A filesystem metric at node scope comes back with `id: "/home"`, a GPU metric at node scope with the first GPU's id, and a hardware-thread metric at core scope with the first hardware thread of each core (`0, 2, 4, 6` instead of cores `0–3`). The job statistics table shows these ids (`{s.id ?? i}`) and sorts by them, and archiving persists them in every job's data.

The core-native branch of the same builder also mixes up id kinds. When a core metric is requested at socket scope, it passes hardware-thread ids where core ids are expected, twice. On nodes with SMT it therefore picks the wrong sockets and aggregates the wrong or non-existent cores.

## What Changes

- An aggregated series carries the id of the scope it represents: no id at node scope, the core id at core scope, the socket id at socket scope. This holds for every native scope, device metrics included.
- Unaggregated series keep the id of their source (hardware thread, core, memory domain, socket, device), as today.
- A core metric requested at socket scope aggregates exactly the cores of each socket the job's cores lie on.
- Both the internal metric store and the external cc-metric-store client label series this way, in job data, scoped job statistics and node-list data.
- **BREAKING (series ids)**: GraphQL, REST and newly archived job data report different `Series.ID` / `ScopedStats.ID` values for aggregated series. Jobs already archived keep their old ids; there is no migration. The frontend uses series ids only for accelerator-scope series, which are unaggregated and unchanged.

Out of scope:
- re-labelling ids in already archived jobs
- the external cc-metric-store server itself (it receives the same queries as before)

## Capabilities

### New Capabilities
- `scope-aggregation`: how series of a metric are combined from its native scope into a coarser requested scope, which sources each combined series covers, and which id it carries.

### Modified Capabilities
<!-- none: device-metric-scopes requirements stay valid; its node-scope series gain no id through the new general rule -->

## Impact

- **Code**:
  - `pkg/metricstore/scopequery.go`: aggregated results carry their target id; the core→socket branch uses core ids
  - `pkg/metricstore/query.go`: `buildQueries`, `buildNodeQueries`, and the result loops of `LoadData`, `LoadScopedStats`, `LoadNodeListData`
  - `internal/metricstoreclient/cc-metric-store-queries.go` and `cc-metric-store.go`: the same builders and result loops
  - the tests next to each, and the `run-cc-backend` smoke test (node series without id)
- **APIs**: GraphQL `Series.id` and `ScopedStats.id`, REST job data. No schema change.
- **Data**: newly archived jobs carry the corrected ids; existing archives are unchanged.
- **Dependencies**: none (cc-lib topology helpers suffice).
