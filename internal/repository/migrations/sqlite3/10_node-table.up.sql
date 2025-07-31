CREATE TABLE "node" (
    id INTEGER PRIMARY KEY,
    time_stamp INTEGER NOT NULL,
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

-- Add Indices For New Node Table VARCHAR Fields
CREATE INDEX IF NOT EXISTS nodes_cluster ON node (cluster);
CREATE INDEX IF NOT EXISTS nodes_cluster_subcluster ON node (cluster, subcluster);
CREATE INDEX IF NOT EXISTS nodes_state ON node (node_state);
CREATE INDEX IF NOT EXISTS nodes_cluster_state ON node (cluster, node_state);
CREATE INDEX IF NOT EXISTS nodes_health ON node (health_state);
CREATE INDEX IF NOT EXISTS nodes_cluster_health ON node (cluster, health_state);

-- Add Indices For Increased Amounts of Tags
CREATE INDEX IF NOT EXISTS tags_jobid ON jobtag (job_id);
CREATE INDEX IF NOT EXISTS tags_tagid ON jobtag (tag_id);
