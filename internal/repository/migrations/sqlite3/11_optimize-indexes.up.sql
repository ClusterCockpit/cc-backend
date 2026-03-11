-- Migration 11: Optimize job table indexes
-- Reduces from ~78 indexes to 20 for better write performance,
-- reduced disk usage, and more reliable query planner decisions.
-- Requires ANALYZE to be run after migration (done automatically on startup).

-- ============================================================
-- Drop ALL existing job indexes (from migrations 08/09)
-- sqlite_autoindex_job_1 (UNIQUE constraint) is kept automatically
-- ============================================================

-- Cluster Filter
DROP INDEX IF EXISTS jobs_cluster_user;
DROP INDEX IF EXISTS jobs_cluster_project;
DROP INDEX IF EXISTS jobs_cluster_subcluster;
-- Cluster Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_numnodes;
DROP INDEX IF EXISTS jobs_cluster_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_numacc;
DROP INDEX IF EXISTS jobs_cluster_energy;
-- Cluster Time Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_duration_starttime;
DROP INDEX IF EXISTS jobs_cluster_starttime_duration;

-- Cluster+Partition Filter
DROP INDEX IF EXISTS jobs_cluster_partition_user;
DROP INDEX IF EXISTS jobs_cluster_partition_project;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate;
DROP INDEX IF EXISTS jobs_cluster_partition_shared;
-- Cluster+Partition Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_partition_numnodes;
DROP INDEX IF EXISTS jobs_cluster_partition_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_partition_numacc;
DROP INDEX IF EXISTS jobs_cluster_partition_energy;
-- Cluster+Partition Time Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_partition_duration_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_starttime_duration;

-- Cluster+JobState Filter
DROP INDEX IF EXISTS jobs_cluster_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_jobstate_project;
-- Cluster+JobState Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numacc;
DROP INDEX IF EXISTS jobs_cluster_jobstate_energy;
-- Cluster+JobState Time Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_jobstate_starttime_duration;
DROP INDEX IF EXISTS jobs_cluster_jobstate_duration_starttime;

-- Cluster+Shared Filter
DROP INDEX IF EXISTS jobs_cluster_shared_user;
DROP INDEX IF EXISTS jobs_cluster_shared_project;
-- Cluster+Shared Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_shared_numnodes;
DROP INDEX IF EXISTS jobs_cluster_shared_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_shared_numacc;
DROP INDEX IF EXISTS jobs_cluster_shared_energy;
-- Cluster+Shared Time Filter Sorting
DROP INDEX IF EXISTS jobs_cluster_shared_starttime_duration;
DROP INDEX IF EXISTS jobs_cluster_shared_duration_starttime;

-- User Filter Sorting
DROP INDEX IF EXISTS jobs_user_numnodes;
DROP INDEX IF EXISTS jobs_user_numhwthreads;
DROP INDEX IF EXISTS jobs_user_numacc;
DROP INDEX IF EXISTS jobs_user_energy;
-- User Time Filter Sorting
DROP INDEX IF EXISTS jobs_user_starttime_duration;
DROP INDEX IF EXISTS jobs_user_duration_starttime;

-- Project Filter
DROP INDEX IF EXISTS jobs_project_user;
-- Project Filter Sorting
DROP INDEX IF EXISTS jobs_project_numnodes;
DROP INDEX IF EXISTS jobs_project_numhwthreads;
DROP INDEX IF EXISTS jobs_project_numacc;
DROP INDEX IF EXISTS jobs_project_energy;
-- Project Time Filter Sorting
DROP INDEX IF EXISTS jobs_project_starttime_duration;
DROP INDEX IF EXISTS jobs_project_duration_starttime;

-- JobState Filter
DROP INDEX IF EXISTS jobs_jobstate_user;
DROP INDEX IF EXISTS jobs_jobstate_project;
-- JobState Filter Sorting
DROP INDEX IF EXISTS jobs_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_jobstate_numacc;
DROP INDEX IF EXISTS jobs_jobstate_energy;
-- JobState Time Filter Sorting
DROP INDEX IF EXISTS jobs_jobstate_starttime_duration;
DROP INDEX IF EXISTS jobs_jobstate_duration_starttime;

-- Shared Filter
DROP INDEX IF EXISTS jobs_shared_user;
DROP INDEX IF EXISTS jobs_shared_project;
-- Shared Filter Sorting
DROP INDEX IF EXISTS jobs_shared_numnodes;
DROP INDEX IF EXISTS jobs_shared_numhwthreads;
DROP INDEX IF EXISTS jobs_shared_numacc;
DROP INDEX IF EXISTS jobs_shared_energy;
-- Shared Time Filter Sorting
DROP INDEX IF EXISTS jobs_shared_starttime_duration;
DROP INDEX IF EXISTS jobs_shared_duration_starttime;

-- ArrayJob Filter
DROP INDEX IF EXISTS jobs_arrayjobid_starttime;
DROP INDEX IF EXISTS jobs_cluster_arrayjobid_starttime;

-- Single filters with default starttime sorting
DROP INDEX IF EXISTS jobs_duration_starttime;
DROP INDEX IF EXISTS jobs_numnodes_starttime;
DROP INDEX IF EXISTS jobs_numhwthreads_starttime;
DROP INDEX IF EXISTS jobs_numacc_starttime;
DROP INDEX IF EXISTS jobs_energy_starttime;

-- Single filters with duration sorting
DROP INDEX IF EXISTS jobs_starttime_duration;
DROP INDEX IF EXISTS jobs_numnodes_duration;
DROP INDEX IF EXISTS jobs_numhwthreads_duration;
DROP INDEX IF EXISTS jobs_numacc_duration;
DROP INDEX IF EXISTS jobs_energy_duration;

-- Backup Indices
DROP INDEX IF EXISTS jobs_starttime;
DROP INDEX IF EXISTS jobs_duration;

-- Legacy indexes from migration 08 (may exist on older DBs)
DROP INDEX IF EXISTS jobs_cluster;
DROP INDEX IF EXISTS jobs_cluster_starttime;
DROP INDEX IF EXISTS jobs_cluster_duration;
DROP INDEX IF EXISTS jobs_cluster_partition;
DROP INDEX IF EXISTS jobs_cluster_partition_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_duration;
DROP INDEX IF EXISTS jobs_cluster_partition_numnodes;
DROP INDEX IF EXISTS jobs_cluster_partition_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_partition_numacc;
DROP INDEX IF EXISTS jobs_cluster_partition_energy;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_project;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_duration;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numacc;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_energy;
DROP INDEX IF EXISTS jobs_cluster_jobstate;
DROP INDEX IF EXISTS jobs_cluster_jobstate_starttime;
DROP INDEX IF EXISTS jobs_cluster_jobstate_duration;
DROP INDEX IF EXISTS jobs_user;
DROP INDEX IF EXISTS jobs_user_starttime;
DROP INDEX IF EXISTS jobs_user_duration;
DROP INDEX IF EXISTS jobs_project;
DROP INDEX IF EXISTS jobs_project_starttime;
DROP INDEX IF EXISTS jobs_project_duration;
DROP INDEX IF EXISTS jobs_jobstate;
DROP INDEX IF EXISTS jobs_jobstate_cluster;
DROP INDEX IF EXISTS jobs_jobstate_starttime;
DROP INDEX IF EXISTS jobs_jobstate_duration;
DROP INDEX IF EXISTS jobs_numnodes;
DROP INDEX IF EXISTS jobs_numhwthreads;
DROP INDEX IF EXISTS jobs_numacc;
DROP INDEX IF EXISTS jobs_energy;

-- ============================================================
-- Create optimized set of 20 indexes
-- ============================================================

-- GROUP 1: Global sort (1 index)
-- Default sort for unfiltered/multi-state IN queries, time range, delete-before
CREATE INDEX jobs_starttime ON job (start_time);

-- GROUP 2: Cluster-prefixed (8 indexes)
-- Cluster + default sort, concurrent jobs, time range within cluster
CREATE INDEX jobs_cluster_starttime_duration ON job (cluster, start_time, duration);
-- Cluster + sort by duration
CREATE INDEX jobs_cluster_duration_starttime ON job (cluster, duration, start_time);
-- COVERING for cluster+state aggregation; running jobs (cluster, state, duration>?)
CREATE INDEX jobs_cluster_jobstate_duration_starttime ON job (cluster, job_state, duration, start_time);
-- Cluster+state+sort start_time (single state equality)
CREATE INDEX jobs_cluster_jobstate_starttime_duration ON job (cluster, job_state, start_time, duration);
-- COVERING for GROUP BY user with cluster filter
CREATE INDEX jobs_cluster_user ON job (cluster, hpc_user);
-- GROUP BY project with cluster filter
CREATE INDEX jobs_cluster_project ON job (cluster, project);
-- GROUP BY subcluster with cluster filter
CREATE INDEX jobs_cluster_subcluster ON job (cluster, subcluster);
-- Cluster + sort by num_nodes (state filtered per-row, fast with LIMIT)
CREATE INDEX jobs_cluster_numnodes ON job (cluster, num_nodes);

-- GROUP 3: User-prefixed (1 index)
-- Security filter (user role) + default sort
CREATE INDEX jobs_user_starttime_duration ON job (hpc_user, start_time, duration);

-- GROUP 4: Project-prefixed (1 index)
-- Security filter (manager role) + default sort
CREATE INDEX jobs_project_starttime_duration ON job (project, start_time, duration);

-- GROUP 5: JobState-prefixed (3 indexes)
-- State + project filter (for manager security within state query)
CREATE INDEX jobs_jobstate_project ON job (job_state, project);
-- State + user filter/aggregation
CREATE INDEX jobs_jobstate_user ON job (job_state, hpc_user);
-- COVERING for non-running jobs scan, state + sort duration
CREATE INDEX jobs_jobstate_duration_starttime ON job (job_state, duration, start_time);

-- GROUP 6: Rare filters (1 index)
-- Array job lookup
CREATE INDEX jobs_arrayjobid ON job (array_job_id);

-- GROUP 7: Secondary sort columns (5 indexes)
CREATE INDEX jobs_cluster_numhwthreads ON job (cluster, num_hwthreads);
CREATE INDEX jobs_cluster_numacc ON job (cluster, num_acc);
CREATE INDEX jobs_cluster_energy ON job (cluster, energy);
-- Cluster+partition + sort start_time
CREATE INDEX jobs_cluster_partition_starttime ON job (cluster, cluster_partition, start_time);
-- Cluster+partition+state filter
CREATE INDEX jobs_cluster_partition_jobstate ON job (cluster, cluster_partition, job_state);

-- Optimize DB index usage
PRAGMA optimize;
