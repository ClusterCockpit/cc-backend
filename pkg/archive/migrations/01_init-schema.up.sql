CREATE TABLE IF NOT EXISTS "job" (
    id INTEGER PRIMARY KEY,
    clustername TEXT NOT NULL,
    job_id INTEGER NOT NULL,
    start_time INTEGER NOT NULL, -- Unix timestamp
    meta_data TEXT,          -- JSON
    metric_data BLOB,
    UNIQUE ("job_id", "clustername", "start_time")
);
