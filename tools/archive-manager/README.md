# Archive Manager

## Overview

The `archive-manager` tool manages ClusterCockpit job archives. It supports inspecting archives, validating jobs, removing jobs by date range, importing jobs between archive backends, and converting archives between JSON and Parquet formats.

## Features

- **Archive Info**: Display statistics about an existing job archive
- **Validation**: Validate job archives against the JSON schema
- **Cleanup**: Remove jobs by date range
- **Import**: Copy jobs between archive backends (file, S3, SQLite) with parallel processing
- **Convert**: Convert archives between JSON and Parquet formats (both directions)
- **Progress Reporting**: Real-time progress display with ETA and throughput metrics
- **Graceful Interruption**: CTRL-C stops processing after finishing current jobs

## Usage

### Build

```bash
go build ./tools/archive-manager/
```

### Archive Info

Display statistics about a job archive:

```bash
./archive-manager -s ./var/job-archive
```

### Validate Archive

```bash
./archive-manager -s ./var/job-archive --validate --config ./config.json
```

### Remove Jobs by Date

```bash
# Remove jobs started before a date
./archive-manager -s ./var/job-archive --remove-before 2023-Jan-01 --config ./config.json

# Remove jobs started after a date
./archive-manager -s ./var/job-archive --remove-after 2024-Dec-31 --config ./config.json
```

### Import Between Backends

Import jobs from one archive backend to another (e.g., file to S3, file to SQLite):

```bash
./archive-manager --import \
  --src-config '{"kind":"file","path":"./var/job-archive"}' \
  --dst-config '{"kind":"s3","endpoint":"https://s3.example.com","bucket":"archive","access-key":"...","secret-key":"..."}'
```

### Convert JSON to Parquet

Convert a JSON job archive to Parquet format:

```bash
./archive-manager --convert --format parquet \
  --src-config '{"kind":"file","path":"./var/job-archive"}' \
  --dst-config '{"kind":"file","path":"./var/parquet-archive"}'
```

The source (`--src-config`) is a standard archive backend config (file, S3, or SQLite). The destination (`--dst-config`) specifies where to write parquet files.

### Convert Parquet to JSON

Convert a Parquet archive back to JSON format:

```bash
./archive-manager --convert --format json \
  --src-config '{"kind":"file","path":"./var/parquet-archive"}' \
  --dst-config '{"kind":"file","path":"./var/json-archive"}'
```

The source (`--src-config`) points to a directory or S3 bucket containing parquet files organized by cluster. The destination (`--dst-config`) is a standard archive backend config.

### S3 Source/Destination Example

Both conversion directions support S3:

```bash
# JSON (S3) -> Parquet (local)
./archive-manager --convert --format parquet \
  --src-config '{"kind":"s3","endpoint":"https://s3.example.com","bucket":"json-archive","accessKey":"...","secretKey":"..."}' \
  --dst-config '{"kind":"file","path":"./var/parquet-archive"}'

# Parquet (local) -> JSON (S3)
./archive-manager --convert --format json \
  --src-config '{"kind":"file","path":"./var/parquet-archive"}' \
  --dst-config '{"kind":"s3","endpoint":"https://s3.example.com","bucket":"json-archive","access-key":"...","secret-key":"..."}'
```

## Command-Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-s` | `./var/job-archive` | Source job archive path (for info/validate/remove modes) |
| `--config` | `./config.json` | Path to config.json |
| `--loglevel` | `info` | Logging level: debug, info, warn, err, fatal, crit |
| `--logdate` | `false` | Add timestamps to log messages |
| `--validate` | `false` | Validate archive against JSON schema |
| `--remove-before` | | Remove jobs started before date (Format: 2006-Jan-02) |
| `--remove-after` | | Remove jobs started after date (Format: 2006-Jan-02) |
| `--import` | `false` | Import jobs between archive backends |
| `--convert` | `false` | Convert archive between JSON and Parquet formats |
| `--format` | `json` | Output format for conversion: `json` or `parquet` |
| `--max-file-size` | `512` | Max parquet file size in MB (only for parquet output) |
| `--src-config` | | Source config JSON (required for import/convert) |
| `--dst-config` | | Destination config JSON (required for import/convert) |

## Parquet Archive Layout

When converting to Parquet, the output is organized by cluster:

```
parquet-archive/
  clusterA/
    cluster.json
    cc-archive-2025-01-20-001.parquet
    cc-archive-2025-01-20-002.parquet
  clusterB/
    cluster.json
    cc-archive-2025-01-20-001.parquet
```

Each parquet file contains job metadata and gzip-compressed metric data. The `cluster.json` file preserves the cluster configuration from the source archive.

## Round-Trip Conversion

Archives can be converted from JSON to Parquet and back without data loss:

```bash
# Original JSON archive
./archive-manager --convert --format parquet \
  --src-config '{"kind":"file","path":"./var/job-archive"}' \
  --dst-config '{"kind":"file","path":"./var/parquet-archive"}'

# Convert back to JSON
./archive-manager --convert --format json \
  --src-config '{"kind":"file","path":"./var/parquet-archive"}' \
  --dst-config '{"kind":"file","path":"./var/json-archive"}'
```
