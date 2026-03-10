-- Expression indexes on footprint JSON fields for WHERE and ORDER BY optimization.
-- SQLite matches expressions textually, so queries must use exactly:
--   json_extract(footprint, '$.field')

-- Standalone expression indexes (for filtering and sorting)
CREATE INDEX IF NOT EXISTS jobs_fp_flops_any_avg ON job (json_extract(footprint, '$.flops_any_avg'));
CREATE INDEX IF NOT EXISTS jobs_fp_mem_bw_avg ON job (json_extract(footprint, '$.mem_bw_avg'));
CREATE INDEX IF NOT EXISTS jobs_fp_mem_used_max ON job (json_extract(footprint, '$.mem_used_max'));
CREATE INDEX IF NOT EXISTS jobs_fp_cpu_load_avg ON job (json_extract(footprint, '$.cpu_load_avg'));
CREATE INDEX IF NOT EXISTS jobs_fp_net_bw_avg ON job (json_extract(footprint, '$.net_bw_avg'));
CREATE INDEX IF NOT EXISTS jobs_fp_net_data_vol_total ON job (json_extract(footprint, '$.net_data_vol_total'));
CREATE INDEX IF NOT EXISTS jobs_fp_file_bw_avg ON job (json_extract(footprint, '$.file_bw_avg'));
CREATE INDEX IF NOT EXISTS jobs_fp_file_data_vol_total ON job (json_extract(footprint, '$.file_data_vol_total'));

-- Composite indexes with cluster (for common filter+sort combinations)
CREATE INDEX IF NOT EXISTS jobs_cluster_fp_cpu_load_avg ON job (cluster, json_extract(footprint, '$.cpu_load_avg'));
CREATE INDEX IF NOT EXISTS jobs_cluster_fp_flops_any_avg ON job (cluster, json_extract(footprint, '$.flops_any_avg'));
CREATE INDEX IF NOT EXISTS jobs_cluster_fp_mem_bw_avg ON job (cluster, json_extract(footprint, '$.mem_bw_avg'));
CREATE INDEX IF NOT EXISTS jobs_cluster_fp_mem_used_max ON job (cluster, json_extract(footprint, '$.mem_used_max'));
