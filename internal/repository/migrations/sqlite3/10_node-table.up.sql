-- sqlfluff:dialect:sqlite
--
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
    cpus_total INTEGER DEFAULT 0 NOT NULL,
    memory_total INTEGER DEFAULT 0 NOT NULL,
    gpus_total INTEGER DEFAULT 0 NOT NULL,
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

-- DROP indices using old column name "cluster"
DROP INDEX IF EXISTS jobs_cluster;
DROP INDEX IF EXISTS jobs_cluster_user;
DROP INDEX IF EXISTS jobs_cluster_project;
DROP INDEX IF EXISTS jobs_cluster_subcluster;
DROP INDEX IF EXISTS jobs_cluster_starttime;
DROP INDEX IF EXISTS jobs_cluster_duration;
DROP INDEX IF EXISTS jobs_cluster_numnodes;
DROP INDEX IF EXISTS jobs_cluster_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_numacc;
DROP INDEX IF EXISTS jobs_cluster_energy;
DROP INDEX IF EXISTS jobs_cluster_partition;
DROP INDEX IF EXISTS jobs_cluster_partition_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_duration;
DROP INDEX IF EXISTS jobs_cluster_partition_numnodes;
DROP INDEX IF EXISTS jobs_cluster_partition_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_partition_numacc;
DROP INDEX IF EXISTS jobs_cluster_partition_energy;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_project;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_starttime;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_duration;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_numacc;
DROP INDEX IF EXISTS jobs_cluster_partition_jobstate_energy;
DROP INDEX IF EXISTS jobs_cluster_jobstate;
DROP INDEX IF EXISTS jobs_cluster_jobstate_user;
DROP INDEX IF EXISTS jobs_cluster_jobstate_project;
DROP INDEX IF EXISTS jobs_cluster_jobstate_starttime;
DROP INDEX IF EXISTS jobs_cluster_jobstate_duration;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numnodes;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numhwthreads;
DROP INDEX IF EXISTS jobs_cluster_jobstate_numacc;
DROP INDEX IF EXISTS jobs_cluster_jobstate_energy;

-- -- CREATE UPDATED indices with new column names
-- Cluster Filter
CREATE INDEX IF NOT EXISTS jobs_cluster ON job (hpc_cluster);
CREATE INDEX IF NOT EXISTS jobs_cluster_user ON job (hpc_cluster, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_project ON job (hpc_cluster, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_subcluster ON job (hpc_cluster, subcluster);
-- Cluster Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_starttime ON job (hpc_cluster, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_duration ON job (hpc_cluster, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_numnodes ON job (hpc_cluster, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_numhwthreads ON job (hpc_cluster, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_numacc ON job (hpc_cluster, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_energy ON job (hpc_cluster, energy);
-- Cluster+Partition Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition ON job (hpc_cluster, cluster_partition);
-- Cluster+Partition Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_starttime ON job (hpc_cluster, cluster_partition, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_duration ON job (hpc_cluster, cluster_partition, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numnodes ON job (hpc_cluster, cluster_partition, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numhwthreads ON job (hpc_cluster, cluster_partition, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numacc ON job (hpc_cluster, cluster_partition, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_energy ON job (hpc_cluster, cluster_partition, energy);
-- Cluster+Partition+Jobstate Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate ON job (hpc_cluster, cluster_partition, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_user ON job (hpc_cluster, cluster_partition, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_project ON job (hpc_cluster, cluster_partition, job_state, project);
-- Cluster+Partition+Jobstate Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_starttime ON job (hpc_cluster, cluster_partition, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_duration ON job (hpc_cluster, cluster_partition, job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numnodes ON job (hpc_cluster, cluster_partition, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numhwthreads ON job (hpc_cluster, cluster_partition, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numacc ON job (hpc_cluster, cluster_partition, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_energy ON job (hpc_cluster, cluster_partition, job_state, energy);
-- Cluster+JobState Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate ON job (hpc_cluster, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user ON job (hpc_cluster, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project ON job (hpc_cluster, job_state, project);
-- Cluster+JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_starttime ON job (hpc_cluster, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_duration ON job (hpc_cluster, job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numnodes ON job (hpc_cluster, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numhwthreads ON job (hpc_cluster, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numacc ON job (hpc_cluster, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_energy ON job (hpc_cluster, job_state, energy);
--- --- END UPDATE existing indices

-- Add NEW Indices For New Job Table Columns
CREATE INDEX IF NOT EXISTS jobs_cluster_submittime ON job (hpc_cluster, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_submittime ON job (hpc_cluster, cluster_partition, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_submittime ON job (hpc_cluster, cluster_partition, job_state, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_submittime ON job (hpc_cluster, job_state, submit_time);

-- Add NEW Indices For New Node Table VARCHAR Fields
CREATE INDEX IF NOT EXISTS nodes_cluster ON node (cluster);
CREATE INDEX IF NOT EXISTS nodes_cluster_subcluster ON node (cluster, subcluster);

-- Add NEW Indices For New Node_State Table Fields
CREATE INDEX IF NOT EXISTS nodeStates_state ON node_state (node_state);
CREATE INDEX IF NOT EXISTS nodeStates_health ON node_state (health_state);
CREATE INDEX IF NOT EXISTS nodeStates_nodeid_state ON node (node_id, node_state);
CREATE INDEX IF NOT EXISTS nodeStates_nodeid_health ON node (node_id, health_state);

-- Add NEW Indices For Increased Amounts of Tags
CREATE INDEX IF NOT EXISTS tags_jobid ON jobtag (job_id);
CREATE INDEX IF NOT EXISTS tags_tagid ON jobtag (tag_id);
