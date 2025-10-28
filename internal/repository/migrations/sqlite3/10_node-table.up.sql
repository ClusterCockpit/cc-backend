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
    node_id INTEGER,
    FOREIGN KEY (node_id) REFERENCES node (id)
);

-- Add NEW Indices For New Job Table Columns
CREATE INDEX IF NOT EXISTS jobs_cluster_submittime ON job (cluster, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_submittime ON job (cluster, cluster_partition, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_submittime ON job (
    cluster, cluster_partition, job_state, submit_time
);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_submittime ON job (cluster, job_state, submit_time);

-- Add NEW Indices For New Node Table VARCHAR Fields
CREATE INDEX IF NOT EXISTS nodes_cluster ON node (cluster);
CREATE INDEX IF NOT EXISTS nodes_cluster_subcluster ON node (cluster, subcluster);

-- Add NEW Indices For New Node_State Table Fields
CREATE INDEX IF NOT EXISTS nodestates_state ON node_state (node_state);
CREATE INDEX IF NOT EXISTS nodestates_health ON node_state (health_state);
CREATE INDEX IF NOT EXISTS nodestates_nodeid_state ON node_state (node_id, node_state);
CREATE INDEX IF NOT EXISTS nodestates_nodeid_health ON node_state (node_id, health_state);

-- Add NEW Indices For Increased Amounts of Tags
CREATE INDEX IF NOT EXISTS tags_jobid ON jobtag (job_id);
CREATE INDEX IF NOT EXISTS tags_tagid ON jobtag (tag_id);
