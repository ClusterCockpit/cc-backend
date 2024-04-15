# `cc-backend` version 1.3.0

Supports job archive version 1 and database version 7.

This is a minor release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Breaking changes

* This release fixes bugs in the MySQL/MariaDB database schema. For this reason
  you have to migrate your database using the `-migrate-db` switch.
