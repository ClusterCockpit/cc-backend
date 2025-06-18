CREATE TABLE "node" (
    id INTEGER PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    cluster VARCHAR(255) NOT NULL,
    subcluster VARCHAR(255) NOT NULL,
    cpus_allocated INTEGER NOT NULL,
    cpus_total INTEGER NOT NULL,
    memory_allocated INTEGER NOT NULL,
    memory_total INTEGER NOT NULL,
    gpus_allocated INTEGER NOT NULL,
    gpus_total INTEGER NOT NULL,
    node_state VARCHAR(255) NOT NULL
    CHECK (node_state IN (
        'allocated', 'reserved', 'idle', 'mixed',
        'down', 'unknown'
    )),
    health_state VARCHAR(255) NOT NULL
    CHECK (health_state IN (
        'full', 'partial', 'failed'
    )),
    meta_data TEXT,          -- JSON
    UNIQUE (hostname, cluster)
);
