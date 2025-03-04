CREATE TABLE IF NOT EXISTS job (
    id                INTEGER AUTO_INCREMENT PRIMARY KEY ,
    job_id            BIGINT NOT NULL,
    cluster           VARCHAR(255) NOT NULL,
    subcluster        VARCHAR(255) NOT NULL,
    start_time        BIGINT NOT NULL, -- Unix timestamp

    user              VARCHAR(255) NOT NULL,
    project           VARCHAR(255) NOT NULL,
    `partition`       VARCHAR(255) NOT NULL,
    array_job_id      BIGINT NOT NULL,
    duration          INT NOT NULL DEFAULT 0,
    walltime          INT NOT NULL DEFAULT 0,
    job_state         VARCHAR(255) NOT NULL 
    CHECK(job_state IN ('running', 'completed', 'failed', 'cancelled',
            'stopped', 'timeout', 'preempted', 'out_of_memory')),
    meta_data         TEXT,          -- JSON
    resources         TEXT NOT NULL, -- JSON

    num_nodes         INT NOT NULL,
    num_hwthreads     INT NOT NULL,
    num_acc           INT NOT NULL,
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
    UNIQUE (job_id, cluster, start_time)
    );

CREATE TABLE IF NOT EXISTS tag (
    id       INTEGER PRIMARY KEY,
    tag_type VARCHAR(255) NOT NULL,
    tag_name VARCHAR(255) NOT NULL,
    UNIQUE (tag_type, tag_name));

CREATE TABLE IF NOT EXISTS jobtag (
    job_id INTEGER,
    tag_id INTEGER,
    PRIMARY KEY (job_id, tag_id),
    FOREIGN KEY (job_id) REFERENCES job (id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE);

CREATE TABLE IF NOT EXISTS user (
	username varchar(255) PRIMARY KEY NOT NULL,
	password varchar(255) DEFAULT NULL,
	ldap     tinyint      NOT NULL DEFAULT 0, /* col called "ldap" for historic reasons, fills the "AuthSource" */
	name     varchar(255) DEFAULT NULL,
	roles    varchar(255) NOT NULL DEFAULT "[]",
	email    varchar(255) DEFAULT NULL);

CREATE TABLE IF NOT EXISTS configuration (
	username varchar(255),
	confkey  varchar(255),
	value    varchar(255),
	PRIMARY KEY (username, confkey),
	FOREIGN KEY (username) REFERENCES user (username) ON DELETE CASCADE ON UPDATE NO ACTION);


