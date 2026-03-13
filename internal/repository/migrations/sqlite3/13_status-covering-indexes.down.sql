-- Reverse migration 13: Remove covering status indexes, restore 3-col indexes

DROP INDEX IF EXISTS jobs_cluster_jobstate_user_stats;
DROP INDEX IF EXISTS jobs_cluster_jobstate_project_stats;
DROP INDEX IF EXISTS jobs_cluster_jobstate_subcluster_stats;

-- Restore the original 3-col indexes
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user
  ON job (cluster, job_state, hpc_user);

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project
  ON job (cluster, job_state, project);
