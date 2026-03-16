# NOTE

While we do our best to keep the master branch in a usable state, there is no guarantee the master branch works.
Please do not use it for production!

Please have a look at the [Release
Notes](https://github.com/ClusterCockpit/cc-backend/blob/master/ReleaseNotes.md)
for breaking changes!

# ClusterCockpit REST and GraphQL API backend

[![Build](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml)

This is a Golang backend implementation for a REST and GraphQL API according to
the [ClusterCockpit
specifications](https://github.com/ClusterCockpit/cc-specifications). It also
includes a web interface for ClusterCockpit. This implementation replaces the
previous PHP Symfony based ClusterCockpit web interface. The reasons for
switching from PHP Symfony to a Golang based solution are explained
[here](https://github.com/ClusterCockpit/ClusterCockpit/wiki/Why-we-switched-from-PHP-Symfony-to-a-Golang-based-solution).

## Overview

This is a Golang web backend for the ClusterCockpit job-specific performance
monitoring framework. It provides a REST API and an optional NATS-based messaging
API for integrating ClusterCockpit with an HPC cluster batch system and external
analysis scripts. Data exchange between the web front-end and the back-end is
based on a GraphQL API. The web frontend is also served by the backend using
[Svelte](https://svelte.dev/) components. Layout and styling are based on
[Bootstrap 5](https://getbootstrap.com/) using
[Bootstrap Icons](https://icons.getbootstrap.com/).

The backend uses [SQLite 3](https://sqlite.org/) as the relational SQL database.
While there are metric data backends for the InfluxDB and Prometheus time series
databases, the only tested and supported setup is to use cc-metric-store as the
metric data backend. Documentation on how to integrate ClusterCockpit with other
time series databases will be added in the future.

For real-time integration with HPC systems, the backend can subscribe to
[NATS](https://nats.io/) subjects to receive job start/stop events and node
state updates, providing an alternative to REST API polling.

Completed batch jobs are stored in a file-based job archive according to
[this specification](https://github.com/ClusterCockpit/cc-specifications/tree/master/job-archive).
The backend supports authentication via local accounts, an external LDAP
directory, and JWT tokens. Authorization for APIs is implemented with
[JWT](https://jwt.io/) tokens created with public/private key encryption.

You find a detailed documentation on the [ClusterCockpit
Webpage](https://clustercockpit.org).

## Build requirements

ClusterCockpit requires a current version of the golang toolchain and node.js.
You can check `go.mod` to see what is the current minimal golang version needed.
Homebrew and Archlinux usually have current golang versions. For other Linux
distros this often means that you have to install the golang compiler yourself.
Fortunately, this is easy with golang. Since much of the functionality is based
on the Go standard library, it is crucial for security and performance to use a
current version of golang. In addition, an old golang toolchain may limit the supported
versions of third-party packages.

## How to try ClusterCockpit with a demo setup

We provide a shell script that downloads demo data and automatically starts the
cc-backend. You will need `wget`, `go`, `node`, `npm` in your path to
start the demo. The demo downloads 32MB of data (223MB on disk).

```sh
git clone https://github.com/ClusterCockpit/cc-backend.git
cd ./cc-backend
./startDemo.sh
```

You can also try the demo using the latest release binary.
Create a folder and put the release binary `cc-backend` into this folder.
Execute the following steps:

```shell
./cc-backend -init
vim config.json (Add a second cluster entry and name the clusters alex and fritz)
wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-demo.tar
tar xf job-archive-demo.tar
./cc-backend -init-db -add-user demo:admin:demo -loglevel info
./cc-backend -server -dev -loglevel info
```

You can access the web interface at [http://localhost:8080](http://localhost:8080).
Credentials for login are `demo:demo`.
Please note that some views do not work without a metric backend (e.g., the
Analysis, Systems and Status views).

## How to build and run

There is a Makefile to automate the build of cc-backend. The Makefile supports
the following targets:

- `make`: Initialize `var` directory and build svelte frontend and backend
  binary. Note that there is no proper prerequisite handling. Any change of
  frontend source files will result in a complete rebuild.
- `make clean`: Clean go build cache and remove binary.
- `make test`: Run the tests that are also run in the GitHub workflow setup.

A common workflow for setting up cc-backend from scratch is:

```sh
git clone https://github.com/ClusterCockpit/cc-backend.git

# Build binary
cd ./cc-backend/
make

# EDIT THE .env FILE BEFORE YOU DEPLOY (Change the secrets)!
# If authentication is disabled, it can be empty.
cp configs/env-template.txt  .env
vim .env

cp configs/config.json .
vim config.json

#Optional: Link an existing job archive:
ln -s <your-existing-job-archive> ./var/job-archive

# This will first initialize the job.db database by traversing all
# `meta.json` files in the job-archive and add a new user.
./cc-backend -init-db -add-user <your-username>:admin:<your-password>

# Start a HTTP server (HTTPS can be enabled in the configuration, the default port is 8080).
# The --dev flag enables GraphQL Playground (http://localhost:8080/playground) and Swagger UI (http://localhost:8080/swagger).
./cc-backend -server  -dev

# Show other options:
./cc-backend -help
```

## Database Configuration

cc-backend uses SQLite as its database. For large installations, SQLite memory
usage can be tuned via the optional `db-config` section in config.json under
`main`:

```json
{
  "main": {
    "db": "./var/job.db",
    "db-config": {
      "cache-size-mb": 2048,
      "soft-heap-limit-mb": 16384,
      "max-open-connections": 4,
      "max-idle-connections": 4,
      "max-idle-time-minutes": 10,
      "busy-timeout-ms": 60000
    }
  }
}
```

All fields are optional. If `db-config` is omitted entirely, built-in defaults
are used.

### Options

| Option                  | Default | Description                                                                                                                                                                             |
| ----------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cache-size-mb`         | 2048    | SQLite page cache size per connection in MB. Maps to `PRAGMA cache_size`. Total cache memory is up to `cache-size-mb × max-open-connections`.                                           |
| `soft-heap-limit-mb`    | 16384   | Process-wide SQLite soft heap limit in MB. SQLite will try to release cache pages to stay under this limit. Queries won't fail if exceeded, but cache eviction becomes more aggressive. |
| `max-open-connections`  | 4       | Maximum number of open database connections.                                                                                                                                            |
| `max-idle-connections`  | 4       | Maximum number of idle database connections kept in the pool.                                                                                                                           |
| `max-idle-time-minutes` | 10      | Maximum time in minutes a connection can sit idle before being closed.                                                                                                                  |
| `busy-timeout-ms`       | 60000   | SQLite busy timeout in milliseconds. When a write is blocked by another writer, SQLite retries internally with backoff for up to this duration before returning `SQLITE_BUSY`.          |

### Sizing Guidelines

SQLite's `cache_size` is a **per-connection** setting — each connection
maintains its own independent page cache. With multiple connections, the total
memory available for caching is the sum across all connections.

In practice, different connections tend to cache **different pages** (e.g., one
handles a job listing query while another runs a statistics aggregation), so
their caches naturally spread across the database. The formula
`DB_size / max-open-connections` gives enough per-connection cache that the
combined caches can cover the entire database.

However, this is a best-case estimate. Connections running similar queries will
cache the same pages redundantly. In the worst case (all connections caching
identical pages), only `cache-size-mb` worth of unique data is cached rather
than `cache-size-mb × max-open-connections`. For workloads with diverse
concurrent queries, cache overlap is typically low.

**Rules of thumb:**

- **cache-size-mb**: Set to `DB_size_in_MB / max-open-connections` to allow the
  entire database to be cached in memory. For example, an 80GB database with 8
  connections needs at least 10240 MB (10GB) per connection. If your workload
  has many similar concurrent queries, consider setting it higher to account for
  cache overlap between connections.

- **soft-heap-limit-mb**: Should be >= `cache-size-mb × max-open-connections` to
  avoid cache thrashing. This is the total SQLite memory budget for the process.
- On small installations the defaults work well. On servers with large databases
  (tens of GB) and plenty of RAM, increasing these values significantly improves
  query performance by reducing disk I/O.

### Example: Large Server (512GB RAM, 80GB database)

```json
{
  "main": {
    "db-config": {
      "cache-size-mb": 16384,
      "soft-heap-limit-mb": 131072,
      "max-open-connections": 8,
      "max-idle-time-minutes": 30
    }
  }
}
```

This allows the entire 80GB database to be cached (8 × 16GB = 128GB page cache)
with a 128GB soft heap limit, using about 25% of available RAM.

The effective configuration is logged at startup for verification.

## Project file structure

- [`.github/`](https://github.com/ClusterCockpit/cc-backend/tree/master/.github)
  GitHub Actions workflows and dependabot configuration for CI/CD.
- [`api/`](https://github.com/ClusterCockpit/cc-backend/tree/master/api)
  contains the API schema files for the REST and GraphQL APIs. The REST API is
  documented in the OpenAPI 3.0 format in
  [./api/swagger.yaml](./api/swagger.yaml). The GraphQL schema is in
  [./api/schema.graphqls](./api/schema.graphqls).
- [`cmd/cc-backend`](https://github.com/ClusterCockpit/cc-backend/tree/master/cmd/cc-backend)
  contains the main application entry point and CLI implementation.
- [`configs/`](https://github.com/ClusterCockpit/cc-backend/tree/master/configs)
  contains documentation about configuration and command line options and required
  environment variables. Sample configuration files are provided.
- [`init/`](https://github.com/ClusterCockpit/cc-backend/tree/master/init)
  contains an example of setting up systemd for production use.
- [`internal/`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal)
  contains library source code that is not intended for use by others.
  - [`api`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/api)
    REST API handlers and NATS integration
  - [`archiver`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/archiver)
    Job archiving functionality
  - [`auth`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/auth)
    Authentication (local, LDAP, OIDC) and JWT token handling
  - [`config`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/config)
    Configuration management and validation
  - [`graph`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/graph)
    GraphQL schema and resolvers
  - [`importer`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/importer)
    Job data import and database initialization
  - [`metricdispatch`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/metricdispatch)
    Dispatches metric data loading to appropriate backends
  - [`repository`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/repository)
    Database repository layer for jobs and metadata
  - [`routerConfig`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/routerConfig)
    HTTP router configuration and middleware
  - [`tagger`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/tagger)
    Job classification and application detection
  - [`taskmanager`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/taskmanager)
    Background task management and scheduled jobs
  - [`metricstoreclient`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/metricstoreclient)
    Client for cc-metric-store queries
- [`pkg/`](https://github.com/ClusterCockpit/cc-backend/tree/master/pkg)
  contains Go packages that can be used by other projects.
  - [`archive`](https://github.com/ClusterCockpit/cc-backend/tree/master/pkg/archive)
    Job archive backend implementations (filesystem, S3, SQLite)
  - [`metricstore`](https://github.com/ClusterCockpit/cc-backend/tree/master/pkg/metricstore)
    In-memory metric data store with checkpointing and metric loading
- [`tools/`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools)
  Additional command line helper tools.
  - [`archive-manager`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/archive-manager)
    Commands for getting infos about an existing job archive, importing jobs
    between archive backends, and converting archives between JSON and Parquet formats.
  - [`archive-migration`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/archive-migration)
    Tool for migrating job archives between formats.
  - [`convert-pem-pubkey`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/convert-pem-pubkey)
    Tool to convert external pubkey for use in `cc-backend`.
  - [`gen-keypair`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/gen-keypair)
    contains a small application to generate a compatible JWT keypair. You find
    documentation on how to use it
    [here](https://github.com/ClusterCockpit/cc-backend/blob/master/docs/JWT-Handling.md).
- [`web/`](https://github.com/ClusterCockpit/cc-backend/tree/master/web)
  Server-side templates and frontend-related files:
  - [`frontend`](https://github.com/ClusterCockpit/cc-backend/tree/master/web/frontend)
    Svelte components and static assets for the frontend UI
  - [`templates`](https://github.com/ClusterCockpit/cc-backend/tree/master/web/templates)
    Server-side Go templates, including monitoring views
- [`gqlgen.yml`](https://github.com/ClusterCockpit/cc-backend/blob/master/gqlgen.yml)
  Configures the behaviour and generation of
  [gqlgen](https://github.com/99designs/gqlgen).
- [`startDemo.sh`](https://github.com/ClusterCockpit/cc-backend/blob/master/startDemo.sh)
  is a shell script that sets up demo data, and builds and starts `cc-backend`.
