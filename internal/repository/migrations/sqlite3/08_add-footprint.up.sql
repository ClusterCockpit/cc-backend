DROP INDEX IF EXISTS job_stats;
DROP INDEX IF EXISTS job_by_user;
DROP INDEX IF EXISTS job_by_starttime;
DROP INDEX IF EXISTS job_by_job_id;
DROP INDEX IF EXISTS job_list;
DROP INDEX IF EXISTS job_list_user;
DROP INDEX IF EXISTS job_list_users;
DROP INDEX IF EXISTS job_list_users_start;

ALTER TABLE job ADD COLUMN energy REAL NOT NULL DEFAULT 0.0;
ALTER TABLE job ADD COLUMN energy_footprint TEXT DEFAULT NULL;

ALTER TABLE job ADD COLUMN footprint TEXT DEFAULT NULL;
ALTER TABLE tag ADD COLUMN tag_scope TEXT NOT NULL DEFAULT 'global';

-- Do not use reserved keywords anymore
ALTER TABLE "user" RENAME TO hpc_user;
ALTER TABLE job RENAME COLUMN "user" TO hpc_user;
ALTER TABLE job RENAME COLUMN "partition" TO cluster_partition;

UPDATE job SET footprint = '{"flops_any_avg": 0.0}';
UPDATE job SET footprint = json_replace(footprint, '$.flops_any_avg', job.flops_any_avg);
UPDATE job SET footprint = json_insert(footprint, '$.mem_bw_avg', job.mem_bw_avg);
UPDATE job SET footprint = json_insert(footprint, '$.mem_used_max', job.mem_used_max);
UPDATE job SET footprint = json_insert(footprint, '$.cpu_load_avg', job.load_avg);
UPDATE job SET footprint = json_insert(footprint, '$.net_bw_avg', job.net_bw_avg) WHERE job.net_bw_avg != 0;
UPDATE job SET footprint = json_insert(footprint, '$.net_data_vol_total', job.net_data_vol_total) WHERE job.net_data_vol_total != 0;
UPDATE job SET footprint = json_insert(footprint, '$.file_bw_avg', job.file_bw_avg) WHERE job.file_bw_avg != 0;
UPDATE job SET footprint = json_insert(footprint, '$.file_data_vol_total', job.file_data_vol_total) WHERE job.file_data_vol_total != 0;

ALTER TABLE job DROP flops_any_avg;
ALTER TABLE job DROP mem_bw_avg;
ALTER TABLE job DROP mem_used_max;
ALTER TABLE job DROP load_avg;
ALTER TABLE job DROP net_bw_avg;
ALTER TABLE job DROP net_data_vol_total;
ALTER TABLE job DROP file_bw_avg;
ALTER TABLE job DROP file_data_vol_total;

-- Indices for: Single filters, combined filters, sorting, sorting with filters
-- Cluster Filter
CREATE INDEX IF NOT EXISTS jobs_cluster ON job (cluster);
CREATE INDEX IF NOT EXISTS jobs_cluster_user ON job (cluster, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_project ON job (cluster, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_subcluster ON job (cluster, subcluster);
-- Cluster Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_starttime ON job (cluster, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_duration ON job (cluster, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_numnodes ON job (cluster, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_numhwthreads ON job (cluster, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_numacc ON job (cluster, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_energy ON job (cluster, energy);

-- Cluster+Partition Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition ON job (cluster, cluster_partition);
-- Cluster+Partition Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_starttime ON job (cluster, cluster_partition, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_duration ON job (cluster, cluster_partition, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numnodes ON job (cluster, cluster_partition, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numhwthreads ON job (cluster, cluster_partition, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numacc ON job (cluster, cluster_partition, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_energy ON job (cluster, cluster_partition, energy);

-- Cluster+Partition+Jobstate Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate ON job (cluster, cluster_partition, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_user ON job (cluster, cluster_partition, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_project ON job (cluster, cluster_partition, job_state, project);
-- Cluster+Partition+Jobstate Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_starttime ON job (cluster, cluster_partition, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_duration ON job (cluster, cluster_partition, job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numnodes ON job (cluster, cluster_partition, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numhwthreads ON job (cluster, cluster_partition, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numacc ON job (cluster, cluster_partition, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_energy ON job (cluster, cluster_partition, job_state, energy);

-- Cluster+JobState Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate ON job (cluster, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user ON job (cluster, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project ON job (cluster, job_state, project);
-- Cluster+JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_starttime ON job (cluster, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_duration ON job (cluster, job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numnodes ON job (cluster, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numhwthreads ON job (cluster, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numacc ON job (cluster, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_energy ON job (cluster, job_state, energy);

-- User Filter
CREATE INDEX IF NOT EXISTS jobs_user ON job (hpc_user);
-- User Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_user_starttime ON job (hpc_user, start_time);
CREATE INDEX IF NOT EXISTS jobs_user_duration ON job (hpc_user, duration);
CREATE INDEX IF NOT EXISTS jobs_user_numnodes ON job (hpc_user, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_user_numhwthreads ON job (hpc_user, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_user_numacc ON job (hpc_user, num_acc);
CREATE INDEX IF NOT EXISTS jobs_user_energy ON job (hpc_user, energy);

-- Project Filter
CREATE INDEX IF NOT EXISTS jobs_project ON job (project);
CREATE INDEX IF NOT EXISTS jobs_project_user ON job (project, hpc_user);
-- Project Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_project_starttime ON job (project, start_time);
CREATE INDEX IF NOT EXISTS jobs_project_duration ON job (project, duration);
CREATE INDEX IF NOT EXISTS jobs_project_numnodes ON job (project, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_project_numhwthreads ON job (project, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_project_numacc ON job (project, num_acc);
CREATE INDEX IF NOT EXISTS jobs_project_energy ON job (project, energy);

-- JobState Filter
CREATE INDEX IF NOT EXISTS jobs_jobstate ON job (job_state);
CREATE INDEX IF NOT EXISTS jobs_jobstate_user ON job (job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_jobstate_project ON job (job_state, project);
CREATE INDEX IF NOT EXISTS jobs_jobstate_cluster ON job (job_state, cluster);
-- JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_jobstate_starttime ON job (job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_jobstate_duration ON job (job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numnodes ON job (job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numhwthreads ON job (job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numacc ON job (job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_jobstate_energy ON job (job_state, energy);

-- ArrayJob Filter
CREATE INDEX IF NOT EXISTS jobs_arrayjobid_starttime ON job (array_job_id, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_arrayjobid_starttime ON job (cluster, array_job_id, start_time);

-- Sorting without active filters
CREATE INDEX IF NOT EXISTS jobs_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS jobs_duration ON job (duration);
CREATE INDEX IF NOT EXISTS jobs_numnodes ON job (num_nodes);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads ON job (num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_numacc ON job (num_acc);
CREATE INDEX IF NOT EXISTS jobs_energy ON job (energy);

-- Single filters with default starttime sorting
CREATE INDEX IF NOT EXISTS jobs_duration_starttime ON job (duration, start_time);
CREATE INDEX IF NOT EXISTS jobs_numnodes_starttime ON job (num_nodes, start_time);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads_starttime ON job (num_hwthreads, start_time);
CREATE INDEX IF NOT EXISTS jobs_numacc_starttime ON job (num_acc, start_time);
CREATE INDEX IF NOT EXISTS jobs_energy_starttime ON job (energy, start_time);

-- Optimize DB index usage
PRAGMA optimize;
