# ClusterCockpit REST and GraphQL API backend

[![Build](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml)

This is a Golang backend implementation for a REST and GraphQL API according to the [ClusterCockpit specifications](https://github.com/ClusterCockpit/cc-specifications).
It also includes a web interface for ClusterCockpit.
While there is a backend for the InfluxDB timeseries database, the only tested and supported setup is using cc-metric-store as a mtric data backend.
We will add documentation how to integrate ClusterCockpit with other timeseries databases in the future.
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
* `./docs`: You can find further documentation here. There is also a Hands-on tutorial that is recommended to get familiar with the ClusterCockpit setup.

**NOTICE**

ClusterCockpit requires a recent version of the golang toolchain and node.js.
You can check in `go.mod` what is the current minimal golang version required.
Homebrew and Archlinux usually have up to date golang versions. For other Linux
distros this often means you have to install the golang compiler yourself.
Fortunatly this is easy with golang. Since a lot of functionality is based on
the go standard library it is crucial for security and performance to use a
recent golang version. Also an old golang tool chain may restrict the supported
versions of third party packages.

## Demo Setup

We provide a shell skript that downloads demo data and automatically builds and starts cc-backend.
You need `wget`, `go`, `node`, `rollup` and `yarn` in your path to start the demo. The demo will download 32MB of data (223MB on disk).

```sh
git clone https://github.com/ClusterCockpit/cc-backend.git
cd ./cc-backend
./startDemo.sh
```
You can access the web interface at http://localhost:8080.
Credentials for login: `demo:AdminDev`.
Please note that some views do not work without a metric backend (e.g., the Systems and Status view).

## Howto Build and Run

There is a Makefile to automate the build of cc-backend. The Makefile supports the following targets:
* `$ make`: Initialize `var` directory and build svelte frontend and backend binary. Please note that there is no proper prerequesite handling. Any change of frontend source files will trigger a complete rebuild.
* `$ make clean`: Clean go build cache and remove binary
* `$ make test`: Run the tests that are also run in the GitHub workflow setup.

A common workflow to setup cc-backend fron scratch is:
```sh
git clone https://github.com/ClusterCockpit/cc-backend.git

# Build binary
cd ./cc-backend/
make

# EDIT THE .env FILE BEFORE YOU DEPLOY (Change the secrets)!
# If authentication is disabled, it can be empty.
cp configs/env-template.txt  .env
vim ./.env

cp configs/config.json ./
vim ./config.json

#Optional: Link an existing job archive:
ln -s <your-existing-job-archive> ./var/job-archive

# This will first initialize the job.db database by traversing all
# `meta.json` files in the job-archive and add a new user. `--no-server` will cause the
# executable to stop once it has done that instead of starting a server.
./cc-backend --init-db --add-user <your-username>:admin:<your-password>

# Start a HTTP server (HTTPS can be enabled, the default port is 8080).
# The --dev flag enables GraphQL Playground (http://localhost:8080/playground) and Swagger UI (http://localhost:8080/swagger).
./cc-backend --server  --dev

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

A config file in the JSON format has to be provided using `--config` to override the defaults.
By default, if there is a `config.json` file in the current directory of the `cc-backend` process, it will be loaded even without the `--config` flag.
You find documentation of all supported configuration and command line options [here](./configs/README.md).

## Database initialization and migration

Every cc-backend version supports a specific database version.
On startup the version of the sqlite database is validated and cc-backend will terminate if the version does not match.
cc-backend supports to migrate the database schema up to the required version using the `--migrate-db` command line option.
In case the database file does not yet exist it is created and initialized by the `--migrate-db` command line option.
In case you want to use a newer database version with an older version of cc-backend you can downgrade a database using the external [migrate](https://github.com/golang-migrate/migrate) tool.
In this case you have to provide the path to the migration files in a recent source tree: `./internal/repository/migrations/`.

## Development
In case the REST or GraphQL API is changed the according code generators have to be used.

### Update GraphQL schema

This project uses [gqlgen](https://github.com/99designs/gqlgen) for the GraphQL API.
The schema can be found in `./api/schema.graphqls`.
After changing it, you need to run `go run github.com/99designs/gqlgen` which will update `./internal/graph/model`.
In case new resolvers are needed, they will be inserted into `./internal/graph/schema.resolvers.go`, where you will need to implement them.
If you start cc-backend with flag `--dev` the GraphQL Playground UI is available at http://localhost:8080/playground .

### Update Swagger UI

This project integrates [swagger ui](https://swagger.io/tools/swagger-ui/) to document and test its REST API.
The swagger doc files can be found in `./api/`.
You can generate the configuration of swagger-ui by running `go run github.com/swaggo/swag/cmd/swag init -d ./internal/api,./pkg/schema  -g rest.go -o ./api `.
You need to move the generated `./api/doc.go` to `./internal/api/doc.go`.
If you start cc-backend with flag `--dev` the Swagger UI is available at http://localhost:8080/swagger/ .
You have to enter a JWT key for a user with role API.

**NOTICE** The user owning the JWT token must not be logged in the same browser (have a running session), otherwise Swagger requests will not work. It is recommended to create a separate user that has just the API role.

## Project Structure

- `api/` contains the API schema files for the REST and GraphQL APIs. The REST API is documented in the OpenAPI 3.0 format in [./api/openapi.yaml](./api/openapi.yaml).
- `cmd/cc-backend` contains `main.go` for the main application.
- `cmd/gen-keypair` contains is a small application to generate a compatible JWT keypair includin a README about JWT setup in ClusterCockpit.
- `configs/` contains documentation about configuration and command line options and required environment variables. An example configuration file is provided.
- `init/` contains an example systemd setup for production use.
- `internal/` contains library source code that is not intended to be used by others.
- `pkg/` contains go packages that can also be used by other projects.
- `test/` Test apps and test data.
- `web/` Server side templates and frontend related files:
   - `templates` Serverside go templates
   - `frontend` Svelte components and static assets for frontend UI
- `gqlgen.yml` configures the behaviour and generation of [gqlgen](https://github.com/99designs/gqlgen).
- `startDemo.sh` is a shell script that sets up demo data, and builds and starts cc-backend.
