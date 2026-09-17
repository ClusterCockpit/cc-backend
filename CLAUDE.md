# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project Overview

ClusterCockpit is a job-specific performance monitoring framework for HPC
clusters. This is a Golang backend that provides REST and GraphQL APIs, serves a
Svelte-based frontend, and manages job archives and metric data from various
time-series databases.

## Build and Development Commands

### Building

```bash
# Build everything (frontend + backend)
make

# Build only the frontend
make frontend

# Build only the backend (requires frontend to be built first)
go build -ldflags='-s -X main.date=$(date +"%Y-%m-%d:T%H:%M:%S") -X main.version=1.5.0 -X main.commit=$(git rev-parse --short HEAD)' ./cmd/cc-backend
```

### Testing

```bash
# Run all tests
make test

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test ./internal/repository
```

### Code Generation

```bash
# Regenerate GraphQL schema and resolvers (after modifying api/schema.graphqls)
make graphql

# Regenerate Swagger/OpenAPI docs (after modifying API comments)
make swagger
```

### Frontend Development

```bash
cd web/frontend

# Install dependencies
npm install

# Build for production
npm run build

# Development mode with watch
npm run dev
```

### Running

```bash
# Initialize database and create admin user
./cc-backend -init-db -add-user demo:admin:demo

# Start server in development mode (enables GraphQL Playground and Swagger UI)
./cc-backend -server -dev -loglevel info

# Start demo with sample data
./startDemo.sh
```

## Architecture

### Backend Structure

The backend follows a layered architecture with clear separation of concerns:

- **cmd/cc-backend**: Entry point, orchestrates initialization of all subsystems
- **internal/repository**: Data access layer using repository pattern
  - Abstracts database operations (SQLite3 only)
  - Implements LRU caching for performance
  - Provides repositories for Job, User, Node, and Tag entities
  - Transaction support for batch operations
- **internal/api**: REST API endpoints (Swagger/OpenAPI documented)
- **internal/graph**: GraphQL API (uses gqlgen)
  - Schema in `api/schema.graphqls`
  - Generated code in `internal/graph/generated/`
  - Resolvers in `internal/graph/schema.resolvers.go`
- **internal/auth**: Authentication layer
  - Supports local accounts, LDAP, OIDC, and JWT tokens
  - Implements rate limiting for login attempts
- **pkg/metricstore**: Metric store with data loading API
  - In-memory metric storage with checkpointing
  - Query API for loading job metric data
- **internal/archiver**: Job archiving to file-based archive
- **internal/logviewer**: Log source for the admin log view
  - Reads the systemd journal via `journalctl` when cc-backend runs as a systemd unit
  - Otherwise captures cclog output in an in-process ring buffer (containers, supervisors, plain shell)
  - Backend resolved once at startup, configurable via `main.log-source`
- **internal/api/nats.go**: NATS-based API for job and node operations
  - Subscribes to NATS subjects for job events (start/stop)
  - Handles node state updates via NATS
  - Uses InfluxDB line protocol message format
- **internal/fleet**: Fleet service discovery and configuration deployment
  - Registry for cluster-scope services, `InfraRegistry` for cluster-independent ones
  - `ConfigStore`: hierarchical JSON config tree, deep-merged broad to specific
  - `FleetPublisher`: per-consumer discovery rosters pushed over NATS
  - `fleet.Init` owns the sweep, config reloader and publisher goroutines
  - Disabled unless the `main.fleet` config block is present
- **internal/api/fleet.go**: REST endpoints for fleet members (register, heartbeat, config pull, deregister)
- **internal/api/nats_fleet.go**: NATS heartbeat consumer, coalescing writes into batched transactions
- **pkg/archive**: Job archive backend implementations
  - File system backend (default)
  - S3 backend
  - SQLite backend (experimental)
  - **parquet** sub-package: Parquet format support (schema, reader, writer, conversion)
- **internal/metricstoreclient**: Client for cc-metric-store queries

### Frontend Structure

- **web/frontend**: Svelte 5 application
  - Uses Rollup for building
  - Components organized by feature (analysis, job, user, etc.)
  - GraphQL client using @urql/svelte
  - Bootstrap 5 + SvelteStrap for UI
  - uPlot for time-series visualization
- **web/templates**: Server-side Go templates

### Key Concepts

**Job Archive**: Completed jobs are stored in a file-based archive following the
[ClusterCockpit job-archive
specification](https://github.com/ClusterCockpit/cc-specifications/tree/master/job-archive).
Each job has a `meta.json` file with metadata and metric data files.

**Metric Data Repositories**: Time-series metric data is stored separately from
job metadata. The system supports multiple backends (cc-metric-store is
recommended). Configuration is per-cluster in `config.json`.

**Authentication Flow**:

1. Multiple authenticators can be configured (local, LDAP, OIDC, JWT)
2. Each authenticator's `CanLogin` method is called to determine if it should handle the request
3. The first authenticator that returns true performs the actual `Login`
4. JWT tokens are used for API authentication

**Database Migrations**: SQL migrations in `internal/repository/migrations/sqlite3/` are
applied automatically on startup. Version tracking in `version` table.

**Scopes**: Metrics can be collected at different scopes:

- Node scope (always available)
- Core scope (for jobs with ≤8 nodes)
- Accelerator scope (for GPU/accelerator metrics)

## Configuration

- **config.json**: Main configuration (clusters, metric repositories, archive settings)
  - `main.log-source`: Backend for the admin log view (optional, default `auto`)
    - `auto`: journald when running as a systemd unit, in-process buffer otherwise
    - `journal`: force `journalctl` against `main.systemd-unit`
    - `memory`: force the in-process ring buffer
    - `disabled`: turn the log view off and hide it from the navbar
  - `main.systemd-unit`: Unit queried in journal mode (default `clustercockpit.service`)
  - `main.log-buffer-size`: Records kept by the in-process buffer (default 4096).
    It only holds messages at or above the `-loglevel` the process was started
    with, so container deployments usually want `-loglevel info`.
  - `main.api-subjects`: NATS subject configuration (optional)
    - `subject-job-event`: Subject for job start/stop events (e.g., "cc.job.event")
    - `subject-node-state`: Subject for node state updates (e.g., "cc.node.state")
    - `job-concurrency`: Worker goroutines for job events (default: 8)
    - `node-concurrency`: Worker goroutines for node state events (default: 2)
  - `main.fleet`: Fleet discovery and configuration service (optional; absent
    disables registry, config deployment, discovery publishing and the heartbeat
    subscription together)
    - `config-dir` (required): Root of the configuration tree served to services
    - `stale-after`: Heartbeat age at which a service becomes `stale` (default "90s")
    - `sweep-interval`: How often the stale sweep runs (default "30s")
    - `config-reload-interval`: How often the config tree is re-scanned (default "1m")
    - `heartbeat-subject`: NATS subject carrying heartbeats (e.g. "cc.fleet.event");
      empty disables the consumer
    - `heartbeat-concurrency`: Worker goroutines decoding heartbeats (default: 2)
    - `discovery-subject-prefix`: Roster subject prefix (default "cc.fleet.discovery")
    - `discovery-interval`: How often all rosters are re-published (default "1m")
  - `nats`: NATS client connection configuration (optional)
    - `address`: NATS server address (e.g., "nats://localhost:4222")
    - `username`: Authentication username (optional; or `CC_NATS_USERNAME`)
    - `password`: Authentication password (optional; or `CC_NATS_PASSWORD`)
    - `creds-file-path`: Path to NATS credentials file (optional)
- **Secrets**: every secret has three sources, in this order of precedence:
  `$VAR`, then the contents of the file named by `$VAR_FILE`, then the value in
  `config.json`. An unreadable or whitespace-only `$VAR_FILE` is fatal, never a
  silent fallback. The former `.env`/godotenv mechanism has been removed.
  - Names are registered in `internal/config/secretenv.go` (`SecretEnvs`), and
    guard tests there enforce that each one is both wired and documented. Add a
    new secret there, never as a bare string literal at the call site.
  - Resolution goes through `util.SecretFromEnv` (cc-lib) for a fixed name, or
    `util.SecretFromConfig` for one instance of a repeated config section.
  - Auth: `JWT_PUBLIC_KEY`, `JWT_PRIVATE_KEY`, `CROSS_LOGIN_JWT_PUBLIC_KEY`,
    `CROSS_LOGIN_JWT_HS512_KEY`, `LDAP_ADMIN_PASSWORD`, `OID_CLIENT_ID`,
    `OID_CLIENT_SECRET`.
  - S3, one distinct set per consumer so none can override another:
    `ARCHIVE_S3_ACCESS_KEY`/`ARCHIVE_S3_SECRET_KEY` (job archive),
    `RETENTION_S3_ACCESS_KEY`/`RETENTION_S3_SECRET_KEY` (job retention target),
    `NODESTATE_S3_ACCESS_KEY`/`NODESTATE_S3_SECRET_KEY` (nodestate retention
    target), and `ARCHIVE_MANAGER_SRC_S3_ACCESS_KEY`,
    `ARCHIVE_MANAGER_SRC_S3_SECRET_KEY`, `ARCHIVE_MANAGER_DST_S3_ACCESS_KEY`,
    `ARCHIVE_MANAGER_DST_S3_SECRET_KEY` (archive-manager).
    - Resolve at the owning config section, **never** inside `S3Archive.Init`
      or `pqarchive.NewS3Target`: those are shared by all five sets, so a fixed
      name there would cross credentials between them.
    - With no key configured the AWS default credential chain still applies.
  - `METRICSTORE_TOKEN` for `metric-store-external[].token`, with
    `METRICSTORE_TOKEN_<SCOPE>` taking precedence (scope uppercased, every
    character outside `A-Z0-9` replaced by `_`).
  - Names cc-backend resolves are unprefixed; names cc-lib resolves carry a
    `CC_` prefix and are referenced from their cc-lib package rather than
    redefined.
- **cluster.json**: Cluster topology and metric definitions (loaded from archive or config)

## Database

- Default: SQLite 3 (`./var/job.db`)
- Connection managed by `internal/repository`
- Schema version in `internal/repository/migration.go`

## Code Generation

**GraphQL** (gqlgen):

- Schema: `api/schema.graphqls`
- Config: `gqlgen.yml`
- Generated code: `internal/graph/generated/`
- Custom resolvers: `internal/graph/schema.resolvers.go`
- Run `make graphql` after schema changes

**Swagger/OpenAPI**:

- Annotations in `internal/api/*.go`
- Generated docs: `internal/api/docs.go`, `api/swagger.yaml`
- Run `make swagger` after API changes

## Testing Conventions

- Test files use `_test.go` suffix
- Test data in `testdata/` subdirectories
- Repository tests use in-memory SQLite
- API tests use httptest

## Common Workflows

### Adding a new GraphQL field

1. Edit schema in `api/schema.graphqls`
2. Run `make graphql`
3. Implement resolver in `internal/graph/schema.resolvers.go`

### Adding a new REST endpoint

1. Add handler in `internal/api/*.go`
2. Add route in `internal/api/rest.go`
3. Add Swagger annotations
4. Run `make swagger`

### Adding a new metric data backend

1. Implement metric loading functions in `pkg/metricstore/query.go`
2. Add cluster configuration to metric store initialization
3. Update config.json schema documentation

### Modifying database schema

1. Create new migration in `internal/repository/migrations/sqlite3/`
2. Increment `repository.Version`
3. Test with fresh database and existing database

## NATS API

The backend supports a NATS-based API as an alternative to the REST API for job and node operations.

### Setup

1. Configure NATS client connection in `config.json`:

   ```json
   {
     "nats": {
       "address": "nats://localhost:4222",
       "username": "user",
       "password": "pass"
     }
   }
   ```

2. Configure API subjects in `config.json` under `main`:

   ```json
   {
     "main": {
       "api-subjects": {
         "subject-job-event": "cc.job.event",
         "subject-node-state": "cc.node.state",
         "job-concurrency": 8,
         "node-concurrency": 2
       }
     }
   }
   ```

   - `subject-job-event` (required): NATS subject for job start/stop events
   - `subject-node-state` (required): NATS subject for node state updates
   - `job-concurrency` (optional, default: 8): Number of concurrent worker goroutines for job events
   - `node-concurrency` (optional, default: 2): Number of concurrent worker goroutines for node state events

### Message Format

Messages use **InfluxDB line protocol** format with the following structure:

#### Job Events

**Start Job:**

```
job,function=start_job event="{\"jobId\":123,\"user\":\"alice\",\"cluster\":\"test\", ...}" 1234567890000000000
```

**Stop Job:**

```
job,function=stop_job event="{\"jobId\":123,\"cluster\":\"test\",\"startTime\":1234567890,\"stopTime\":1234571490,\"jobState\":\"completed\"}" 1234571490000000000
```

**Tags:**

- `function`: Either `start_job` or `stop_job`

**Fields:**

- `event`: JSON payload containing job data (see REST API documentation for schema)

#### Node State Updates

```json
{
  "cluster": "testcluster",
  "nodes": [
    {
      "hostname": "node001",
      "states": ["allocated"],
      "cpusAllocated": 8,
      "memoryAllocated": 16384,
      "gpusAllocated": 0,
      "jobsRunning": 1
    }
  ]
}
```

#### Fleet Heartbeats

```
fleet,function=heartbeat event="{\"instanceId\":\"3f1c9a2b7d4e6f8a0b1c2d3e4f5a6b7c\"}" 1734000000000000000
```

- Subject: `main.fleet.heartbeat-subject`; measurement `fleet`
- `heartbeat` is the **only** accepted `function`. `register`, `deregister` and
  config operations are rejected with a warning — they require authenticated REST.
- The message timestamp is ignored in favour of the server clock, so a skewed or
  hostile publisher cannot park `last_heartbeat` in the future and evade the sweep.
- Unknown or deregistered instance ids are a no-op, counted and summarised once
  per minute rather than logged per message.
- Multiple heartbeat lines may share one message — that is the supported way for
  an edge aggregator to batch.
- Writes are coalesced: decoder workers feed a single flusher that applies at
  most one transaction per second (or every 512 distinct instances).
- Subscribed with the queue group `cc-backend-fleet`, so several cc-backend
  instances share the stream instead of each writing the same row.

#### Fleet Discovery Rosters

Published by cc-backend, not consumed:

```
fleetdiscovery,cluster=fritz,type=ccmc event="[{\"type\":\"ccb\",\"hostname\":\"mgmt01\",\"state\":\"active\"}]" <ts>
```

- Subject: `<discovery-subject-prefix>.<cluster-or-"infra">.<consumer-service-type>`
- A consumer subscribes to exactly one subject and gets a ready-to-use provider list
- Re-published every `discovery-interval`, and immediately (debounced by 2s) when
  a service registers or deregisters
- Rosters carry **no** `instance_id`, no configuration and no `config_revision`

### Implementation Notes

- NATS API mirrors REST API functionality but uses messaging
- Job start/stop events are processed asynchronously via configurable worker pools
- Duplicate job detection is handled (same as REST API)
- All validation rules from REST API apply
- Node state updates include health checks against the metric store (identical to REST handler): nodes are grouped by subcluster, metric configurations are fetched, and `HealthCheck()` is called per subcluster. Nodes default to `MonitoringStateFailed` if no health data is available.
- Messages are logged; no responses are sent back to publishers
- If NATS client is unavailable, API subscriptions are skipped (logged as warning)

### Security Considerations

**The NATS API has no application-layer authentication or authorization.** Unlike
the REST endpoints (which require a JWT with `RoleAPI`), the subscribers process
any message delivered on the configured subjects. Anyone with publish rights to
those subjects on the broker can:

- Insert arbitrary jobs (potentially attributed to other users)
- Mark running jobs as stopped, triggering archive/finalization
- Overwrite node state and health metadata for any cluster
- Keep a dead or spoofed service listed as `active` in the discovery rosters,
  if they have learned its `instance_id` (heartbeat subject)

Operators MUST restrict publish ACLs at the NATS broker (per-account or
per-subject permissions) so that only trusted producers — e.g. the scheduler
integration on a known host or service account — can publish to the configured
`subject-job-event`, `subject-node-state` and `main.fleet.heartbeat-subject`
subjects. A shared, unrestricted NATS broker is not a safe deployment topology
for this API. A startup warning is logged when these subscriptions are enabled.

For the fleet subject the blast radius is deliberately bounded: heartbeat is the
**only** function reachable over NATS. Registration, deregistration and
configuration require an authenticated REST call, so no NATS publisher can
create, resurrect or terminate a service identity. An `instance_id` is a bearer
credential — do not log or share it. Discovery rosters are broadcast
unauthenticated and contain hostnames, service types and registration metadata,
so never put secrets in a service's `meta_data`.

## Fleet Service Discovery & Configuration

Auxiliary cc-* services (`ccms`, `ccmc`, `ccb`, `cces`, `ccsa`, `ccnc`, `ccem`)
register with cc-backend, are discovered by each other over NATS, and pull their
configuration over REST. Enabled by the `main.fleet` config block.

### REST endpoints

All are machine-to-machine and mounted under `/api` (JWT with `RoleAPI`, plus the
`api-allowed-ips` allowlist when configured — every fleet member's IP has to be
listed there). Read views for the web UI are served by GraphQL, not from here.

| Endpoint | Success | Notes |
|---|---|---|
| `POST /api/fleet/register/cluster/` | 201 `{instanceId, configRevision}` | Body: `cluster`, `hostname`, `serviceType`, optional `metaData` |
| `POST /api/fleet/register/infra/` | 201 `{instanceId, configRevision}` | Same without `cluster` |
| `POST /api/fleet/heartbeat/{instanceID}` | 204 | For deployments without NATS; 404 for an unknown or deregistered id |
| `GET /api/fleet/config/{instanceID}` | 200 / 204 / 304 | Merged configuration |
| `DELETE /api/fleet/deregister/{instanceID}` | 204 | Idempotent, terminal |

Config pull contract:

- The body is the merged configuration object itself; the revision (a content
  hash) travels in `ETag` and `X-CC-Config-Revision`.
- A client that sends `If-None-Match` gets `304` when nothing changed, so the
  steady state is a header-only round trip.
- `204` means no configuration has been authored for this service — a normal
  state, not an error.
- The revision is acknowledged only when it differs from the stored one, so
  polling does not generate a write per member per interval.

### Configuration tree

Hand-edited JSON, deep-merged broad to specific, under `main.fleet.config-dir`:

```
defaults.json                        # all services
<type>/defaults.json                 # all instances of one service type
<type>/<cluster>/defaults.json       # cluster scope only
<type>/<cluster>/<hostname>.json     # cluster scope only
<type>/<hostname>.json               # infra scope
```

Objects merge recursively; scalars and arrays overwrite. The tree is re-scanned
every `config-reload-interval` and swapped in atomically, so pulls never read
from disk and a torn or malformed tree keeps the last good generation.

### Registration lifecycle

`pending` → `active` (first heartbeat) → `stale` (no heartbeat for
`stale-after`) → `deregistered` (terminal). Re-registering the same
cluster/hostname/serviceType issues a **new** `instance_id`, invalidating the
previous one while preserving the config revision. Only `active` services appear
in discovery rosters.

**Security**: REST is the only path that can create or resurrect a service
identity. See the NATS security section for what the heartbeat subject allows.

## Development Guidelines

### Performance

This application processes large volumes of HPC monitoring data (metrics, job
records, archives) at scale. All code changes must prioritize maximum throughput
and minimal latency. Avoid unnecessary allocations, prefer streaming over
buffering, and be mindful of lock contention. When in doubt, benchmark.

### Commit Message Convention

Commits must use conventional commit prefixes so goreleaser can generate the
changelog automatically. Only commits with these prefixes appear in releases:

| Prefix  | Changelog group        |
|---------|------------------------|
| `feat:` | New Features           |
| `fix:`  | Bug fixes              |
| `sec:`  | Security updates       |
| `docs:` | Documentation updates  |

Scoped variants are also recognised, e.g. `feat(api):`, `fix(deps):`.
Commits without one of these prefixes are excluded from the changelog.

### Change Impact Analysis

For any significant change, you MUST:

1. **Check all call paths**: Trace every caller of modified functions to ensure
   correctness is preserved throughout the call chain.
2. **Evaluate side effects**: Identify and verify all side effects — database
   writes, cache invalidations, channel sends, goroutine lifecycle changes, file
   I/O, and external API calls.
3. **Consider concurrency implications**: This codebase uses goroutines and
   channels extensively. Verify that changes do not introduce races, deadlocks,
   or contention bottlenecks.

## Dependencies

- Go 1.25.0+ (check go.mod for exact version)
- Node.js (for frontend builds)
- SQLite 3 (only supported database)
- Optional: NATS server for NATS API integration
