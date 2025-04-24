# `cc-backend` version 1.4.4

Supports job archive version 2 and database version 8.

This is a bug fix release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Breaking changes for minor release 1.4.x

- You need to perform a database migration. Depending on your database size the
  migration might require several hours!
- You need to adapt the `cluster.json` configuration files in the job-archive,
  add new required attributes to the metric list and after that edit
  `./job-archive/version.txt` to version 2. Only metrics that have the footprint
  attribute set can be filtered and show up in the footprint UI and polar plot.
- Continuous scrolling is default now in all job lists. You can change this back
  to paging globally, also every user can configure to use paging or continuous
  scrolling individually.
- Tags have a scope now. Existing tags will get global scope in the database
  migration.

## New features

- Enable to delete tags from the web interface

## Known issues

- Currently energy footprint metrics of type energy are ignored for calculating
  total energy.
- Resampling for running jobs only works with cc-metric-store
- With energy footprint metrics of type power the unit is ignored and it is
  assumed the metric has the unit Watt.
