-- Migration 12: Add covering index for grouped stats queries
-- Column order: cluster (equality), hpc_user (GROUP BY), start_time (range scan)
-- Includes aggregated columns to avoid main table lookups entirely.

CREATE INDEX IF NOT EXISTS jobs_cluster_user_starttime_stats
  ON job (cluster, hpc_user, start_time, duration, job_state, num_nodes, num_hwthreads, num_acc);

CREATE INDEX IF NOT EXISTS jobs_cluster_project_starttime_stats
  ON job (cluster, project, start_time, duration, job_state, num_nodes, num_hwthreads, num_acc);

PRAGMA optimize;
