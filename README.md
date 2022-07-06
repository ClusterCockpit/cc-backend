# ClusterCockpit REST and GraphQL API backend

[![Build](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml)

This is a Golang backend implementation for a REST and GraphQL API according to the [ClusterCockpit specifications](https://github.com/ClusterCockpit/cc-specifications).
It also includes a web interface for ClusterCockpit.
This implementation replaces the previous PHP Symfony based ClusterCockpit web-interface.
[Here](https://github.com/ClusterCockpit/ClusterCockpit/wiki/Why-we-switched-from-PHP-Symfony-to-a-Golang-based-solution) is a discussion of the reasons why we switched from PHP Symfony to a Golang based solution.

## Overview

This is a golang web backend for the ClusterCockpit job-specific performance monitoring framework.
It provides a REST API for integrating ClusterCockpit with a HPC cluster batch system and external analysis scripts.
Data exchange between the web frontend and backend is based on a GraphQL API.
The web frontend is also served by the backend using [Svelte](https://svelte.dev/) components.
Layout and styling is based on [Bootstrap 5](https://getbootstrap.com/) using [Bootstrap Icons](https://icons.getbootstrap.com/).
The backend uses [SQLite 3](https://sqlite.org/) as relational SQL database by default.
It can optionally use a MySQL/MariaDB database server.
Finished batch jobs are stored in a file-based job archive following [this specification](https://github.com/ClusterCockpit/cc-specifications/tree/master/job-archive).
The backend supports authentication using local accounts or an external LDAP directory.
Authorization for APIs is implemented using [JWT](https://jwt.io/) tokens created with public/private key encryption.

You find more detailed information here:
* `./configs/README.md`: Infos about configuration and setup of cc-backend.
* `./init/README.md`: Infos on how to setup cc-backend as systemd service on Linux.
* `./tools/README.md`: Infos on the JWT authorizatin token workflows in ClusterCockpit.

## Demo Setup

We provide a shell skript that downloads demo data and automatically builds and starts cc-backend.
You need `wget`, `go`, and `yarn` in your path to start the demo. The demo will download 32MB of data (223MB on disk).

```sh
git clone git@github.com:ClusterCockpit/cc-backend.git

./startDemo.sh
```
You can access the web interface at http://localhost:8080.
Credentials for login: `demo:AdminDev`.
Please note that some views do not work without a metric backend (e.g., the Systems and Status view).

## Howto Build and Run

```sh
git clone git@github.com:ClusterCockpit/cc-backend.git

# Prepare frontend
cd ./cc-backend/web/frontend
yarn install
yarn build

cd ..
go build ./cmd/cc-backend

ln -s <your-existing-job-archive> ./var/job-archive

# Create empty job.db (Will be initialized as SQLite3 database)
touch ./var/job.db

# EDIT THE .env FILE BEFORE YOU DEPLOY (Change the secrets)!
# If authentication is disabled, it can be empty.
cp configs/env-template.txt  .env
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

In order to run this program as a daemon, cc-backend ships with an [example systemd setup](./init/README.md).

## Configuration and Setup

cc-backend can be used as a local web-interface for an existing job archive or as a general web-interface server for a live ClusterCockpit Monitoring framework.

Create your job-archive according to [this specification](https://github.com/ClusterCockpit/cc-specifications/tree/master/job-archive).
At least one cluster with a valid `cluster.json` file is required.
Having no jobs in the job-archive at all is fine.

### Configuration

A config file in the JSON format can be provided using `--config` to override the defaults.
You find documentation of all supported configuration and command line options [here](./configs.README.md).

### Update GraphQL schema

This project uses [gqlgen](https://github.com/99designs/gqlgen) for the GraphQL API.
The schema can be found in `./api/schema.graphqls`.
After changing it, you need to run `go run github.com/99designs/gqlgen` which will update `./internal/graph/model`.
In case new resolvers are needed, they will be inserted into `./internal/graph/schema.resolvers.go`, where you will need to implement them.

## Project Structure

- `api/` contains the API schema files for the REST and GraphQL APIs. The REST API is documented in the OpenAPI 3.0 format in [./api/openapi.yaml](./api/openapi.yaml).
- `cmd/cc-backend` contains `main.go` for the main application.
- `configs/` contains documentation about configuration and command line options and required environment variables. An example configuration file is provided.
- `init/` contains an example systemd setup for production use.
- `internal/` contains library source code that is not intended to be used by others.
- `pkg/` contains go packages that can also be used by other projects.
- `test/` Test apps and test data.
- `tools/` contains supporting tools for cc-backend. At the moment this is a small application to generate a compatible JWT keypair includin a README about JWT setup in ClusterCockpit.
- `web/` Server side templates and frontend related files:
   - `templates` Serverside go templates
   - `frontend` Svelte components and static assets for frontend UI
- `gqlgen.yml` configures the behaviour and generation of [gqlgen](https://github.com/99designs/gqlgen).
- `startDemo.sh` is a shell script that sets up demo data, and builds and starts cc-backend.
