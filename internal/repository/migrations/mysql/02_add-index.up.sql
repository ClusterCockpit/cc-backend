CREATE INDEX IF NOT EXISTS job_stats        ON job (cluster,subcluster,user);
CREATE INDEX IF NOT EXISTS job_by_user      ON job (user);
CREATE INDEX IF NOT EXISTS job_by_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS job_by_job_id    ON job (job_id);
CREATE INDEX IF NOT EXISTS job_by_state     ON job (job_state);
