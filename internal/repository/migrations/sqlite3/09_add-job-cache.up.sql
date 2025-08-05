CREATE TABLE "job_cache" (
    id INTEGER PRIMARY KEY,
    job_id BIGINT NOT NULL,
    hpc_cluster VARCHAR(255) NOT NULL,
    subcluster VARCHAR(255) NOT NULL,
    submit_time BIGINT NOT NULL, -- Unix timestamp
    start_time BIGINT NOT NULL DEFAULT 0, -- Unix timestamp
    hpc_user VARCHAR(255) NOT NULL,
    project VARCHAR(255) NOT NULL,
    cluster_partition VARCHAR(255),
    array_job_id BIGINT,
    duration INT NOT NULL,
    walltime INT NOT NULL,
    job_state VARCHAR(255) NOT NULL
    CHECK (job_state IN (
        'boot_fail', 'cancelled', 'completed', 'deadline',
        'failed', 'node_fail', 'out-of-memory', 'pending',
        'preempted', 'running', 'suspended', 'timeout'
    )),
    meta_data TEXT,          -- JSON
    resources TEXT NOT NULL, -- JSON
    num_nodes INT NOT NULL,
    num_hwthreads INT,
    num_acc INT,
    smt TINYINT NOT NULL DEFAULT 1 CHECK (smt IN (0, 1)),
    shared TEXT NOT NULL
    CHECK (shared IN ("none", "single_user", "multi_user")),
    monitoring_status TINYINT NOT NULL DEFAULT 1
    CHECK (monitoring_status IN (0, 1, 2, 3)),
    energy REAL NOT NULL DEFAULT 0.0,
    energy_footprint TEXT DEFAULT NULL,
    footprint TEXT DEFAULT NULL,
    UNIQUE (job_id, cluster, start_time)
);

CREATE TABLE "job_new" (
    id INTEGER PRIMARY KEY,
    job_id BIGINT NOT NULL,
    hpc_cluster TEXT NOT NULL,
    subcluster TEXT NOT NULL,
    submit_time BIGINT NOT NULL DEFAULT 0, -- Unix timestamp
    start_time BIGINT NOT NULL DEFAULT 0, -- Unix timestamp
    hpc_user TEXT NOT NULL,
    project TEXT NOT NULL,
    cluster_partition TEXT,
    array_job_id BIGINT,
    duration INT NOT NULL,
    walltime INT NOT NULL,
    job_state TEXT NOT NULL
    CHECK (job_state IN (
        'boot_fail', 'cancelled', 'completed', 'deadline',
        'failed', 'node_fail', 'out-of-memory', 'pending',
        'preempted', 'running', 'suspended', 'timeout'
    )),
    meta_data TEXT,          -- JSON
    resources TEXT NOT NULL, -- JSON
    num_nodes INT NOT NULL,
    num_hwthreads INT,
    num_acc INT,
    smt INT NOT NULL DEFAULT 1,
    shared TEXT NOT NULL
    CHECK (shared IN ("none", "single_user", "multi_user")),
    monitoring_status TINYINT NOT NULL DEFAULT 1
    CHECK (monitoring_status IN (0, 1, 2, 3)),
    energy REAL NOT NULL DEFAULT 0.0,
    energy_footprint TEXT DEFAULT NULL,
    footprint TEXT DEFAULT NULL,
    UNIQUE (job_id, cluster, start_time)
);

ALTER TABLE job RENAME COLUMN cluster TO hpc_cluster;
INSERT INTO job_new SELECT * FROM job;
DROP TABLE job;
ALTER TABLE job_new RENAME TO job;
