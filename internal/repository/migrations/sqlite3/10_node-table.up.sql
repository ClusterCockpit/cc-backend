CREATE TABLE "node" (
    id INTEGER PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    cluster VARCHAR(255) NOT NULL,
    subcluster VARCHAR(255) NOT NULL,
    meta_data TEXT,          -- JSON
    UNIQUE (hostname, cluster)
);

CREATE TABLE "node_state" (
    id INTEGER PRIMARY KEY,
    time_stamp INTEGER NOT NULL,
    jobs_running INTEGER DEFAULT 0 NOT NULL,
    cpus_allocated INTEGER DEFAULT 0 NOT NULL,
    memory_allocated INTEGER DEFAULT 0 NOT NULL,
    gpus_allocated INTEGER DEFAULT 0 NOT NULL,
    node_state VARCHAR(255) NOT NULL
    CHECK (node_state IN (
        'allocated', 'reserved', 'idle', 'mixed',
        'down', 'unknown'
    )),
    health_state VARCHAR(255) NOT NULL
    CHECK (health_state IN (
        'full', 'partial', 'failed'
    )),
    health_metrics TEXT,          -- JSON array of strings
    node_id INTEGER,
    FOREIGN KEY (node_id) REFERENCES node (id)
);

-- Add NEW Indices For New Node Table VARCHAR Fields
CREATE INDEX IF NOT EXISTS nodes_cluster ON node (cluster);
CREATE INDEX IF NOT EXISTS nodes_cluster_subcluster ON node (cluster, subcluster);

-- Add NEW Indices For New Node_State Table Fields
CREATE INDEX IF NOT EXISTS nodestates_timestamp ON node_state (time_stamp);
CREATE INDEX IF NOT EXISTS nodestates_state_timestamp ON node_state (node_state, time_stamp);
CREATE INDEX IF NOT EXISTS nodestates_health_timestamp ON node_state (health_state, time_stamp);
CREATE INDEX IF NOT EXISTS nodestates_nodeid_state ON node_state (node_id, node_state);
CREATE INDEX IF NOT EXISTS nodestates_nodeid_health ON node_state (node_id, health_state);
CREATE INDEX IF NOT EXISTS nodestates_nodeid_timestamp ON node_state (node_id, time_stamp DESC);

-- Add NEW Indices For Increased Amounts of Tags
CREATE INDEX IF NOT EXISTS tags_jobid ON jobtag (job_id);
CREATE INDEX IF NOT EXISTS tags_tagid ON jobtag (tag_id);

-- Optimize DB index usage
PRAGMA optimize;