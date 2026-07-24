CREATE TABLE "service" (
    id INTEGER PRIMARY KEY,
    cluster VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    service_type VARCHAR(255) NOT NULL,
    instance_id VARCHAR(64) NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'active', 'stale', 'deregistered')),
    registered_at INTEGER NOT NULL,
    last_heartbeat INTEGER,
    config_revision INTEGER NOT NULL DEFAULT 0,
    meta_data TEXT,          -- JSON
    UNIQUE (cluster, hostname, service_type),
    UNIQUE (instance_id)
);

CREATE INDEX IF NOT EXISTS services_cluster ON service (cluster);
CREATE INDEX IF NOT EXISTS services_state_heartbeat ON service (state, last_heartbeat);

PRAGMA optimize;
