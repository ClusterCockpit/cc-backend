# NOTE

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
monitoring framework. It provides a REST API for integrating ClusterCockpit with
an HPC cluster batch system and external analysis scripts. Data exchange between
the web front-end and the back-end is based on a GraphQL API. The web frontend
is also served by the backend using [Svelte](https://svelte.dev/) components.
Layout and styling are based on [Bootstrap 5](https://getbootstrap.com/) using
[Bootstrap Icons](https://icons.getbootstrap.com/).

The backend uses [SQLite 3](https://sqlite.org/) as a relational SQL database by
default. Optionally it can use a MySQL/MariaDB database server. While there are
metric data  backends for the InfluxDB and Prometheus time series databases, the
only tested and supported setup is to use cc-metric-store as the metric data
backend. Documentation on how to integrate ClusterCockpit with other time series
databases will be added in the future.

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

``` shell
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

* `make`: Initialize `var` directory and build svelte frontend and backend
binary. Note that there is no proper prerequisite handling. Any change of
frontend source files will result in a complete rebuild.
* `make clean`: Clean go build cache and remove binary.
* `make test`: Run the tests that are also run in the GitHub workflow setup.

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

## Project file structure

* [`api/`](https://github.com/ClusterCockpit/cc-backend/tree/master/api)
contains the API schema files for the REST and GraphQL APIs. The REST API is
documented in the OpenAPI 3.0 format in
[./api/openapi.yaml](./api/openapi.yaml).
* [`cmd/cc-backend`](https://github.com/ClusterCockpit/cc-backend/tree/master/cmd/cc-backend)
contains `main.go` for the main application.
* [`configs/`](https://github.com/ClusterCockpit/cc-backend/tree/master/configs)
contains documentation about configuration and command line options and required
environment variables. A sample configuration file is provided.
* [`docs/`](https://github.com/ClusterCockpit/cc-backend/tree/master/docs)
contains more in-depth documentation.
* [`init/`](https://github.com/ClusterCockpit/cc-backend/tree/master/init)
contains an example of setting up systemd for production use.
* [`internal/`](https://github.com/ClusterCockpit/cc-backend/tree/master/internal)
contains library source code that is not intended for use by others.
* [`pkg/`](https://github.com/ClusterCockpit/cc-backend/tree/master/pkg)
contains Go packages that can be used by other projects.
* [`tools/`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools)
Additional command line helper tools.
  * [`archive-manager`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/archive-manager)
  Commands for getting infos about and existing job archive.
  * [`convert-pem-pubkey`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/convert-pem-pubkey)
  Tool to convert external pubkey for use in `cc-backend`.
  * [`gen-keypair`](https://github.com/ClusterCockpit/cc-backend/tree/master/tools/gen-keypair)
  contains a small application to generate a compatible JWT keypair. You find
  documentation on how to use it
  [here](https://github.com/ClusterCockpit/cc-backend/blob/master/docs/JWT-Handling.md).
* [`web/`](https://github.com/ClusterCockpit/cc-backend/tree/master/web)
Server-side templates and frontend-related files:
  * [`frontend`](https://github.com/ClusterCockpit/cc-backend/tree/master/web/frontend)
  Svelte components and static assets for the frontend UI
  * [`templates`](https://github.com/ClusterCockpit/cc-backend/tree/master/web/templates)
  Server-side Go templates
* [`gqlgen.yml`](https://github.com/ClusterCockpit/cc-backend/blob/master/gqlgen.yml)
Configures the behaviour and generation of
[gqlgen](https://github.com/99designs/gqlgen).
* [`startDemo.sh`](https://github.com/ClusterCockpit/cc-backend/blob/master/startDemo.sh)
is a shell script that sets up demo data, and builds and starts `cc-backend`.
