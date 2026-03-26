# `cc-backend` version 1.5.3

Supports job archive version 3 and database version 11.

This is a bugfix release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).
If you are upgrading from v1.5.1 no database migration is required.
If you are upgrading from v1.5.0 you need to do another DB migration. This
should not take long. For optimal database performance after the migration it is
recommended to apply the new `optimize-db` flag, which runs the sqlite `ANALYZE`
and `VACUUM` commands. Depending on your database size (more then 40GB) the
`VACUUM` may take up to 2h. You can also run the `ANALYZE` command manually.
While we are confident that the memory issue with the metricstore cleanup move
policy is fixed, it is still recommended to use delete policy for cleanup.
This is also the default.

## Changes in 1.5.3

### Bug fixes

- **Doubleranged filter fixes**: Range filters now correctly handle zero as a
  boundary value. Improved validation and UI text for "more than equal" and
  "less than equal" range selections.
- **Lineprotocol body parsing interrupted**: Switched from `ReadTimeout` to
  `ReadHeaderTimeout` so that long-running metric submissions are no longer
  cut off mid-stream.
- **Checkpoint archiving continues on error**: A single cluster's archiving
  failure no longer aborts the entire cleanup operation. Errors are collected
  and reported per cluster.
- **Parquet row group overflow**: Added periodic flush during checkpoint
  archiving to prevent exceeding the parquet-go 32k column-write limit.
- **Removed metrics excluded from subcluster config**: Metrics removed from a
  subcluster are no longer returned by `GetMetricConfigSubCluster`.

### MetricStore performance

- **WAL writer throughput**: Decoupled WAL file flushing from message processing
  using a periodic 5-second batch flush (up to 4096 messages per cycle),
  significantly increasing metric ingestion throughput.
- **Improved shutdown time**: HTTP shutdown timeout reduced; metricstore and
  archiver now shut down concurrently. Overall shutdown deadline raised to
  60 seconds.

### New features

- **Manual checkpoint cleanup flag**: New `-cleanup-checkpoints` CLI flag
  triggers checkpoint cleanup without starting the server, useful for
  maintenance windows or automated cleanup scripts.
- **Explicit node state queries in node view**: Node health and scheduler state
  are now fetched independently from metric data for fresher status information.

### Logging improvements

- **Reduced tagger log noise**: Missing metrics and expression evaluation errors
  in the job classification tagger are now logged at debug level instead of
  error level.

## Changes in 1.5.2

### Bug fixes

- **Memory spike in parquet writer**: Fixed memory spikes when using the
  metricstore move (archive) policy with the parquet writer. The writer now
  processes data in a streaming fashion to avoid accumulating large allocations.
- **Top list query fixes**: Fixed top list queries in analysis and dashboard
  views.
- **Exclude down nodes from HealthCheck**: Down nodes are now excluded from
  health checks in both the REST and NATS handlers.
- **Node state priority order**: Node state determination now enforces a
  priority order. Exception: idle+down results in idle.
- **Blocking ReceiveNats call**: Fixed a blocking NATS receive call in the
  metricstore.

### Database performance

- **Reduced insert pressure**: Bulk insert operations (node state updates, user
  and job cache syncs) now use explicit transactions and deferred inserts,
  significantly reducing write contention on the SQLite database.
- **SyncJobs wrapped in transaction**: `SyncJobs` now runs inside a transaction
  for better consistency and reduced lock contention.
- **Configurable busy timeout**: New `busy-timeout` configuration option for the
  SQLite connection. This allows tuning how long the driver waits for a locked
  database before returning an error, which improves resilience under concurrent
  write load.
- **Increased default SQLite timeout**: The default SQLite connection timeout
  has been raised to reduce spurious timeout errors under load.
- **Optimized stats queries**: Improved sortby handling in stats queries, fixed
  cache key passing, and simplified a stats query condition that caused an
  expensive unnecessary subquery.

### MetricStore performance

- **Sharded WAL consumer**: The WAL consumer is now sharded for significantly
  higher write throughput.
- **NATS contention fix**: Fixed contention in the metricstore NATS ingestion
  path.

### NATS API

- **Nodestate health checks in NATS API**: The NATS node state handler now
  performs the same metric health checks as the REST API handler, including
  per-subcluster health checks and `MonitoringStateFailed` fallback for nodes
  without health data.

### Logging improvements

- **Better error context**: Several repository functions now include the calling
  function name in error messages for easier diagnosis.
- **Reduced log noise**: `ErrNoRows` (no results found) is no longer logged as
  an error in `scanRow`; common "no rows" paths are now silent.
- **Debug-level missing metrics**: Warning about missing metrics in the metric
  store has been downgraded to debug level to reduce log noise in normal
  operation.
- **Checkpoint archiving log**: Added an informational log message when the
  metricstore checkpoint archiving process runs.
- **Auth failure context**: Auth failure log messages now include more context
  information.

### Behavior changes

- **DB-based metricHealth**: Replaced heuristic-based metric health with
  DB-based metric health for the node view, providing more accurate health
  status information.
- **Removed minRunningFor filter remnants**: Cleaned up remaining `minRunningFor`
  references from the GraphQL schema and query builder.

### Frontend

- **Streamlined statsSeries**: Unified stats series calculation and rendering
  across plot components.
- **Clarified plot titles**: Improved titles in dashboard and health views.
- **Bumped frontend dependencies**: Updated frontend dependencies to latest
  versions.

### Dependencies

- **cc-lib upgraded**: Updated to latest cc-lib version.

## Known issues

- The new dynamic memory management is not bullet proof yet across restarts.
  Buffers that are kept outside the retention period may be lost across a
  restart. We will fix that in a subsequent patch release.
- To use the new log viewer (which is only working when starting cc-backend with
  systemd) in the admin interface the user under which the cc-backend process is
  running has to be allowed to execute the journalctl command.
- The user configuration keys for the ui have changed. Therefore old user
  configuration persisted in the database is not used anymore. It is recommended
  to configure the metrics shown in the ui-config section and remove all records
  in the table after the update.
- Currently energy footprint metrics of type energy are ignored for calculating
  total energy.
- With energy footprint metrics of type power the unit is ignored and it is
  assumed the metric has the unit Watt.

## Changes in 1.5.1

### Database

- **New migration (version 11)**: Optimized database index count and added covering indexes for stats queries for significantly improved query performance
- **Migration 9 fix**: Removed redundant indices from migration 9 that are superseded by migration 11
- **Optional DB optimization flag**: Added `-optimize-db` CLI flag to run `ANALYZE` on demand; removed automatic ANALYZE on startup
- **Selective stats queries**: Stats queries are now selective, reducing unnecessary computation
- **User list paging**: Added paging support to the user list for better scalability
- **SQLite configuration hardening**: Sanitized SQLite configuration with new configurable options; fixes large heap allocations in the SQLite driver
- **Query cancellation**: Long-running database queries can now be cancelled
- **Resource leak fix**: Added missing `defer Close()` calls for all query result sets

### Bug fixes

- **Segfault when taggers misconfigured**: Fixed crash when `enable-job-taggers` is set but tagger rule directories are missing
- **GroupBy stats query complexity**: Reduced complexity for `groupBy` statistics queries
- **Ranged filter conditions**: Fixed GT and LT conditions in ranged filters
- **Energy filter preset**: Reduced energy filter preset to a more practical default
- **JSON validity check**: Fixed wrong field being checked for JSON validity
- **Tagger float rounding**: Fixed rounding of floats in tagger messages
- **Node view null safety**: Added null-safe checks in node view to prevent runtime errors
- **Public dashboard null safety**: Added null-safe checks in the public dashboard to prevent runtime errors

### Frontend

- **Bumped patch versions**: Updated frontend dependencies to latest patch versions

### Documentation

- **New DB config options**: Added new database configuration options to README

---

_The sections below document all features and changes introduced in the 1.5.0 major release, which 1.5.1 is based on._

## Breaking changes

### Configuration changes

- **JSON attribute naming**: All JSON configuration attributes now use `kebab-case`
  style consistently (e.g., `api-allowed-ips` instead of `apiAllowedIPs`).
  Update your `config.json` accordingly.
- **Removed `disable-archive` option**: This obsolete configuration option has been removed.
- **Removed `clusters` config section**: The separate clusters configuration section
  has been removed. Cluster information is now derived from the job archive.
- **`apiAllowedIPs` is now optional**: If not specified, defaults to not
  restricted.

### Architecture changes

- **Web framework replaced**: Migrated from `gorilla/mux` to `chi` as the HTTP
  router. This should be transparent to users but affects how middleware and
  routes are composed. A proper 404 handler is now in place.
- **MetricStore moved**: The `metricstore` package has been moved from `internal/`
  to `pkg/` as it is now part of the public API.
- **MySQL/MariaDB support removed**: Only SQLite is now supported as the database backend.
- **Archive to Cleanup renaming**: Archive-related functions have been refactored
  and renamed to "Cleanup" for clarity.
- **`minRunningFor` filter removed**: This undocumented filter has been removed
  from the API and frontend.

### Dependency changes

- **cc-lib v2.8.0**: Switched to cc-lib version 2 with updated APIs
- **cclib NATS client**: Now using the cclib NATS client implementation
- Removed obsolete `util.Float` usage from cclib

## Major new features

### NATS API Integration

- **Real-time job events**: Subscribe to job start/stop events via NATS
- **Node state updates**: Receive real-time node state changes via NATS
- **Configurable subjects**: NATS API subjects are now configurable via `api-subjects`
- **Deadlock fixes**: Improved NATS client stability and graceful shutdown

### Public Dashboard

- **Public-facing interface**: New public dashboard route for external users
- **DoubleMetricPlot component**: New visualization component for comparing metrics
- **Improved layout**: Reviewed and optimized dashboard layouts for better readability

### Enhanced Node Management

- **Node state tracking**: New node table in database with timestamp tracking
- **Node state filtering**: Filter jobs by node state in systems view
- **Node list enhancements**: Improved paging, filtering, and continuous scroll support
- **Nodestate retention and archiving**: Node state data is now subject to configurable
  retention policies and can be archived to Parquet format for long-term storage
- **Faulty node metric tracking**: Faulty node state metric lists are persisted to the database

### Health Monitoring

- **Health status dashboard**: New dedicated "Health" tab in the status details view
  showing per-node metric health across the cluster
- **CCMS health check**: Support for querying health status of external
  cc-metric-store (CCMS) instances via the API
- **GraphQL health endpoints**: New GraphQL queries and resolvers for health data
- **Cluster/subcluster filter**: Filter health status view by cluster or subcluster

### Log Viewer

- **Web-based log viewer**: New log viewer page in the admin interface for inspecting
  backend log output directly from the browser without shell access
- **Accessible from header**: Quick access link from the navigation header

### MetricStore Improvements

- **Memory tracking worker**: New worker for CCMS memory usage tracking
- **Dynamic retention**: Support for job specific dynamic retention times
- **Improved compression**: Transparent compression for job archive imports
- **Parallel processing**: Parallelized Iter function in all archive backends

### Job Tagging System

- **Job tagger option**: Enable automatic job tagging via configuration flag
- **Application detection**: Automatic detection of applications (MATLAB, GROMACS, etc.)
- **Job classification**: Automatic detection of pathological jobs
- **omit-tagged**: Option to exclude tagged jobs from retention/cleanup operations (`none`, `all`, or `user`)
- **Admin UI trigger**: Taggers can be run on-demand from the admin web interface
  without restarting the backend

### Archive Backends

- **Parquet archive format**: New Parquet file format for job archiving, providing
  columnar storage with efficient compression for analytical workloads
- **S3 backend**: Full support for S3-compatible object storage
- **SQLite backend**: Full support for SQLite backend using blobs
- **Performance improvements**: Fixed performance bugs in archive backends
- **Better error handling**: Improved error messages and fallback handling
- **Zstd compression**: Parquet writers use zstd compression for better
  compression ratios compared to the previous snappy default
- **Optimized sort order**: Job and nodestate Parquet files are sorted by
  cluster, subcluster, and start time for efficient range queries

### Unified Archive Retention and Format Conversion

- **Uniform retention policy**: Job archive retention now supports both JSON and
  Parquet as target formats under a single, consistent policy configuration
- **Archive manager tool**: The `tools/archive-manager` utility now supports
  format conversion between JSON and Parquet job archives
- **Parquet reader**: Full Parquet archive reader implementation for reading back
  archived job data

## New features and improvements

### Frontend

- **Loading indicators**: Added loading indicators to status detail and job lists
- **Job info layout**: Reviewed and improved job info row layout
- **Metric selection**: Enhanced metric selection with drag-and-drop fixes
- **Filter presets**: Move list filter preset to URL for easy sharing
- **Job comparison**: Improved job comparison views and plots
- **Subcluster reactivity**: Job list now reacts to subcluster filter changes
- **Short jobs quick selection**: New "Short jobs" quick-filter button in job lists
  replaces the removed undocumented `minRunningFor` filter
- **Row plot cursor sync**: Cursor position is now synchronized across all metric
  plots in a job list row for easier cross-metric comparison
- **Disabled metrics handling**: Improved handling and display of disabled metrics
  across job view, node view, and list rows
- **"Not configured" info cards**: Informational cards shown when optional features
  are not yet configured
- **Frontend dependencies**: Bumped frontend dependencies to latest versions
- **Svelte 5 compatibility**: Fixed Svelte state warnings and compatibility issues

### Backend

- **Progress bars**: Import function now shows progress during long operations
- **Better logging**: Improved logging with appropriate log levels throughout
- **Graceful shutdown**: Fixed shutdown timeout bugs and hanging issues
- **Configuration defaults**: Sensible defaults for most configuration options
- **Documentation**: Extensive documentation improvements across packages
- **Server flag in systemd unit**: Example systemd unit now includes the `-server` flag

### Security

- **LDAP security hardening**: Improved input validation, connection handling, and
  error reporting in the LDAP authenticator
- **OIDC security hardening**: Stricter token validation and improved error handling
  in the OIDC authenticator
- **Auth schema extensions**: Additional schema fields for improved auth configuration

### API improvements

- **Role-based metric visibility**: Metrics can now have role-based access control
- **Job exclusivity filter**: New filter for exclusive vs. shared jobs
- **Improved error messages**: Better error messages and documentation in REST API
- **GraphQL enhancements**: Improved GraphQL queries and resolvers
- **Stop job lookup order**: Reversed lookup order in stop job requests for
  more reliable job matching (cluster+jobId first, then jobId alone)

### Performance

- **Database indices**: Optimized SQLite indices for better query performance
- **Job cache**: Introduced caching table for faster job inserts
- **Parallel imports**: Archive imports now run in parallel where possible
- **External tool integration**: Optimized use of external tools (fd) for better performance
- **Node repository queries**: Reviewed and optimized node repository SQL queries
- **Buffer pool**: Resized and pooled internal buffers for better memory reuse

### Developer experience

- **AI agent guidelines**: Added documentation for AI coding agents (AGENTS.md, CLAUDE.md)
- **Example API payloads**: Added example JSON API payloads for testing
- **Unit tests**: Added more unit tests for NATS API, node repository, and other components
- **Test improvements**: Better test coverage; test DB is now copied before unit tests
  to avoid state pollution between test runs
- **Parquet writer tests**: Comprehensive tests for Parquet archive writing and conversion

## Bug fixes

- Fixed nodelist paging issues
- Fixed metric select drag and drop functionality
- Fixed render race conditions in nodeList
- Fixed tag count grouping including type
- Fixed wrong metricstore schema (missing comma)
- Fixed configuration issues causing shutdown hangs
- Fixed deadlock when NATS is not configured
- Fixed archive backend performance bugs
- Fixed continuous scroll buildup on refresh
- Improved footprint calculation logic
- Fixed polar plot data query decoupling
- Fixed missing resolution parameter handling
- Fixed node table initialization fallback
- Fixed reactivity key placement in nodeList
- Fixed nodeList resolver data handling and increased nodestate filter cutoff
- Fixed job always being transferred to main job table before archiving
- Fixed AppTagger error handling and logging
- Fixed log endpoint formatting and correctness
- Fixed automatic refresh in metric status tab
- Fixed NULL value handling in `health_state` and `health_metrics` columns
- Fixed bugs related to `job_cache` IDs being used in the main job table
- Fixed SyncJobs bug causing start job hooks to be called with wrong (cache) IDs
- Fixed 404 handler route for sub-routers

## Configuration changes

### New configuration options

```json
{
  "main": {
    "enable-job-taggers": true,
    "resampling": {
      "minimum-points": 600,
      "trigger": 180,
      "resolutions": [240, 60]
    },
    "api-subjects": {
      "subject-job-event": "cc.job.event",
      "subject-node-state": "cc.node.state"
    }
  },
  "nats": {
    "address": "nats://0.0.0.0:4222",
    "username": "root",
    "password": "root"
  },
  "cron": {
    "commit-job-worker": "1m",
    "duration-worker": "5m",
    "footprint-worker": "10m"
  },
  "metric-store": {
    "cleanup": {
      "mode": "archive",
      "interval": "48h",
      "directory": "./var/archive"
    }
  },
  "archive": {
    "retention": {
      "policy": "delete",
      "age": "6months",
      "target-format": "parquet"
    }
  },
  "nodestate": {
    "retention": {
      "policy": "archive",
      "age": "30d",
      "archive-path": "./var/nodestate-archive"
    }
  }
}
```

## Migration notes

- Review and update your `config.json` to use kebab-case attribute names
- If using NATS, configure the new `nats` and `api-subjects` sections
- If using S3 archive backend, configure the new `archive` section options
- Test the new public dashboard at `/public` route
- Review cron worker configuration if you need different frequencies
- If using the archive retention feature, configure the `target-format` option
  to choose between `json` (default) and `parquet` output formats
- Consider enabling nodestate retention if you track node states over time
