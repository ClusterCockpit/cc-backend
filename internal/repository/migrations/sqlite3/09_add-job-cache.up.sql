CREATE TABLE "job_cache" (
    id INTEGER PRIMARY KEY,
    job_id BIGINT NOT NULL,
    cluster VARCHAR(255) NOT NULL,
    subcluster VARCHAR(255) NOT NULL,
    submit_time BIGINT NOT NULL DEFAULT 0, -- Unix timestamp
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
        'failed', 'node_fail', 'out_of_memory', 'pending',
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
    cluster TEXT NOT NULL,
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
        'failed', 'node_fail', 'out_of_memory', 'pending',
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


CREATE TABLE IF NOT EXISTS lookup_exclusive (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO lookup_exclusive (id, name) VALUES
(0, 'multi_user'),
(1, 'none'),
(2, 'single_user');

INSERT INTO job_new (
    id, job_id, cluster, subcluster, submit_time, start_time, hpc_user, project,
    cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources,
    num_nodes, num_hwthreads, num_acc, smt, shared, monitoring_status, energy,
    energy_footprint, footprint
) SELECT
    id,
    job_id,
    cluster,
    subcluster,
    0,
    start_time,
    hpc_user,
    project,
    cluster_partition,
    array_job_id,
    duration,
    walltime,
    job_state,
    meta_data,
    resources,
    num_nodes,
    num_hwthreads,
    num_acc,
    smt,
    (
        SELECT name FROM lookup_exclusive
        WHERE id = job.exclusive
    ),
    monitoring_status,
    energy,
    energy_footprint,
    footprint
FROM job;

DROP TABLE lookup_exclusive;
DROP TABLE job; -- Deletes All Existing 'job' Indices; Recreate after Renaming
ALTER TABLE job_new RENAME TO job;

-- Recreate Indices from 08_add-footprint, include new submit_time indices
-- Cluster Filter
CREATE INDEX IF NOT EXISTS jobs_cluster ON job (cluster);
CREATE INDEX IF NOT EXISTS jobs_cluster_user ON job (cluster, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_project ON job (cluster, project);
CREATE INDEX IF NOT EXISTS jobs_cluster_subcluster ON job (cluster, subcluster);
-- Cluster Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_starttime ON job (cluster, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_submittime ON job (cluster, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_duration ON job (cluster, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_numnodes ON job (cluster, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_numhwthreads ON job (cluster, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_numacc ON job (cluster, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_energy ON job (cluster, energy);

-- Cluster+Partition Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition ON job (cluster, cluster_partition);
-- Cluster+Partition Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_starttime ON job (cluster, cluster_partition, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_submittime ON job (cluster, cluster_partition, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_duration ON job (cluster, cluster_partition, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numnodes ON job (cluster, cluster_partition, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numhwthreads ON job (cluster, cluster_partition, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_numacc ON job (cluster, cluster_partition, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_energy ON job (cluster, cluster_partition, energy);

-- Cluster+Partition+Jobstate Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate ON job (cluster, cluster_partition, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_user ON job (cluster, cluster_partition, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_project ON job (cluster, cluster_partition, job_state, project);
-- Cluster+Partition+Jobstate Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_starttime ON job (cluster, cluster_partition, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_submittime ON job (cluster, cluster_partition, job_state, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_duration ON job (cluster, cluster_partition, job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numnodes ON job (cluster, cluster_partition, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numhwthreads ON job (cluster, cluster_partition, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_numacc ON job (cluster, cluster_partition, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_partition_jobstate_energy ON job (cluster, cluster_partition, job_state, energy);

-- Cluster+JobState Filter
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate ON job (cluster, job_state);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_user ON job (cluster, job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_project ON job (cluster, job_state, project);
-- Cluster+JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_starttime ON job (cluster, job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_submittime ON job (cluster, job_state, submit_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_duration ON job (cluster, job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numnodes ON job (cluster, job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numhwthreads ON job (cluster, job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_numacc ON job (cluster, job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_cluster_jobstate_energy ON job (cluster, job_state, energy);

-- User Filter
CREATE INDEX IF NOT EXISTS jobs_user ON job (hpc_user);
-- User Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_user_starttime ON job (hpc_user, start_time);
CREATE INDEX IF NOT EXISTS jobs_user_duration ON job (hpc_user, duration);
CREATE INDEX IF NOT EXISTS jobs_user_numnodes ON job (hpc_user, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_user_numhwthreads ON job (hpc_user, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_user_numacc ON job (hpc_user, num_acc);
CREATE INDEX IF NOT EXISTS jobs_user_energy ON job (hpc_user, energy);

-- Project Filter
CREATE INDEX IF NOT EXISTS jobs_project ON job (project);
CREATE INDEX IF NOT EXISTS jobs_project_user ON job (project, hpc_user);
-- Project Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_project_starttime ON job (project, start_time);
CREATE INDEX IF NOT EXISTS jobs_project_duration ON job (project, duration);
CREATE INDEX IF NOT EXISTS jobs_project_numnodes ON job (project, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_project_numhwthreads ON job (project, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_project_numacc ON job (project, num_acc);
CREATE INDEX IF NOT EXISTS jobs_project_energy ON job (project, energy);

-- JobState Filter
CREATE INDEX IF NOT EXISTS jobs_jobstate ON job (job_state);
CREATE INDEX IF NOT EXISTS jobs_jobstate_user ON job (job_state, hpc_user);
CREATE INDEX IF NOT EXISTS jobs_jobstate_project ON job (job_state, project);
CREATE INDEX IF NOT EXISTS jobs_jobstate_cluster ON job (job_state, cluster);
-- JobState Filter Sorting
CREATE INDEX IF NOT EXISTS jobs_jobstate_starttime ON job (job_state, start_time);
CREATE INDEX IF NOT EXISTS jobs_jobstate_duration ON job (job_state, duration);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numnodes ON job (job_state, num_nodes);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numhwthreads ON job (job_state, num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_jobstate_numacc ON job (job_state, num_acc);
CREATE INDEX IF NOT EXISTS jobs_jobstate_energy ON job (job_state, energy);

-- ArrayJob Filter
CREATE INDEX IF NOT EXISTS jobs_arrayjobid_starttime ON job (array_job_id, start_time);
CREATE INDEX IF NOT EXISTS jobs_cluster_arrayjobid_starttime ON job (cluster, array_job_id, start_time);

-- Sorting without active filters
CREATE INDEX IF NOT EXISTS jobs_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS jobs_duration ON job (duration);
CREATE INDEX IF NOT EXISTS jobs_numnodes ON job (num_nodes);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads ON job (num_hwthreads);
CREATE INDEX IF NOT EXISTS jobs_numacc ON job (num_acc);
CREATE INDEX IF NOT EXISTS jobs_energy ON job (energy);

-- Single filters with default starttime sorting
CREATE INDEX IF NOT EXISTS jobs_duration_starttime ON job (duration, start_time);
CREATE INDEX IF NOT EXISTS jobs_numnodes_starttime ON job (num_nodes, start_time);
CREATE INDEX IF NOT EXISTS jobs_numhwthreads_starttime ON job (num_hwthreads, start_time);
CREATE INDEX IF NOT EXISTS jobs_numacc_starttime ON job (num_acc, start_time);
CREATE INDEX IF NOT EXISTS jobs_energy_starttime ON job (energy, start_time);

-- Optimize DB index usage
PRAGMA optimize;

-- Optimize DB size: https://sqlite.org/lang_vacuum.html 
-- Not allowed within a migration transaction; Keep command here for documentation and recommendation
-- Command: 'VACUUM;'