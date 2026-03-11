# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Consolidate buildQueries / Scope Transformation Logic

## Context

`pkg/metricstore/query.go` (InternalMetricStore) and `internal/metricstoreclient/cc-metric-store-queries.go` (CCMetricStore) both implement the `MetricDataRepository` interface. They contain nearly identical scope-transformation logic for building metric queries, but the code has diverged:

- **metricstoreclient** has the cleaner design: a standalone `buildScopeQueries()` function handling all ...

