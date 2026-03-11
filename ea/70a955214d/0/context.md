# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Fix Missing `rows.Close()` Memory Leaks in SQLite3 Queries

## Context

Production memory leaks traced to queries that do full table scans (e.g., job state list sorted by `start_time` on all jobs). The root cause is `sql.Rows` objects not being closed after query execution. In Go's `database/sql`, every `rows` returned by `.Query()` holds a database connection and associated buffers until `rows.Close()` is called. Without `defer rows.Close()`, these leak on ev...

### Prompt 2

Check if the fixes are correctly merged in nodes.go

### Prompt 3

There also have to be bugs in jobQuery.go . Especially the following query triggers the memory leak: SELECT * FROM job WHERE job.job_state IN ("completed", "running", "failed") ORDER BY job.start_time DESC LIMIT 1 OFFSET 10; Dig deeper to find the cause. Also investigate why no existing index is used for this query.

