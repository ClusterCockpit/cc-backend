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


CREATE TABLE IF NOT EXISTS notifications (
    id INT AUTO_INCREMENT PRIMARY KEY,
    message TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS influxdb_configurations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    type VARCHAR(255) NOT NULL,
    database_name VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    port INT NOT NULL,
    user VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL,
    organization VARCHAR(255) NOT NULL,
    ssl_enabled BOOLEAN NOT NULL,
    batch_size INT NOT NULL,
    retry_interval VARCHAR(255) NOT NULL,
    retry_exponential_base INT NOT NULL,
    max_retries INT NOT NULL,
    max_retry_time VARCHAR(255) NOT NULL,
    meta_as_tags TEXT 
);

CREATE TABLE IF NOT EXISTS realtime_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    log_message TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS lvm_conf (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    minAvailableSpaceGB FLOAT NOT NULL,
    maxAvailableSpaceGB FLOAT NOT NULL
);

-- linux lvm schemas
CREATE TABLE IF NOT EXISTS machines (
    machine_id VARCHAR(255) PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    os_version VARCHAR(255) NOT NULL,
    ip_address VARCHAR(255) NOT NULL
);

CREATE TABLE IF NOT EXISTS logical_volumes (
    lv_id INT AUTO_INCREMENT PRIMARY KEY,
    machine_id VARCHAR(255) NOT NULL,
    lv_name VARCHAR(255) NOT NULL,
    vg_name VARCHAR(255) NOT NULL,
    lv_attr VARCHAR(255) NOT NULL,
    lv_size VARCHAR(255) NOT NULL,
    FOREIGN KEY (machine_id) REFERENCES machines(machine_id)
);

CREATE TABLE IF NOT EXISTS volume_groups (
    vg_id INT AUTO_INCREMENT PRIMARY KEY,
    machine_id VARCHAR(255) NOT NULL,
    vg_name VARCHAR(255) NOT NULL,
    pv_count VARCHAR(255) NOT NULL,
    lv_count VARCHAR(255) NOT NULL,
    snap_count VARCHAR(255) NOT NULL,
    vg_attr VARCHAR(255) NOT NULL,
    vg_size VARCHAR(255) NOT NULL,
    vg_free VARCHAR(255) NOT NULL,
    FOREIGN KEY (machine_id) REFERENCES machines(machine_id)
);

CREATE TABLE IF NOT EXISTS physical_volumes (
    pv_id INT AUTO_INCREMENT PRIMARY KEY,
    machine_id VARCHAR(255) NOT NULL,
    pv_name VARCHAR(255) NOT NULL,
    vg_name VARCHAR(255) NOT NULL,
    pv_fmt VARCHAR(255) NOT NULL,
    pv_attr VARCHAR(255) NOT NULL,
    pv_size VARCHAR(255) NOT NULL,
    pv_free VARCHAR(255) NOT NULL,
    FOREIGN KEY (machine_id) REFERENCES machines(machine_id)
);


CREATE TABLE IF NOT EXISTS lv_storage_issuer (
    id INT AUTO_INCREMENT PRIMARY KEY,
    machine_serial_number VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    username VARCHAR(255) NOT NULL,
    minAvailableSpaceGB FLOAT NOT NULL,
    maxAvailableSpaceGB FLOAT NOT NULL,
    FOREIGN KEY (machine_serial_number) REFERENCES machines(machine_id)
);

CREATE TABLE IF NOT EXISTS machine_conf (
    id INT AUTO_INCREMENT PRIMARY KEY,
    machine_serial_number VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    username VARCHAR(255) NOT NULL,
    passphrase LONGTEXT,
    port_number INT NOT NULL,
    password VARCHAR(255),
    host_key VARCHAR(255),
    folder_path VARCHAR(255) ,
    FOREIGN KEY (machine_serial_number) REFERENCES machines(machine_id)
);
