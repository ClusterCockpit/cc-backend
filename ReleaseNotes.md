# `cc-backend` version 1.5.0

Supports job archive version 3 and database version 10.

This is a feature release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Breaking changes

### Configuration changes

- **JSON attribute naming**: All JSON configuration attributes now use `kebab-case`
  style consistently (e.g., `api-allowed-ips` instead of `apiAllowedIPs`).
  Update your `config.json` accordingly.
- **Removed `disable-archive` option**: This obsolete configuration option has been removed.
- **Removed `clusters` config section**: The separate clusters configuration section
  has been removed. Cluster information is now derived from the job archive.
- **`apiAllowedIPs` is now optional**: If not specified, defaults to secure settings.

### Architecture changes

- **MetricStore moved**: The `metricstore` package has been moved from `internal/`
  to `pkg/` as it is now part of the public API.
- **MySQL/MariaDB support removed**: Only SQLite is now supported as the database backend.
- **Archive to Cleanup renaming**: Archive-related functions have been refactored
  and renamed to "Cleanup" for clarity.

### Dependency changes

- **cc-lib v2**: Switched to cc-lib version 2 with updated APIs
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
- **Node metrics improvements**: Better handling of node-level metrics and data
- **Node list enhancements**: Improved paging, filtering, and continuous scroll support

### MetricStore Improvements

- **Memory tracking worker**: New worker for CCMS memory usage tracking
- **Dynamic retention**: Support for cluster/subcluster-specific retention times
- **Improved compression**: Transparent compression for job archive imports
- **Parallel processing**: Parallelized Iter function in all archive backends

### Job Tagging System

- **Job tagger option**: Enable automatic job tagging via configuration flag
- **Application detection**: Automatic detection of applications (MATLAB, GROMACS, etc.)
- **Job classifaction**: Automatic detection of pathological jobs
- **omitTagged flag**: Option to exclude tagged jobs from retention/cleanup operations

### Archive Backends

- **S3 backend**: Full support for S3-compatible object storage
- **SQLite backend**: Full support for SQLite backend using blobs
- **Performance improvements**: Fixed performance bugs in archive backends
- **Better error handling**: Improved error messages and fallback handling

## New features and improvements

### Frontend

- **Loading indicators**: Added loading indicators to status detail and job lists
- **Job info layout**: Reviewed and improved job info row layout
- **Metric selection**: Enhanced metric selection with drag-and-drop fixes
- **Filter presets**: Move list filter preset to URL for easy sharing
- **Job comparison**: Improved job comparison views and plots
- **Subcluster reactivity**: Job list now reacts to subcluster filter changes
- **Frontend dependencies**: Bumped frontend dependencies to latest versions
- **Svelte 5 compatibility**: Fixed Svelte state warnings and compatibility issues

### Backend

- **Progress bars**: Import function now shows progress during long operations
- **Better logging**: Improved logging with appropriate log levels throughout
- **Graceful shutdown**: Fixed shutdown timeout bugs and hanging issues
- **Configuration defaults**: Sensible defaults for most configuration options
- **Documentation**: Extensive documentation improvements across packages

### API improvements

- **Role-based metric visibility**: Metrics can now have role-based access control
- **Job exclusivity filter**: New filter for exclusive vs. shared jobs
- **Improved error messages**: Better error messages and documentation in REST API
- **GraphQL enhancements**: Improved GraphQL queries and resolvers

### Performance

- **Database indices**: Optimized SQLite indices for better query performance
- **Job cache**: Introduced caching table for faster job inserts
- **Parallel imports**: Archive imports now run in parallel where possible
- **External tool integration**: Optimized use of external tools (fd) for better performance

### Developer experience

- **AI agent guidelines**: Added documentation for AI coding agents (AGENTS.md, CLAUDE.md)
- **Example API payloads**: Added example JSON API payloads for testing
- **Unit tests**: Added more unit tests for NATS API and other components
- **Test improvements**: Better test coverage and test data

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
  }
}
```

## Migration notes

- Review and update your `config.json` to use kebab-case attribute names
- If using NATS, configure the new `nats` and `api-subjects` sections
- If using S3 archive backend, configure the new `archive` section options
- Test the new public dashboard at `/public` route
- Review cron worker configuration if you need different frequencies

## Known issues

- Currently energy footprint metrics of type energy are ignored for calculating
  total energy.
- Resampling for running jobs only works with cc-metric-store
- With energy footprint metrics of type power the unit is ignored and it is
  assumed the metric has the unit Watt.
