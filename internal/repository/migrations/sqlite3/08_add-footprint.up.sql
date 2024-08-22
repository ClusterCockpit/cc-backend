CREATE INDEX IF NOT EXISTS job_by_project ON job (project);
CREATE INDEX IF NOT EXISTS job_list_projects ON job (project, job_state);

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
