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
go build -ldflags='-s -X main.date=$(date +"%Y-%m-%d:T%H:%M:%S") -X main.version=1.4.4 -X main.commit=$(git rev-parse --short HEAD)' ./cmd/cc-backend
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
# Regenerate GraphQL schema and resolvers (after modifying api/*.graphqls)
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
  - Schema in `api/*.graphqls`
  - Generated code in `internal/graph/generated/`
  - Resolvers in `internal/graph/schema.resolvers.go`
- **internal/auth**: Authentication layer
  - Supports local accounts, LDAP, OIDC, and JWT tokens
  - Implements rate limiting for login attempts
- **internal/metricdata**: Metric data repository abstraction
  - Pluggable backends: cc-metric-store, Prometheus, InfluxDB
  - Each cluster can have a different metric data backend
- **internal/archiver**: Job archiving to file-based archive
- **pkg/archive**: Job archive backend implementations
  - File system backend (default)
  - S3 backend
  - SQLite backend (experimental)
- **pkg/nats**: NATS integration for metric ingestion

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

**Database Migrations**: SQL migrations in `internal/repository/migrations/` are
applied automatically on startup. Version tracking in `version` table.

**Scopes**: Metrics can be collected at different scopes:

- Node scope (always available)
- Core scope (for jobs with ≤8 nodes)
- Accelerator scope (for GPU/accelerator metrics)

## Configuration

- **config.json**: Main configuration (clusters, metric repositories, archive settings)
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

- Schema: `api/*.graphqls`
- Config: `gqlgen.yml`
- Generated code: `internal/graph/generated/`
- Custom resolvers: `internal/graph/schema.resolvers.go`
- Run `make graphql` after schema changes

**Swagger/OpenAPI**:

- Annotations in `internal/api/*.go`
- Generated docs: `api/docs.go`, `api/swagger.yaml`
- Run `make swagger` after API changes

## Testing Conventions

- Test files use `_test.go` suffix
- Test data in `testdata/` subdirectories
- Repository tests use in-memory SQLite
- API tests use httptest

## Common Workflows

### Adding a new GraphQL field

1. Edit schema in `api/*.graphqls`
2. Run `make graphql`
3. Implement resolver in `internal/graph/schema.resolvers.go`

### Adding a new REST endpoint

1. Add handler in `internal/api/*.go`
2. Add route in `internal/api/rest.go`
3. Add Swagger annotations
4. Run `make swagger`

### Adding a new metric data backend

1. Implement `MetricDataRepository` interface in `internal/metricdata/`
2. Register in `metricdata.Init()` switch statement
3. Update config.json schema documentation

### Modifying database schema

1. Create new migration in `internal/repository/migrations/`
2. Increment `repository.Version`
3. Test with fresh database and existing database

## Dependencies

- Go 1.24.0+ (check go.mod for exact version)
- Node.js (for frontend builds)
- SQLite 3 (only supported database)
- Optional: NATS server for metric ingestion
