# `cc-backend` version 1.0.0

Supports job archive version 1 and database version 4.

This is the initial release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.

**Breaking changes**

The aggregate job statistic core hours is now computed using the job table
column `num_hwthreads`. In a future release this column will be renamed to
`num_cores`. For correct display of core hours `num_hwthreads` must be correctly
filled on job start. If your existing jobs do not provide the correct value in
this column then you can set this with one SQL INSERT statement. This only applies
if you have exclusive jobs, only. Please be aware that we treat this column as
it is the number of cores. In case you have SMT enabled and `num_hwthreads`
is not the number of cores the core hours will be too high by a factor!

**Notable changes**
* Supports user roles admin, support, manager, user, and api.
* Unified search bar supports job id, job name, project id, user name, and name
* Performance improvements for sqlite db backend
* Extended REST api supports to query job metrics
* Better support for shared jobs
* More flexible metric list configuration
* Versioning and migration for database and job archive
