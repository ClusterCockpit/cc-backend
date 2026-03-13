-- Migration 13: Add covering indexes for status/dashboard queries
-- Column order: cluster (equality), job_state (equality), grouping col, then aggregated columns
-- These indexes allow the status views to be served entirely from index scans.

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user_stats
  ON job (cluster, job_state, hpc_user, duration, start_time, num_nodes, num_hwthreads, num_acc);

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project_stats
  ON job (cluster, job_state, project, duration, start_time, num_nodes, num_hwthreads, num_acc);

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_subcluster_stats
  ON job (cluster, job_state, subcluster, duration, start_time, num_nodes, num_hwthreads, num_acc);

-- Drop 3-col indexes that are now subsumed by the covering indexes above
DROP INDEX IF EXISTS jobs_cluster_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_jobstate_project;

PRAGMA optimize;
