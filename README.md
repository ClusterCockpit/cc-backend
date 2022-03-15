# ClusterCockpit REST and GraphQL API backend

[![Build](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml)

This is a Golang backend implementation for a REST and GraphQL API according to the [ClusterCockpit specifications](https://github.com/ClusterCockpit/cc-specifications).
It also includes a web interface for ClusterCockpit based on the components implemented in
[cc-frontend](https://github.com/ClusterCockpit/cc-frontend), which is included as a git submodule.
This implementation replaces the previous PHP Symfony based ClusterCockpit web-interface.

## Overview

This is a golang web backend for the ClusterCockpit job-specific performance monitoring framework.
It provides a REST API for integrating ClusterCockpit with a HPC cluster batch system and external analysis scripts.
Data exchange between the web frontend and backend is based on a GraphQL API.
The web frontend is also served by the backend using [Svelte](https://svelte.dev/) components implemented in [cc-frontend](https://github.com/ClusterCockpit/cc-frontend).
Layout and styling is based on [Bootstrap 5](https://getbootstrap.com/) using [Bootstrap Icons](https://icons.getbootstrap.com/).
The backend uses [SQLite 3](https://sqlite.org/) as relational SQL database by default. It can optionally use a MySQL/MariaDB database server.
Finished batch jobs are stored in a so called job archive following [this specification](https://github.com/ClusterCockpit/cc-backend/wiki).
The backend supports authentication using local accounts or an external LDAP directory.
Authorization for APIs is implemented using [JWT](https://jwt.io/) tokens created with  public/private key encryption.

## Howto Build and Run

```sh
# The frontend is a submodule, so use `--recursive`
git clone --recursive git@github.com:ClusterCockpit/cc-backend.git

# Prepare frontend
cd ./cc-backend/frontend
yarn install
yarn build

cd ..
go get
go build

# The job-archive directory must be organised the same way as
# as for the regular ClusterCockpit.
ln -s <your-existing-job-archive> ./var/job-archive

# Create empty job.db (Will be initialized as SQLite3 database)
touch ./var/job.db

# EDIT THE .env FILE BEFORE YOU DEPLOY (Change the secrets)!
# If authentication is disabled, it can be empty.
vim ./.env

# This will first initialize the job.db database by traversing all
# `meta.json` files in the job-archive and add a new user. `--no-server` will cause the
# executable to stop once it has done that instead of starting a server.
./cc-backend --init-db --add-user <your-username>:admin:<your-password> --no-server

# Start a HTTP server (HTTPS can be enabled, the default port is 8080):
./cc-backend

# Show other options:
./cc-backend --help
```
### Run as systemd daemon

In order to run this program as a daemon, look at [utils/systemd/README.md](./utils/systemd/README.md) where a systemd unit file and more explanation is provided.

## Configuration and Setup

cc-backend can be used as a local web-interface for an existing job archive or
as a general web-interface server for a live ClusterCockpit Monitoring
framework.

Create your job-archive according to [this specification](https://github.com/ClusterCockpit/cc-specifications). At least
one cluster with a valid `cluster.json` file is required. Having no jobs in the
job-archive at all is fine. You may use the sample job-archive available for
download [in cc-docker/develop](https://github.com/ClusterCockpit/cc-docker/tree/develop).

### Configuration

A config file in the JSON format can be provided using `--config` to override the defaults.
Look at the beginning of `server.go` for the defaults and consequently the format of the configuration file.

### Update GraphQL schema

This project uses [gqlgen](https://github.com/99designs/gqlgen) for the GraphQL
API. The schema can be found in `./graph/schema.graphqls`. After changing it,
you need to run `go run github.com/99designs/gqlgen` which will update
`graph/model`. In case new resolvers are needed, they will be inserted into
`graph/schema.resolvers.go`, where you will need to implement them.

## Project Structure

- `api/` contains the REST API. The routes defined there should be called whenever a job starts/stops. The API is documented in the OpenAPI 3.0 format in [./api/openapi.yaml](./api/openapi.yaml).
- `auth/` is where the (optional) authentication middleware can be found, which adds the currently authenticated user to the request context. The `user` table is created and managed here as well.
  - `auth/ldap.go` contains everything to do with automatically syncing and authenticating users form an LDAP server.
- `config` handles the `cluster.json` files and the user-specific configurations (changeable via GraphQL) for the Web-UI such as the selected metrics etc.
- `frontend` is a submodule, this is where the Svelte based frontend resides.
- `graph/generated` should *not* be touched.
- `graph/model` contains all types defined in the GraphQL schema not manually defined in `schema/`. Manually defined types have to be listed in `gqlgen.yml`.
- `graph/schema.graphqls` contains the GraphQL schema. Whenever you change it, you should call `go run github.com/99designs/gqlgen`.
- `graph/` contains the resolvers and handlers for the GraphQL API. Function signatures in `graph/schema.resolvers.go` are automatically generated.
- `metricdata/` handles getting and archiving the metrics associated with a job.
  - `metricdata/metricdata.go` defines the interface `MetricDataRepository` and provides functions to the GraphQL and REST API for accessing a jobs metrics which automatically take care of selecting the source for the metrics (the archive or one of the metric data repositories).
  - `metricdata/archive.go` provides functions for fetching metrics from the job-archive and archiving a job to the job-archive.
  - `metricdata/cc-metric-store.go` contains an implementation of the `MetricDataRepository` interface which can fetch data from an [cc-metric-store](https://github.com/ClusterCockpit/cc-metric-store)
  - `metricdata/influxdb-v2` contains an implementation of the `MetricDataRepository` interface which can fetch data from an InfluxDBv2 database. It is currently disabled and out of date and can not be used as of writing.
- `repository/` all SQL related stuff.
- `repository/init.go` initializes the `job` (and `tag` and `jobtag`) table if the `--init-db` flag is provided. Not only is the table created in the correct schema, but the job-archive is traversed as well.
- `schema/` contains type definitions used all over this project extracted in this package as Go disallows cyclic dependencies between packages.
  - `schema/float.go` contains a custom `float64` type which overwrites JSON and GraphQL Marshaling/Unmarshalling. This is needed because a regular optional `Float` in GraphQL will map to `*float64` types in Go. Wrapping every single metric value in an allocation would be a lot of overhead.
  - `schema/job.go` provides the types representing a job and its resources. Those can be used as type for a `meta.json` file and/or a row in the `job` table.
- `templates/` is mostly full of HTML templates and a small helper go module.
- `utils/systemd` describes how to deploy/install this as a systemd service
- `test/` rudimentery tests.
- `utils/`
- `.env` *must* be changed before you deploy this. It contains a Base64 encoded [Ed25519](https://en.wikipedia.org/wiki/EdDSA) key-pair, the secret used for sessions and the password to the LDAP server if LDAP authentication is enabled.
- `gqlgen.yml` configures the behaviour and generation of [gqlgen](https://github.com/99designs/gqlgen).
- `server.go` contains the main function and starts the actual http server.
