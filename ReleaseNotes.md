# `cc-backend` version 1.4.1

Supports job archive version 2 and database version 8.

This is a small bug fix release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Breaking changes

- You need to perform a database migration. Depending on your database size the
  migration might require several hours!
- You need to adapt the `cluster.json` configuration files in the job-archive,
  add new required attributes to the metric list and after that edit
  `./job-archive/version.txt` to version 2.
- Continuous scrolling is default now in all job lists. You can change this back
  to paging globally, also every user can configure to use paging or continuous
  scrolling individually.
- Tags have a scope now. Existing tags will get global scope in the database
  migration.

## New features

- Tags have a scope now. Tags created by a basic user are only visible by that
  user. Tags created by an admin/support role can be configured to be visible by
  all users (global scope) or only be admin/support role.
- Re-sampling support for running (requires a recent `cc-metric-store`) and
  archived jobs. This greatly speeds up loading of large or very long jobs. You
  need to add the new configuration key `enable-resampling` to the `config.json`
  file.
- For finished jobs a total job energy is shown in the job view.
- Continuous scrolling in job lists is default now.
- All database queries (especially for sqlite) were optimized resulting in
  dramatically faster load times.
- A performance and energy footprint can be freely configured on a per
  subcluster base. One can filter for footprint statistics for running and
  finished jobs.

## Known issues

- Currently energy footprint metrics of type energy are ignored for calculating
  total energy.
- Resampling for running jobs only works with cc-metric-store
- With energy footprint metrics of type power the unit is ignored and it is
  assumed the metric has the unit Watt.
