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

CREATE INDEX jobs_cluster IF NOT EXISTS ON job (cluster);
CREATE INDEX jobs_cluster_starttime IF NOT EXISTS ON job (cluster, start_time);
CREATE INDEX jobs_cluster_user IF NOT EXISTS ON job (cluster, user);
CREATE INDEX jobs_cluster_project IF NOT EXISTS ON job (cluster, project);
CREATE INDEX jobs_cluster_subcluster IF NOT EXISTS ON job (cluster, subcluster);

CREATE INDEX jobs_cluster_partition IF NOT EXISTS ON job (cluster, partition);
CREATE INDEX jobs_cluster_partition_starttime IF NOT EXISTS ON job (cluster, partition, start_time);
CREATE INDEX jobs_cluster_partition_jobstate IF NOT EXISTS ON job (cluster, partition, job_state);
CREATE INDEX jobs_cluster_partition_jobstate_user IF NOT EXISTS ON job (cluster, partition, job_state, user);
CREATE INDEX jobs_cluster_partition_jobstate_project IF NOT EXISTS ON job (cluster, partition, job_state, project);
CREATE INDEX jobs_cluster_partition_jobstate_starttime IF NOT EXISTS ON job (cluster, partition, job_state, start_time);

CREATE INDEX jobs_cluster_jobstate IF NOT EXISTS ON job (cluster, job_state);
CREATE INDEX jobs_cluster_jobstate_starttime IF NOT EXISTS ON job (cluster, job_state, starttime);
CREATE INDEX jobs_cluster_jobstate_user IF NOT EXISTS ON job (cluster, job_state, user);
CREATE INDEX jobs_cluster_jobstate_project IF NOT EXISTS ON job (cluster, job_state, project);

CREATE INDEX jobs_user IF NOT EXISTS ON job (user);
CREATE INDEX jobs_user_starttime IF NOT EXISTS ON job (user, start_time);

CREATE INDEX jobs_project IF NOT EXISTS ON job (project);
CREATE INDEX jobs_project_starttime IF NOT EXISTS ON job (project, start_time);
CREATE INDEX jobs_project_user IF NOT EXISTS ON job (project, user);

CREATE INDEX jobs_jobstate IF NOT EXISTS ON job (job_state);
CREATE INDEX jobs_jobstate_user IF NOT EXISTS ON job (job_state, user);
CREATE INDEX jobs_jobstate_project IF NOT EXISTS ON job (job_state, project);
CREATE INDEX jobs_jobstate_cluster IF NOT EXISTS ON job (job_state, cluster);
CREATE INDEX jobs_jobstate_starttime IF NOT EXISTS ON job (job_state, start_time);

CREATE INDEX jobs_arrayjobid_starttime IF NOT EXISTS ON job (array_job_id, start_time);
CREATE INDEX jobs_cluster_arrayjobid_starttime IF NOT EXISTS ON job (cluster, array_job_id, start_time);

CREATE INDEX jobs_starttime IF NOT EXISTS ON job (start_time);
CREATE INDEX jobs_duration IF NOT EXISTS ON job (duration);
CREATE INDEX jobs_numnodes IF NOT EXISTS ON job (num_nodes);
CREATE INDEX jobs_numhwthreads IF NOT EXISTS ON job (num_hwthreads);
CREATE INDEX jobs_numacc IF NOT EXISTS ON job (num_acc);

PRAGMA optimize;
