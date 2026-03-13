# Session Context

Session ID: 42401d2e-7d1c-4c0e-abe6-356cb2d48747
Commit Message: There also have to be bugs in jobQuery.go . Especially the following que

## Prompts

### Prompt 1

There also have to be bugs in jobQuery.go . Especially the following query triggers the memory leak: SELECT * FROM job WHERE job.job_state IN ("completed", "running", "failed") ORDER BY job.start_time DESC LIMIT 1 OFFSET 10; Dig deeper to find the cause. Also investigate why no existing index is used for this query.

## Summary

All passes. Here's a summary of findings and fixes:

---

## Findings

### Bug 1: Missing `defer rows.Close()` in `user.go:GetLdapUsernames()` (line 100)
**Fixed.** Every call leaked a database connection and its buffers.

### Bug 2: SQLite never runs `ANALYZE` — wrong index selection
**Fixed.** Added `ANALYZE` to `setupSqlite()` in `dbConnection.go`.

Without statistics in `sqlite_stat1`, SQLite's query planner **guessed** and chose `jobs_jobstate_energy` (a terrible index for this query) instead of scanning `jobs_starttime` in reverse. The result:

| | Before ANALYZE | After ANALYZE |
|---|---|---|
| Index used | `jobs_jobstate_energy` | `jobs_starttime` |
| Sort | **TEMP B-TREE** (materializes ALL rows) | None (index order) |
| Memory | Proportional to total matching rows | Constant |
| I/O | Full scan of all matching rows | Stops at OFFSET+LIMIT |

### Bug 3: `IN` clause + `ORDER BY` is fundamentally incompatible with composite indexes

Even with the "correct" index `(job_state, start_time)`, SQLite **cannot** merge-sort across 3 separate index range scans for `IN ('completed','running','failed')`. It always falls back to a temp B-tree sort. The only efficient plan is to use the standalone `jobs_starttime` index — which SQLite does automatically **after ANALYZE** because it realizes the 3 states cover virtually all rows, making the WHERE clause nearly a no-op.

### Observation: 79 indexes on the `job` table
This is excessive and actively harmful — it confuses the query planner (especially without ANALYZE) and slows writes. The `jobs_jobstate_starttime` index from migration 08 is also missing from the actual DB (only the 3-column `jobs_jobstate_starttime_duration` exists). This is worth investigating separately but is a schema/migration concern, not a code bug.
