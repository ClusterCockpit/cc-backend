-- Drop standalone expression indexes
DROP INDEX IF EXISTS jobs_fp_flops_any_avg;
DROP INDEX IF EXISTS jobs_fp_mem_bw_avg;
DROP INDEX IF EXISTS jobs_fp_mem_used_max;
DROP INDEX IF EXISTS jobs_fp_cpu_load_avg;
DROP INDEX IF EXISTS jobs_fp_net_bw_avg;
DROP INDEX IF EXISTS jobs_fp_net_data_vol_total;
DROP INDEX IF EXISTS jobs_fp_file_bw_avg;
DROP INDEX IF EXISTS jobs_fp_file_data_vol_total;

-- Drop composite indexes
DROP INDEX IF EXISTS jobs_cluster_fp_cpu_load_avg;
DROP INDEX IF EXISTS jobs_cluster_fp_flops_any_avg;
DROP INDEX IF EXISTS jobs_cluster_fp_mem_bw_avg;
DROP INDEX IF EXISTS jobs_cluster_fp_mem_used_max;
