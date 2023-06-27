CREATE TABLE IF NOT EXISTS job_new (
id                INTEGER PRIMARY KEY,
job_id            BIGINT NOT NULL,
cluster           VARCHAR(255) NOT NULL,
subcluster        VARCHAR(255) NOT NULL,
start_time        BIGINT NOT NULL, -- Unix timestamp
user              VARCHAR(255) NOT NULL,
project           VARCHAR(255) NOT NULL,
partition         VARCHAR(255),
array_job_id      BIGINT,
duration          INT NOT NULL,
walltime          INT NOT NULL,
job_state         VARCHAR(255) NOT NULL
CHECK(job_state IN ('running', 'completed', 'failed', 'cancelled', 'stopped', 'timeout', 'preempted', 'out_of_memory')),
meta_data         TEXT,          -- JSON
resources         TEXT NOT NULL, -- JSON
num_nodes         INT NOT NULL,
num_hwthreads     INT,
num_acc           INT,
smt               TINYINT NOT NULL DEFAULT 1 CHECK(smt               IN (0, 1   )),
exclusive         TINYINT NOT NULL DEFAULT 1 CHECK(exclusive         IN (0, 1, 2)),
monitoring_status TINYINT NOT NULL DEFAULT 1 CHECK(monitoring_status IN (0, 1, 2, 3)),
mem_used_max        REAL NOT NULL DEFAULT 0.0,
flops_any_avg       REAL NOT NULL DEFAULT 0.0,
mem_bw_avg          REAL NOT NULL DEFAULT 0.0,
load_avg            REAL NOT NULL DEFAULT 0.0,
net_bw_avg          REAL NOT NULL DEFAULT 0.0,
net_data_vol_total  REAL NOT NULL DEFAULT 0.0,
file_bw_avg         REAL NOT NULL DEFAULT 0.0,
file_data_vol_total REAL NOT NULL DEFAULT 0.0,
UNIQUE (job_id, cluster, start_time));


UPDATE job SET job_state='cancelled' WHERE job_state='canceled';
INSERT INTO job_new SELECT * FROM job;
DROP TABLE job;
ALTER TABLE job_new RENAME TO job;

CREATE INDEX IF NOT EXISTS job_stats        ON job (cluster,subcluster,user);
CREATE INDEX IF NOT EXISTS job_by_user      ON job (user);
CREATE INDEX IF NOT EXISTS job_by_starttime ON job (start_time);
CREATE INDEX IF NOT EXISTS job_by_job_id    ON job (job_id, cluster, start_time);
CREATE INDEX IF NOT EXISTS job_list         ON job (cluster, job_state);
CREATE INDEX IF NOT EXISTS job_list_user    ON job (user, cluster, job_state);
CREATE INDEX IF NOT EXISTS job_list_users   ON job (user, job_state);
CREATE INDEX IF NOT EXISTS job_list_users_start ON job (start_time, user, job_state);
