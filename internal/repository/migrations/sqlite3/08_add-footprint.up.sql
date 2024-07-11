ALTER TABLE job ADD COLUMN energy REAL NOT NULL DEFAULT 0.0;

ALTER TABLE job ADD COLUMN footprint TEXT DEFAULT NULL;
UPDATE job SET footprint = '{"flops_any_avg": 0.0}';
UPDATE job SET footprint = json_replace(footprint, '$.flops_any_avg', job.flops_any_avg);
UPDATE job SET footprint = json_insert(footprint, '$.mem_bw_avg', job.mem_bw_avg);
UPDATE job SET footprint = json_insert(footprint, '$.mem_used_max', job.mem_used_max);
UPDATE job SET footprint = json_insert(footprint, '$.cpu_load_avg', job.load_avg);
ALTER TABLE job DROP flops_any_avg;
ALTER TABLE job DROP mem_bw_avg;
ALTER TABLE job DROP mem_used_max;
ALTER TABLE job DROP load_avg;
