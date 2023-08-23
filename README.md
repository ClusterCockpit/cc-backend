# ClusterCockpit REST and GraphQL API backend

[![Build](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml)

This is a Golang backend implementation for a REST and GraphQL API according to
the [ClusterCockpit specifications](https://github.com/ClusterCockpit/cc-specifications). It also
includes a web interface for ClusterCockpit. This implementation replaces the
previous PHP Symfony based ClusterCockpit web interface. The reasons for
switching from PHP Symfony to a Golang based solution are explained
[here](https://github.com/ClusterCockpit/ClusterCockpit/wiki/Why-we-switched-from-PHP-Symfony-to-a-Golang-based-solution).

## Overview


This is a Golang web backend for the ClusterCockpit job-specific performance monitoring framework.
It provides a REST API for integrating ClusterCockpit with an HPC cluster batch system and external analysis scripts.
Data exchange between the web front-end and the back-end is based on a GraphQL API.
The web frontend is also served by the backend using [Svelte](https://svelte.dev/) components.
Layout and styling are based on [Bootstrap 5](https://getbootstrap.com/) using [Bootstrap Icons](https://icons.getbootstrap.com/).

The backend uses [SQLite 3](https://sqlite.org/) as a relational SQL database by default.
Optionally it can use a MySQL/MariaDB database server.
While there are metric data  backends for the InfluxDB and Prometheus time series databases, the only tested and supported setup is to use cc-metric-store as the metric data backend.
Documentation on how to integrate ClusterCockpit with other time series databases will be added in the future. 

Completed batch jobs are stored in a file-based job archive according to
[this specification] (https://github.com/ClusterCockpit/cc-specifications/tree/master/job-archive).
The backend supports authentication via local accounts, an external LDAP
directory, and JWT tokens. Authorization for APIs is implemented with
[JWT](https://jwt.io/) tokens created with public/private key encryption.

You find more detailed information here:
* `./configs/README.md`: Infos about configuration and setup of cc-backend.
* `./init/README.md`: Infos on how to setup cc-backend as systemd service on Linux.
* `./tools/README.md`: Infos on the JWT authorizatin token workflows in ClusterCockpit.
* `./docs`: You can find further documentation here. There is also a Hands-on tutorial that is recommended to get familiar with the ClusterCockpit setup.

**NOTE**

ClusterCockpit requires a current version of the golang toolchain and node.js.
You can check `go.mod` to see what is the current minimal golang version needed.
Homebrew and Archlinux usually have current golang versions. For other Linux
distros this often means that you have to install the golang compiler yourself.
Fortunately, this is easy with golang. Since much of the functionality is based
on the Go standard library, it is crucial for security and performance to use a
current version of golang. In addition, an old golang toolchain may limit the supported
versions of third-party packages.

## How to try ClusterCockpit with a demo setup.

We provide a shell script that downloads demo data and automatically starts the
cc-backend. You will need `wget`, `go`, `node`, `npm` in your path to
start the demo. The demo downloads 32MB of data (223MB on disk).

```sh
git clone https://github.com/ClusterCockpit/cc-backend.git
cd ./cc-backend
./startDemo.sh
```

You can also try the demo using the lates release binary.
Create a folder and put the release binary `cc-backend` into this folder.
Execute the following steps:
```
$ ./cc-backend -init
$ vim config.json (Add a second cluster entry and name the clusters alex and fritz)
$ wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-demo.tar
$ tar xf job-archive-demo.tar
$ ./cc-backend -init-db -add-user demo:admin:demo -loglevel info
$ ./cc-backend -server -dev -loglevel info
```

You can access the web interface at http://localhost:8080.
Credentials for login are `demo:demo`.
Please note that some views do not work without a metric backend (e.g., the
Analysis, Systems and Status views).

## Howto build and run

There is a Makefile to automate the build of cc-backend. The Makefile supports the following targets:
* `$ make`: Initialize `var` directory and build svelte frontend and backend binary. Note that there is no proper prerequesite handling. Any change of frontend source files will result in a complete rebuild.
* `$ make clean`: Clean go build cache and remove binary.
* `$ make test`: Run the tests that are also run in the GitHub workflow setup.

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

### Run as systemd daemon

To run this program as a daemon, cc-backend comes with a [example systemd setup](./init/README.md).

## Configuration and setup

cc-backend can be used as a local web interface for an existing job archive or
as a server for the ClusterCockpit monitoring framework.

Create your job archive according to [this specification] (https://github.com/ClusterCockpit/cc-specifications/tree/master/job-archive).
At least one cluster directory with a valid `cluster.json` file is required. If
you configure the job archive from scratch, you must also create the job
archive version file that contains the job archive version as an integer.
You can retrieve the currently supported version by running the following
command:
```
$ ./cc-backend -version
```
It is ok to have no jobs in the job archive.

### Configuration

A configuration file in JSON format must be specified with `-config` to override the default settings.
By default, a `config.json` file located in the current directory of the `cc-backend` process will be loaded even without the `-config` flag.
Documentation of all supported configuration and command line options can be found [here](./configs/README.md).

## Database initialization and migration

Each `cc-backend` version supports a specific database version.
At startup, the version of the sqlite database is checked and `cc-backend` terminates if the version does not match.
`cc-backend` supports the migration of the database schema to the required version with the command line option `-migrate-db`.
If the database file does not exist yet, it will be created and initialized with the command line option `-migrate-db`.
If you want to use a newer database version with an older version of cc-backend, you can downgrade a database with the external tool [migrate](https://github.com/golang-migrate/migrate).
In this case, you must specify the path to the migration files in a current source tree: `./internal/repository/migrations/`.

## Development and testing
When making changes to the REST or GraphQL API, the appropriate code generators must be used.
You must always rebuild `cc-backend` after updating the API files.

### Update GraphQL schema

This project uses [gqlgen](https://github.com/99designs/gqlgen) for the GraphQL API.
The schema can be found in `./api/schema.graphqls`.
After changing it, you need to run `go run github.com/99designs/gqlgen`, which will update `./internal/graph/model`.
If new resolvers are needed, they will be added to `./internal/graph/schema.resolvers.go`, where you will then need to implement them.
If you start `cc-backend` with the `-dev` flag, the GraphQL Playground UI is available at http://localhost:8080/playground.

### Update Swagger UI

This project integrates [swagger ui] (https://swagger.io/tools/swagger-ui/) to document and test its REST API.
The swagger documentation files can be found in `./api/`.
You can generate the swagger-ui configuration by running `go run github.com/swaggo/swag/cmd/swag init -d ./internal/api,./pkg/schema -g rest.go -o ./api `.
You need to move the created `./api/docs.go` to `./internal/api/docs.go`.
If you start cc-backend with the `-dev` flag, the Swagger interface is available
at http://localhost:8080/swagger/.
You must enter a JWT key for a user with the API role.

**NOTE**

The user who owns the JWT key must not be logged into the same browser (have a
running session), or the Swagger requests will not work. It is recommended to
create a separate user that has only the API role.

## Development and testing
In case the REST or GraphQL API is changed the according code generators have to be used.

## Project file structure

- [`api/`](https://github.com/ClusterCockpit/cc-backend/tree/master/api) contains the API schema files for the REST and GraphQL APIs. The REST API is documented in the OpenAPI 3.0 format in [./api/openapi.yaml](./api/openapi.yaml).
- [`cmd/cc-backend`](https://github.com/ClusterCockpit/cc-backend/tree/master/cmd/cc-backend) contains `main.go` for the main application.
- [`configs/`](https://github.com/ClusterCockpit/cc-backend/tree/master/configs) contains documentation about configuration and command line options and required environment variables. A sample configuration file is provided.
- [`docs/`](https://github.com/ClusterCockpit/cc-backend/tree/master/docs) contains more in-depth documentation.
- [`init/`](https://github.com/ClusterCockpit/cc-backend/tree/master/init) contains an example of setting up systemd for production use.
- [`internal/`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal) contains library source code that is not intended for use by others.
- [`pkg/`](https://github.com/ClusterCockpit/cc-backend/tree/master/pkg) contains Go packages that can be used by other projects.
- [`tools/`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools) Additional command line helper tools.
   - [`archive-manager`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/archive-manager) Commands for getting infos about and existing job archive.
   - [`archive-migration`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/archive-migration) Tool to migrate from previous to current job archive version.
   - [`convert-pem-pubkey`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/convert-pem-pubkey) Tool to convert external pubkey for use in `cc-backend`.
   - [`gen-keypair`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/gen-keypair) contains a small application to generate a compatible JWT keypair. You find documentation on how to use it [here](https://github.com/ClusterCockpit/cc-backend/blob/master/docs/JWT-Handling.md).
- [`web/`](https://github.com/ClusterCockpit/cc-backend/tree/master/web) Server-side templates and frontend-related files:
   - [`frontend`](https://github.com/ClusterCockpit/cc-backend/tree/master/web/frontend) Svelte components and static assets for the frontend UI
   - [`templates`](https://github.com/ClusterCockpit/cc-backend/tree/master/web/templates) Server-side Go templates
- [`gqlgen.yml`](https://github.com/ClusterCockpit/cc-backend/blob/master/gqlgen.yml) Configures the behaviour and generation of [gqlgen](https://github.com/99designs/gqlgen).
- [`startDemo.sh`](https://github.com/ClusterCockpit/cc-backend/blob/master/startDemo.sh) is a shell script that sets up demo data, and builds and starts `cc-backend`.

