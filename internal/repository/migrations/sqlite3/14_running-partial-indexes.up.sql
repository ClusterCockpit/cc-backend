-- Migration 14: Partial covering indexes for running jobs
-- Only running jobs are in the B-tree, so these indexes are tiny compared to
-- the full-table indexes from migration 13. SQLite uses them when the query
-- contains the literal `job_state = 'running'` (not a parameter placeholder).

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
