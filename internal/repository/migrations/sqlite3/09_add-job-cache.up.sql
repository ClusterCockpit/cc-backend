CREATE TABLE "job_cache" (
    id INTEGER PRIMARY KEY,
    job_id BIGINT NOT NULL,
    cluster VARCHAR(255) NOT NULL,
    subcluster VARCHAR(255) NOT NULL,
    start_time BIGINT NOT NULL, -- Unix timestamp
    hpc_user VARCHAR(255) NOT NULL,
    project VARCHAR(255) NOT NULL,
    cluster_partition VARCHAR(255),
    array_job_id BIGINT,
    duration INT NOT NULL,
    walltime INT NOT NULL,
    job_state VARCHAR(255) NOT NULL
    CHECK (job_state IN (
        'running', 'completed', 'failed', 'cancelled',
        'stopped', 'timeout', 'preempted', 'out_of_memory'
    )),
    meta_data TEXT,          -- JSON
    resources TEXT NOT NULL, -- JSON
    num_nodes INT NOT NULL,
    num_hwthreads INT,
    num_acc INT,
    smt TINYINT NOT NULL DEFAULT 1 CHECK (smt IN (0, 1)),
    exclusive TINYINT NOT NULL DEFAULT 1 CHECK (exclusive IN (0, 1, 2)),
    monitoring_status TINYINT NOT NULL DEFAULT 1
    CHECK (monitoring_status IN (0, 1, 2, 3)),
    energy REAL NOT NULL DEFAULT 0.0,
    energy_footprint TEXT DEFAULT NULL,
    footprint TEXT DEFAULT NULL,
    UNIQUE (job_id, cluster, start_time)
);
