CREATE INDEX IF NOT EXISTS job_stats        ON job (cluster, subcluster, "user");
CREATE INDEX IF NOT EXISTS job_by_user      ON job ("user");
CREATE INDEX IF NOT EXISTS job_by_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS job_by_job_id    ON job (job_id);
CREATE INDEX IF NOT EXISTS job_list         ON job (cluster, job_state);
CREATE INDEX IF NOT EXISTS job_list_user    ON job ("user", cluster, job_state);
CREATE INDEX IF NOT EXISTS job_list_users   ON job ("user", job_state);
CREATE INDEX IF NOT EXISTS job_list_users_start ON job (start_time, "user", job_state);
