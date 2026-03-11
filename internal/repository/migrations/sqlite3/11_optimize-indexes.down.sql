-- Migration 11 DOWN: Restore indexes from migration 09

-- ============================================================
-- Recreate all removed indexes from migration 09
-- ============================================================

-- Cluster+Partition Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numnodes ON job (cluster, cluster_partition, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numhwthreads ON job (cluster, cluster_partition, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numacc ON job (cluster, cluster_partition, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_energy ON job (cluster, cluster_partition, energy);

-- Cluster+JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numnodes ON job (cluster, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numhwthreads ON job (cluster, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numacc ON job (cluster, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_energy ON job (cluster, job_state, energy);

-- Cluster+Shared Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_numnodes ON job (cluster, shared, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_numhwthreads ON job (cluster, shared, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_numacc ON job (cluster, shared, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_energy ON job (cluster, shared, energy);

-- User Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_user_numnodes ON job (hpc_user, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_user_numhwthreads ON job (hpc_user, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_user_numacc ON job (hpc_user, num_acc);
CREATE INDEX IF NOT EXISTS jobs_user_energy ON job (hpc_user, energy);

-- Project Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_project_numnodes ON job (project, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_project_numhwthreads ON job (project, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_project_numacc ON job (project, num_acc);
CREATE INDEX IF NOT EXISTS jobs_project_energy ON job (project, energy);

-- JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_jobstate_numnodes ON job (job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numhwthreads ON job (job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numacc ON job (job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_jobstate_energy ON job (job_state, energy);

-- Shared Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_shared_numnodes ON job (shared, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_shared_numhwthreads ON job (shared, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_shared_numacc ON job (shared, num_acc);
CREATE INDEX IF NOT EXISTS jobs_shared_energy ON job (shared, energy);

-- ArrayJob Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_arrayjobid_starttime ON job (cluster, array_job_id, start_time);

-- Backup Indices For High Variety Columns
CREATE INDEX IF NOT EXISTS jobs_duration ON job (duration);

-- Optimize DB index usage
PRAGMA optimize;
