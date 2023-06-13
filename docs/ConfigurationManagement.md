# Release versioning

Releases are numbered with an integer ID, starting with 1.
Each release embeds the following assets in the binary:
* Web front-end with Javascript files and all static assets.
* Golang template files for server-side rendering.
* JSON schema files for validation.
* Database migration files

The remaining external assets are:
* The SQL database used
* The job archive
* The configuration file `config.json`

Both external assets are also versioned with integer IDs.
This means that each release binary is bound to specific versions of the SQL
database and the job archive.
The configuration file is validated against the current schema on startup.
The command line switch `-migrate-db` can be used to upgrade the SQL database
to migrate from a previous version to the latest one.
We offer a separate tool `archive-migration` to migrate an existing job archive
archive from the previous to the latest version.

# Versioning of APIs

cc-backend provides two API backends:
* A REST API for querying jobs
* A GraphQL API for data exchange between web frontend and cc-backend

Both APIs will also be versioned. We still need to decide wether we will also support
older REST API version by versioning the endpoint URLs.

# How to build

Please always build `cc-backend` with the supplied Makefile. This will ensure
that the frontend is also built correctly and that the version in the binary file is coded
in the binary.
