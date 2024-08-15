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
