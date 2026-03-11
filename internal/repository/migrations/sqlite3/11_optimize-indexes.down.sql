-- Migration 11 DOWN: Restore all indexes from migration 09
-- Reverts the index optimization by dropping the 20 optimized indexes
-- and recreating the original full set.

-- ============================================================
-- Drop optimized indexes
-- ============================================================

DROP INDEX IF EXISTS jobs_starttime;
DROP INDEX IF EXISTS jobs_cluster_starttime_duration;
DROP INDEX IF EXISTS jobs_cluster_duration_starttime;
DROP INDEX IF EXISTS jobs_cluster_jobstate_duration_starttime;
DROP INDEX IF EXISTS jobs_cluster_jobstate_starttime_duration;
DROP INDEX IF EXISTS jobs_cluster_user;
DROP INDEX IF EXISTS jobs_cluster_project;
DROP INDEX IF EXISTS jobs_cluster_subcluster;
DROP INDEX IF EXISTS jobs_cluster_numnodes;
DROP INDEX IF EXISTS jobs_user_starttime_duration;
DROP INDEX IF EXISTS jobs_project_starttime_duration;
DROP INDEX IF EXISTS jobs_jobstate_project;
DROP INDEX IF EXISTS jobs_jobstate_user;
DROP INDEX IF EXISTS jobs_jobstate_duration_starttime;
DROP INDEX IF EXISTS jobs_arrayjobid;
DROP INDEX IF EXISTS jobs_cluster_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_numacc;
DROP INDEX IF EXISTS jobs_cluster_energy;
DROP INDEX IF EXISTS jobs_cluster_partition_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate;

-- ============================================================
-- Recreate all indexes from migration 09
-- ============================================================

-- Cluster Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_user ON job (cluster, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_project ON job (cluster, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_subcluster ON job (cluster, subcluster);
-- Cluster Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_numnodes ON job (cluster, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_numhwthreads ON job (cluster, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_numacc ON job (cluster, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_energy ON job (cluster, energy);

-- Cluster Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_duration_starttime ON job (cluster, duration, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_starttime_duration ON job (cluster, start_time, duration);

-- Cluster+Partition Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_user ON job (cluster, cluster_partition, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_project ON job (cluster, cluster_partition, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate ON job (cluster, cluster_partition, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_shared ON job (cluster, cluster_partition, shared);

-- Cluster+Partition Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numnodes ON job (cluster, cluster_partition, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numhwthreads ON job (cluster, cluster_partition, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numacc ON job (cluster, cluster_partition, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_energy ON job (cluster, cluster_partition, energy);

-- Cluster+Partition Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_duration_starttime ON job (cluster, cluster_partition, duration, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_starttime_duration ON job (cluster, cluster_partition, start_time, duration);

-- Cluster+JobState Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user ON job (cluster, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project ON job (cluster, job_state, project);
-- Cluster+JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numnodes ON job (cluster, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numhwthreads ON job (cluster, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numacc ON job (cluster, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_energy ON job (cluster, job_state, energy);

-- Cluster+JobState Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_starttime_duration ON job (cluster, job_state, start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_duration_starttime ON job (cluster, job_state, duration, start_time);

-- Cluster+Shared Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_user ON job (cluster, shared, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_project ON job (cluster, shared, project);
-- Cluster+Shared Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_numnodes ON job (cluster, shared, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_numhwthreads ON job (cluster, shared, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_numacc ON job (cluster, shared, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_energy ON job (cluster, shared, energy);

-- Cluster+Shared Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_starttime_duration ON job (cluster, shared, start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_shared_duration_starttime ON job (cluster, shared, duration, start_time);

-- User Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_user_numnodes ON job (hpc_user, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_user_numhwthreads ON job (hpc_user, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_user_numacc ON job (hpc_user, num_acc);
CREATE INDEX IF NOT EXISTS jobs_user_energy ON job (hpc_user, energy);

-- User Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_user_starttime_duration ON job (hpc_user, start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_user_duration_starttime ON job (hpc_user, duration, start_time);

-- Project Filter
CREATE INDEX IF NOT EXISTS jobs_project_user ON job (project, hpc_user);
-- Project Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_project_numnodes ON job (project, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_project_numhwthreads ON job (project, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_project_numacc ON job (project, num_acc);
CREATE INDEX IF NOT EXISTS jobs_project_energy ON job (project, energy);

-- Project Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_project_starttime_duration ON job (project, start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_project_duration_starttime ON job (project, duration, start_time);

-- JobState Filter
CREATE INDEX IF NOT EXISTS jobs_jobstate_user ON job (job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_jobstate_project ON job (job_state, project);
-- JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_jobstate_numnodes ON job (job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numhwthreads ON job (job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numacc ON job (job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_jobstate_energy ON job (job_state, energy);

-- JobState Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_jobstate_starttime_duration ON job (job_state, start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_jobstate_duration_starttime ON job (job_state, duration, start_time);

-- Shared Filter
CREATE INDEX IF NOT EXISTS jobs_shared_user ON job (shared, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_shared_project ON job (shared, project);
-- Shared Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_shared_numnodes ON job (shared, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_shared_numhwthreads ON job (shared, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_shared_numacc ON job (shared, num_acc);
CREATE INDEX IF NOT EXISTS jobs_shared_energy ON job (shared, energy);

-- Shared Time Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_shared_starttime_duration ON job (shared, start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_shared_duration_starttime ON job (shared, duration, start_time);

-- ArrayJob Filter
CREATE INDEX IF NOT EXISTS jobs_arrayjobid_starttime ON job (array_job_id, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_arrayjobid_starttime ON job (cluster, array_job_id, start_time);

-- Single filters with default starttime sorting
CREATE INDEX IF NOT EXISTS jobs_duration_starttime ON job (duration, start_time);
CREATE INDEX IF NOT EXISTS jobs_numnodes_starttime ON job (num_nodes, start_time);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads_starttime ON job (num_hwthreads, start_time);
CREATE INDEX IF NOT EXISTS jobs_numacc_starttime ON job (num_acc, start_time);
CREATE INDEX IF NOT EXISTS jobs_energy_starttime ON job (energy, start_time);

-- Single filters with duration sorting
CREATE INDEX IF NOT EXISTS jobs_starttime_duration ON job (start_time, duration);
CREATE INDEX IF NOT EXISTS jobs_numnodes_duration ON job (num_nodes, duration);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads_duration ON job (num_hwthreads, duration);
CREATE INDEX IF NOT EXISTS jobs_numacc_duration ON job (num_acc, duration);
CREATE INDEX IF NOT EXISTS jobs_energy_duration ON job (energy, duration);

-- Backup Indices For High Variety Columns
CREATE INDEX IF NOT EXISTS jobs_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS jobs_duration ON job (duration);

-- Optimize DB index usage
PRAGMA optimize;
