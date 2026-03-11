-- Migration 11: Remove overly specific table indexes formerly used in sorting
-- When one or two indexed columns are used, sorting usually is fast
-- Reduces from ~78 indexes to 48 for better write performance,
-- reduced disk usage, and more reliable query planner decisions.
-- Requires ANALYZE to be run after migration (done automatically on startup).

-- ============================================================
-- Drop SELECTED existing job indexes (from migrations 08/09)
-- sqlite_autoindex_job_1 (UNIQUE constraint) is kept automatically
-- ============================================================

-- Cluster+Partition Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_partition_numnodes;
DROP INDEX IF EXISTS jobs_cluster_partition_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_partition_numacc;
DROP INDEX IF EXISTS jobs_cluster_partition_energy;

-- Cluster+JobState Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numacc;
DROP INDEX IF EXISTS jobs_cluster_jobstate_energy;

-- Cluster+Shared Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_shared_numnodes;
DROP INDEX IF EXISTS jobs_cluster_shared_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_shared_numacc;
DROP INDEX IF EXISTS jobs_cluster_shared_energy;

-- User Filter Sorting
DROP INDEX IF EXISTS jobs_user_numnodes;
DROP INDEX IF EXISTS jobs_user_numhwthreads;
DROP INDEX IF EXISTS jobs_user_numacc;
DROP INDEX IF EXISTS jobs_user_energy;

-- Project Filter Sorting
DROP INDEX IF EXISTS jobs_project_numnodes;
DROP INDEX IF EXISTS jobs_project_numhwthreads;
DROP INDEX IF EXISTS jobs_project_numacc;
DROP INDEX IF EXISTS jobs_project_energy;

-- JobState Filter Sorting
DROP INDEX IF EXISTS jobs_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_jobstate_numacc;
DROP INDEX IF EXISTS jobs_jobstate_energy;

-- Shared Filter Sorting
DROP INDEX IF EXISTS jobs_shared_numnodes;
DROP INDEX IF EXISTS jobs_shared_numhwthreads;
DROP INDEX IF EXISTS jobs_shared_numacc;
DROP INDEX IF EXISTS jobs_shared_energy;

-- ArrayJob Filter
DROP INDEX IF EXISTS jobs_cluster_arrayjobid_starttime;

-- Backup Indices For High Variety Columns
DROP INDEX IF EXISTS jobs_duration;

-- Optimize DB index usage
PRAGMA optimize;
