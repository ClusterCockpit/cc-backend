# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Plan: Improve scanJob logging

## Context

`scanJob` in `internal/repository/job.go` (line 162) logs a `Warn` for every scan error, including the very common `sql.ErrNoRows` case. This produces noisy, unhelpful log lines like:

```
WARN Error while scanning rows (Job): sql: no rows in result set
```

Two problems:
1. `sql.ErrNoRows` is a normal, expected condition (callers are documented to check for it). It should not produce a warning.
2. When a real scan er...

