In general an upgrade is not more then replacing the binary. All files that are
required, apart from the database file, the configuration file and the job
archive are embedded into the binary. It is recommended to use a directory where
the binary filenames are named using some version tag. This may be, e.g., the date or
unix epoch time. A symbolic link points to the version to be used. This allows
to easily switch to previous versions.

The database and the job archive are versioned. Every release binary supports
specific versions of the database and the job archive. In case a version
mismatch is detected the application will exit and a migration is required.

**IMPORTANT NOTICE**

It is recommended to backup the database before any upgrade. It is mandatory
to backup in case the database is migrated. Using sqlite this means to stop
`cc-backend` and copy the sqlite database file somewhere.

#  Migrating the database
After backing up the database execute the the following command to migrate the
databse to the most recent version:
```
$ ./cc-backend -migrate-db
```

The migration files are embedded into the binary but can be reviewed in the
cc-backend [source tree](https://github.com/ClusterCockpit/cc-backend/tree/master/internal/repository/migrations).
There are separate migration files for both supported database backends.
We use the [migrate library](https://github.com/golang-migrate/migrate).

In case something goes wrong you can check the state and get the current schema
(here for sqlite):
```
$ sqlite3 var/job.db
```
In the sqlite console execute:
```
.schema
```
to get the current databse schema.
You can query the current version and if the migration failed with:
```
SELECT * FROM schema_migrations;
```
The first column indicates the current database version and the second column is
a dirty flag indicating if the migration was successful.

# Migrating the job archive

To migrate the job archive a separate tool (`archive-migration`) is required that is part of the
`cc-backend` source tree (build with `go build ./tools/archive-migration`) and also provided as part of releases.

Migration is only supported between two subsequent versions. The migration tool
will migrate the existing job archive into a new job archive. This means there
has to be enough disk space for two complete job-archives. If the tool is
called without options:
```
$ ./archive-migration
```

it is assumed that there is a job archive in `./var/job-archive`. The new job
archive will be written to `./var/job-archive-new`. Because execution is
threaded in case of a fatal error it is impossible to pinpoint in which job the
error occured. In this case you can run the tool in debug mode (using the flag
`-debug`). In debug mode threading will be disabled and the job id of every
migrated job is printed. Jobs with empty files are skipped. Between multiple runs
of the tools the directory `job-archive-new` has to be moved or deleted.

The cluster.json files in `job-archive-new` need to be reviewed for errors, in
particular it has to be checked if the aggregation attribute is set correctly
for all metrics.

The migration will take in the order of hours for fairly large (several hundred
GB) job archives. A versioned job archive contains a file `version.txt` in the
job archive root directory. This file contains the version as unsigned integer.
