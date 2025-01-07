ALTER TABLE job DROP energy;
ALTER TABLE job DROP energy_footprint;
ALTER TABLE job ADD COLUMN flops_any_avg;
ALTER TABLE job ADD COLUMN mem_bw_avg;
ALTER TABLE job ADD COLUMN mem_used_max;
ALTER TABLE job ADD COLUMN load_avg;
ALTER TABLE job ADD COLUMN net_bw_avg;
ALTER TABLE job ADD COLUMN net_data_vol_total;
ALTER TABLE job ADD COLUMN file_bw_avg;
ALTER TABLE job ADD COLUMN file_data_vol_total;

UPDATE job SET flops_any_avg = json_extract(footprint, '$.flops_any_avg');
UPDATE job SET mem_bw_avg = json_extract(footprint, '$.mem_bw_avg');
UPDATE job SET mem_used_max = json_extract(footprint, '$.mem_used_max');
UPDATE job SET load_avg = json_extract(footprint, '$.cpu_load_avg');
UPDATE job SET net_bw_avg = json_extract(footprint, '$.net_bw_avg');
UPDATE job SET net_data_vol_total = json_extract(footprint, '$.net_data_vol_total');
UPDATE job SET file_bw_avg = json_extract(footprint, '$.file_bw_avg');
UPDATE job SET file_data_vol_total = json_extract(footprint, '$.file_data_vol_total');

ALTER TABLE job DROP footprint;
-- Do not use reserved keywords anymore
RENAME TABLE hpc_user TO `user`;
ALTER TABLE job RENAME COLUMN hpc_user TO `user`;
ALTER TABLE job RENAME COLUMN cluster_partition TO `partition`;

DROP INDEX IF EXISTS jobs_cluster;
DROP INDEX IF EXISTS jobs_cluster_user;
DROP INDEX IF EXISTS jobs_cluster_project;
DROP INDEX IF EXISTS jobs_cluster_subcluster;
DROP INDEX IF EXISTS jobs_cluster_starttime;
DROP INDEX IF EXISTS jobs_cluster_duration;
DROP INDEX IF EXISTS jobs_cluster_numnodes;

DROP INDEX IF EXISTS jobs_cluster_partition;
DROP INDEX IF EXISTS jobs_cluster_partition_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_duration;
DROP INDEX IF EXISTS jobs_cluster_partition_numnodes;

DROP INDEX IF EXISTS jobs_cluster_partition_jobstate;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_project;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_duration;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numnodes;

DROP INDEX IF EXISTS jobs_cluster_jobstate;
DROP INDEX IF EXISTS jobs_cluster_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_jobstate_project;

DROP INDEX IF EXISTS jobs_cluster_jobstate_starttime;
DROP INDEX IF EXISTS jobs_cluster_jobstate_duration;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numnodes;

DROP INDEX IF EXISTS jobs_user;
DROP INDEX IF EXISTS jobs_user_starttime;
DROP INDEX IF EXISTS jobs_user_duration;
DROP INDEX IF EXISTS jobs_user_numnodes;

DROP INDEX IF EXISTS jobs_project;
DROP INDEX IF EXISTS jobs_project_user;
DROP INDEX IF EXISTS jobs_project_starttime;
DROP INDEX IF EXISTS jobs_project_duration;
DROP INDEX IF EXISTS jobs_project_numnodes;

DROP INDEX IF EXISTS jobs_jobstate;
DROP INDEX IF EXISTS jobs_jobstate_user;
DROP INDEX IF EXISTS jobs_jobstate_project;
DROP INDEX IF EXISTS jobs_jobstate_starttime;
DROP INDEX IF EXISTS jobs_jobstate_duration;
DROP INDEX IF EXISTS jobs_jobstate_numnodes;

DROP INDEX IF EXISTS jobs_arrayjobid_starttime;
DROP INDEX IF EXISTS jobs_cluster_arrayjobid_starttime;

DROP INDEX IF EXISTS jobs_starttime;
DROP INDEX IF EXISTS jobs_duration;
DROP INDEX IF EXISTS jobs_numnodes;

DROP INDEX IF EXISTS jobs_duration_starttime;
DROP INDEX IF EXISTS jobs_numnodes_starttime;
DROP INDEX IF EXISTS jobs_numacc_starttime;
DROP INDEX IF EXISTS jobs_energy_starttime;
