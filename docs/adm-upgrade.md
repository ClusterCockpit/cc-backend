In general, an upgrade is nothing more than a replacement of the binary file.
All the necessary files, except the database file, the configuration file and
the job archive, are embedded in the binary file. It is recommended to use a
directory where the file names of the binary files are named with a version
indicator. This can be, for example, the date or the Unix epoch time. A symbolic
link points to the version to be used. This makes it easier to switch to earlier
versions.

The database and the job archive are versioned. Each release binary supports
specific versions of the database and job archive. If a version mismatch is
detected, the application is terminated and migration is required.

**IMPORTANT NOTE**

It is recommended to make a backup copy of the database before each update. This
is mandatory in case the database needs to be migrated. In the case of sqlite,
this means to stopping `cc-backend` and copying the sqlite database file
somewhere.

#  Migrating the database

After you have backed up the database, run the following command to migrate the
database to the latest version:
```
$ ./cc-backend -migrate-db
```

The migration files are embedded in the binary and can also be viewed in the cc
backend [source tree](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/repository/migrations).
There are separate migration files for both supported
database backends.
We use the [migrate library](https://github.com/golang-migrate/migrate).

If something goes wrong, you can check the status and get the current schema
(here for sqlite):
```
$ sqlite3 var/job.db
```
In the sqlite console execute:
```
.schema
```
to get the current databse schema.
You can query the current version and whether the migration failed with:
```
SELECT * FROM schema_migrations;
```
The first column indicates the current database version and the second column is
a dirty flag indicating whether the migration was successful.

# Migrating the job archive

Job archive migration requires a separate tool (`archive-migration`), which is
part of the cc-backend source tree (build with `go build ./tools/archive-migration`)
and is also provided as part of the releases.

Migration is supported only between two successive releases. The migration tool
migrates the existing job archive to a new job archive. This means that there
must be enough disk space for two complete job archives. If the tool is called
without options:
```
$ ./archive-migration
```

it is assumed that a job archive exists in `./var/job-archive`. The new job
archive is written to `./var/job-archive-new`. Since execution is threaded in case
of a fatal error, it is impossible to determine in which job the error occurred.
In this case, you can run the tool in debug mode (with the `-debug` flag). In
debug mode, threading is disabled and the job ID of each migrated job is output.
Jobs with empty files will be skipped. Between multiple runs of the tools, the
`job-archive-new` directory must be moved or deleted.

The `cluster.json` files in `job-archive-new` must be checked for errors, especially
whether the aggregation attribute is set correctly for all metrics.

Migration takes several hours for relatively large job archives (several hundred
GB). A versioned job archive contains a version.txt file in the root directory
of the job archive. This file contains the version as an unsigned integer.
