-- Reverse migration 14: Drop partial indexes for running jobs

DROP INDEX IF EXISTS jobs_running_user_stats;
DROP INDEX IF EXISTS jobs_running_project_stats;
DROP INDEX IF EXISTS jobs_running_subcluster_stats;
