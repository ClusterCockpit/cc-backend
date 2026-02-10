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
- **internal/api/nats.go**: NATS-based API for job and node operations
  - Subscribes to NATS subjects for job events (start/stop)
  - Handles node state updates via NATS
  - Uses InfluxDB line protocol message format
- **pkg/archive**: Job archive backend implementations
  - File system backend (default)
  - S3 backend
  - SQLite backend (experimental)
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
  - `main.apiSubjects`: NATS subject configuration (optional)
    - `subjectJobEvent`: Subject for job start/stop events (e.g., "cc.job.event")
    - `subjectNodeState`: Subject for node state updates (e.g., "cc.node.state")
  - `nats`: NATS client connection configuration (optional)
    - `address`: NATS server address (e.g., "nats://localhost:4222")
    - `username`: Authentication username (optional)
    - `password`: Authentication password (optional)
    - `creds-file-path`: Path to NATS credentials file (optional)
- **.env**: Environment variables (secrets like JWT keys)
  - Copy from `configs/env-template.txt`
  - NEVER commit this file
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
       "apiSubjects": {
         "subjectJobEvent": "cc.job.event",
         "subjectNodeState": "cc.node.state"
       }
     }
   }
   ```

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

### Implementation Notes

- NATS API mirrors REST API functionality but uses messaging
- Job start/stop events are processed asynchronously
- Duplicate job detection is handled (same as REST API)
- All validation rules from REST API apply
- Messages are logged; no responses are sent back to publishers
- If NATS client is unavailable, API subscriptions are skipped (logged as warning)

## Dependencies

- Go 1.24.0+ (check go.mod for exact version)
- Node.js (for frontend builds)
- SQLite 3 (only supported database)
- Optional: NATS server for NATS API integration
