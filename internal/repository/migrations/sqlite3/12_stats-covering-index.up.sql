-- Migration 12: Add covering index for grouped stats queries
-- Column order: cluster (equality), hpc_user (GROUP BY), start_time (range scan)
-- Includes aggregated columns to avoid main table lookups entirely.

DROP INDEX IF EXISTS jobs_cluster_starttime_user_stats;

CREATE INDEX IF NOT EXISTS jobs_cluster_user_starttime_stats
  ON job (cluster, hpc_user, start_time, duration, job_state, num_nodes, num_hwthreads, num_acc);

PRAGMA optimize;
