CREATE TABLE "node" (
    id INTEGER PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    cluster VARCHAR(255) NOT NULL,
    subcluster VARCHAR(255) NOT NULL,
    node_state VARCHAR(255) NOT NULL
    CHECK (job_state IN (
        'allocated', 'reserved', 'idle', 'mixed',
        'down', 'unknown'
    )),
    health_state VARCHAR(255) NOT NULL
    CHECK (job_state IN (
        'full', 'partial', 'failed'
    )),
    meta_data TEXT,          -- JSON
    UNIQUE (hostname, cluster)
);
