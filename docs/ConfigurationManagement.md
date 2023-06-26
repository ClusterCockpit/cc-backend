# Release versions

Versions are marked according to [semantic versioning] (https://semver.org).
Each version embeds the following static assets in the binary:
* Web frontend with javascript files and all static assets.
* Golang template files for server-side rendering.
* JSON schema files for validation.
* Database migration files.

The remaining external assets are:
* The SQL database used.
* The job archive
* The configuration files `config.json` and `.env`.

The external assets are versioned with integer IDs.
This means that each release binary is bound to specific versions of the SQL
database and the job archive.
The configuration file is checked against the current schema at startup.
The `-migrate-db` command line switch can be used to upgrade the SQL database
to migrate from a previous version to the latest one.
We offer a separate tool `archive-migration` to migrate an existing job archive
archive from the previous to the latest version.

# Versioning of APIs

cc-backend provides two API backends:
* A REST API for querying jobs.
* A GraphQL API for data exchange between web frontend and cc-backend.

The REST API will also be versioned. We still have to decide whether we will also
support older REST API versions by versioning the endpoint URLs.
The GraphQL API is for internal use and will not be versioned.

# How to build

In general it is recommended to use the provided release binary.
In case you want to build build `cc-backend` please always use the provided makefile. This will ensure
that the frontend is also built correctly and that the version in the binary is encoded in the binary.
