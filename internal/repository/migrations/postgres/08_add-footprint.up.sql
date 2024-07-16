ALTER TABLE job ADD COLUMN energy REAL NOT NULL DEFAULT 0.0;

ALTER TABLE job ADD COLUMN footprint TEXT DEFAULT NULL;
ALTER TABLE job DROP flops_any_avg;
ALTER TABLE job DROP mem_bw_avg;
ALTER TABLE job DROP mem_used_max;
ALTER TABLE job DROP load_avg;

ALTER TABLE "user" RENAME TO users;

CREATE TABLE IF NOT EXISTS job_meta (
    id                SERIAL PRIMARY KEY,
    job_id            BIGINT NOT NULL,
    cluster           VARCHAR(255) NOT NULL,
    start_time        BIGINT NOT NULL, -- Unix timestamp

    meta_data         JSONB,          -- JSON
    metric_data       JSONB,          -- JSON
    UNIQUE (job_id, cluster, start_time)
);