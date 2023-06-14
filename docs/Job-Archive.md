The job archive specifies an exchange format for job meta and performance metric
data. It consists of two parts:
* a [SQLite database schema](https://github.com/ClusterCockpit/cc-backend/wiki/Job-Archive#sqlite-database-schema)  for job meta data and performance statistics
* a [Json file format](https://github.com/ClusterCockpit/cc-backend/wiki/Job-Archive#json-file-format) together with a [Directory hierarchy specification](https://github.com/ClusterCockpit/cc-backend/wiki/Job-Archive#directory-hierarchy-specification)

By using an open, portable and simple specification based on files it is
possible to exchange job performance data for research and analysis purposes as
well as use it as a robust way for archiving job performance data to disk.

# SQLite database schema
## Introduction

A SQLite 3 database schema is provided to standardize the job meta data
information in a portable way. The schema also includes optional columns for job
performance statistics (called a job performance footprint). The database acts
as a front end to filter and select subsets of job IDs, that are the keys to get
the full job performance data in the job performance tree hierarchy.

## Database schema

The schema includes 3 tables: the job table, a tag table and a jobtag table
representing the MANY-TO-MANY relation between jobs and tags. The SQL schema is
specified
[here](https://github.com/ClusterCockpit/cc-specifications/blob/master/schemas/jobs-sqlite.sql).
Explanation of the various columns including the JSON datatypes is documented
[here](https://github.com/ClusterCockpit/cc-specifications/blob/master/datastructures/job-meta.schema.json).

# Directory hierarchy specification

## Specification

To manage the number of directories within a single directory a tree approach is
used splitting the integer job ID. The job id is split in junks of 1000 each.
Usually 2 layers of directories is sufficient but the concept can be used for an
arbitrary number of layers.

For a 2 layer schema this can be achieved with (code example in Perl):
``` perl
$level1 = $jobID/1000;
$level2 = $jobID%1000;
$dstPath = sprintf("%s/%s/%d/%03d", $trunk, $destdir, $level1, $level2);
```

## Example

For the job ID 1034871 the directory path is `./1034/871/`.

# Json file format
## Overview

Every cluster must be configured in a `cluster.json` file.

The job data consists of two files:
* `meta.json`: Contains job meta information and job statistics.
* `data.json`: Contains complete job data with time series

The description of the json format specification is available as [[json
schema|https://json-schema.org/]] format file. The latest version of the json
schema is part of the `cc-backend` source tree. For external reference it is
also available in a separate repository.

## Specification `cluster.json`

The json schema specification is available
[here](https://github.com/ClusterCockpit/cc-specifications/blob/master/datastructures/cluster.schema.json).

## Specification `meta.json`

The json schema specification is available
[here](https://github.com/RRZE-HPC/HPCJobDatabase/blob/master/json-schema/job-meta.schema.json).

## Specification `data.json`

The json schema specification is available
[here](https://github.com/RRZE-HPC/HPCJobDatabase/blob/master/json-schema/job-data.schema.json).
Metric time series data is stored for a fixed time step. The time step is set
per metric. If no value is available for a metric time series data timestamp
`null` is entered. 
