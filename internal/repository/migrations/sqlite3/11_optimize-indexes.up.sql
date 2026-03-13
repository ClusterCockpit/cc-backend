-- Migration 11: Optimize job table indexes
--   - Remove overly specific sorting indexes (reduces ~78 → 48)
--   - Add covering index for grouped stats queries
--   - Add covering indexes for status/dashboard queries
--   - Add partial covering indexes for running jobs (tiny B-tree)

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

-- ============================================================
-- Covering indexes for grouped stats queries
-- Column order: cluster (equality), hpc_user/project (GROUP BY), start_time (range scan)
-- Includes aggregated columns to avoid main table lookups entirely.
-- ============================================================

CREATE INDEX IF NOT EXISTS jobs_cluster_user_starttime_stats
  ON job (cluster, hpc_user, start_time, duration, job_state, num_nodes, num_hwthreads, num_acc);

CREATE INDEX IF NOT EXISTS jobs_cluster_project_starttime_stats
  ON job (cluster, project, start_time, duration, job_state, num_nodes, num_hwthreads, num_acc);

-- ============================================================
-- Covering indexes for status/dashboard queries
-- Column order: cluster (equality), job_state (equality), grouping col, then aggregated columns
-- These indexes allow the status views to be served entirely from index scans.
-- ============================================================

-- Drop 3-col indexes that are subsumed by the covering indexes below
DROP INDEX IF EXISTS jobs_cluster_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_jobstate_project;

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user_stats
  ON job (cluster, job_state, hpc_user, duration, start_time, num_nodes, num_hwthreads, num_acc);

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project_stats
  ON job (cluster, job_state, project, duration, start_time, num_nodes, num_hwthreads, num_acc);

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_subcluster_stats
  ON job (cluster, job_state, subcluster, duration, start_time, num_nodes, num_hwthreads, num_acc);

-- ============================================================
-- Partial covering indexes for running jobs
-- Only running jobs are in the B-tree, so these indexes are tiny compared to
-- the full-table indexes above. SQLite uses them when the query contains the
-- literal `job_state = 'running'` (not a parameter placeholder).
-- ============================================================

CREATE INDEX IF NOT EXISTS jobs_running_user_stats
  ON job (cluster, hpc_user, num_nodes, num_hwthreads, num_acc, duration, start_time)
  WHERE job_state = 'running';

CREATE INDEX IF NOT EXISTS jobs_running_project_stats
  ON job (cluster, project, num_nodes, num_hwthreads, num_acc, duration, start_time)
  WHERE job_state = 'running';

CREATE INDEX IF NOT EXISTS jobs_running_subcluster_stats
  ON job (cluster, subcluster, num_nodes, num_hwthreads, num_acc, duration, start_time)
  WHERE job_state = 'running';

PRAGMA optimize;
