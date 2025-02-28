# `cc-backend` version 1.4.3

Supports job archive version 2 and database version 8.

This is a bug fix release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Breaking changes

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

- Detailed Node List
  - Adds new routes `/systems/list/$cluster` and `/systems/list/$cluster/$subcluster`
  - Displays live, scoped metric data requested from the nodes indepenent of jobs
- Color Blind Mode
  - Set on a per-user basis in options
  - Applies to plot data, plot background color, statsseries colors, roofline timescale
- Histogram Bin Select in User-View
  - Metric-Histograms: `10 Bins` now default, selectable options `20, 50, 100`
  - Job-Duration-Histogram: `48h in 1h Bins` now default, selectable options:
    - `60 minutes in 1 minute Bins`
    - `12 hours in 10 minute Bins`
    - `3 days in 6 hour Bins`
    - `7 days in 12 hour Bins`

## Known issues

- Currently energy footprint metrics of type energy are ignored for calculating
  total energy.
- Resampling for running jobs only works with cc-metric-store
- With energy footprint metrics of type power the unit is ignored and it is
  assumed the metric has the unit Watt.
