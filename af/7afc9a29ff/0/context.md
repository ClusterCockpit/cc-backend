# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Optimize Job Table Indexes for 20M Row Production Database

## Context

The `job` table has **79 indexes** (created in migrations 08/09), causing:
1. **Wrong index selection** — without `ANALYZE` statistics, SQLite picks wrong indexes (e.g., `jobs_jobstate_energy` instead of `jobs_starttime` for ORDER BY queries), causing full-table temp B-tree sorts on 20M rows → timeouts
2. **Excessive disk/memory overhead** — each index costs ~200-400MB at 20M rows; 79 inde...

