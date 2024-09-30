DROP INDEX job_stats;
DROP INDEX job_by_user;
DROP INDEX job_by_starttime;
DROP INDEX job_by_job_id;
DROP INDEX job_list;
DROP INDEX job_list_user;
DROP INDEX job_list_users;
DROP INDEX job_list_users_start;

ALTER TABLE job ADD COLUMN energy REAL NOT NULL DEFAULT 0.0;
ALTER TABLE job ADD COLUMN energy_footprint TEXT DEFAULT NULL;

ALTER TABLE job ADD COLUMN footprint TEXT DEFAULT NULL;
ALTER TABLE tag ADD COLUMN tag_scope TEXT NOT NULL DEFAULT 'global';

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

CREATE INDEX IF NOT EXISTS jobs_cluster ON job (cluster);
CREATE INDEX IF NOT EXISTS jobs_cluster_starttime ON job (cluster, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_user ON job (cluster, user);
CREATE INDEX IF NOT EXISTS jobs_cluster_project ON job (cluster, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_subcluster ON job (cluster, subcluster);

CREATE INDEX IF NOT EXISTS jobs_cluster_partition ON job (cluster, partition);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_starttime ON job (cluster, partition, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate ON job (cluster, partition, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_user ON job (cluster, partition, job_state, user);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_project ON job (cluster, partition, job_state, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_starttime ON job (cluster, partition, job_state, start_time);

CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate ON job (cluster, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_starttime ON job (cluster, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user ON job (cluster, job_state, user);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project ON job (cluster, job_state, project);

CREATE INDEX IF NOT EXISTS jobs_user ON job (user);
CREATE INDEX IF NOT EXISTS jobs_user_starttime ON job (user, start_time);

CREATE INDEX IF NOT EXISTS jobs_project ON job (project);
CREATE INDEX IF NOT EXISTS jobs_project_starttime ON job (project, start_time);
CREATE INDEX IF NOT EXISTS jobs_project_user ON job (project, user);

CREATE INDEX IF NOT EXISTS jobs_jobstate ON job (job_state);
CREATE INDEX IF NOT EXISTS jobs_jobstate_user ON job (job_state, user);
CREATE INDEX IF NOT EXISTS jobs_jobstate_project ON job (job_state, project);
CREATE INDEX IF NOT EXISTS jobs_jobstate_cluster ON job (job_state, cluster);
CREATE INDEX IF NOT EXISTS jobs_jobstate_starttime ON job (job_state, start_time);

CREATE INDEX IF NOT EXISTS jobs_arrayjobid_starttime ON job (array_job_id, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_arrayjobid_starttime ON job (cluster, array_job_id, start_time);

CREATE INDEX IF NOT EXISTS jobs_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS jobs_energy ON job (energy);
CREATE INDEX IF NOT EXISTS jobs_duration ON job (duration);
CREATE INDEX IF NOT EXISTS jobs_numnodes ON job (num_nodes);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads ON job (num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_numacc ON job (num_acc);

PRAGMA optimize;
