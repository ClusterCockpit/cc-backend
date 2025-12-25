# Archiver Package

The `archiver` package provides asynchronous job archiving functionality for ClusterCockpit. When jobs complete, their metric data is archived from the metric store to a persistent archive backend (filesystem, S3, SQLite, etc.).

## Architecture

### Producer-Consumer Pattern

```
┌──────────────┐     TriggerArchiving()      ┌───────────────┐
│  API Handler │  ───────────────────────▶   │ archiveChannel│
│ (Job Stop)   │                             │  (buffer: 128)│
└──────────────┘                             └───────┬───────┘
                                                     │
                   ┌─────────────────────────────────┘
                   │
                   ▼
         ┌──────────────────────┐
         │  archivingWorker()   │
         │   (goroutine)        │
         └──────────┬───────────┘
                    │
                    ▼
         1. Fetch job metadata
         2. Load metric data
         3. Calculate statistics
         4. Archive to backend
         5. Update database
         6. Call hooks
```

### Components

- **archiveChannel**: Buffered channel (128 jobs) for async communication
- **archivePending**: WaitGroup tracking in-flight archiving operations
- **archivingWorker**: Background goroutine processing archiving requests
- **shutdownCtx**: Context for graceful cancellation during shutdown

## Usage

### Initialization

```go
// Start archiver with context for shutdown control
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

archiver.Start(jobRepository, ctx)
```

### Archiving a Job

```go
// Called automatically when a job completes
archiver.TriggerArchiving(job)
```

The function returns immediately. Actual archiving happens in the background.

### Graceful Shutdown

```go
// Shutdown with 10 second timeout
if err := archiver.Shutdown(10 * time.Second); err != nil {
    log.Printf("Archiver shutdown timeout: %v", err)
}
```

**Shutdown process:**
1. Closes channel (rejects new jobs)
2. Waits for pending jobs (up to timeout)
3. Cancels context if timeout exceeded
4. Waits for worker to exit cleanly

## Configuration

### Channel Buffer Size

The archiving channel has a buffer of 128 jobs. If more than 128 jobs are queued simultaneously, `TriggerArchiving()` will block until space is available.

To adjust:
```go
// In archiveWorker.go Start() function
archiveChannel = make(chan *schema.Job, 256) // Increase buffer
```

### Scope Selection

Archive data scopes are automatically selected based on job size:

- **Node scope**: Always included
- **Core scope**: Included for jobs with ≤8 nodes (reduces data volume for large jobs)
- **Accelerator scope**: Included if job used accelerators (`NumAcc > 0`)

To adjust the node threshold:
```go
// In archiver.go ArchiveJob() function
if job.NumNodes <= 16 { // Change from 8 to 16
    scopes = append(scopes, schema.MetricScopeCore)
}
```

### Resolution

Data is archived at the highest available resolution (typically 60s intervals). To change:

```go
// In archiver.go ArchiveJob() function
jobData, err := metricdispatch.LoadData(job, allMetrics, scopes, ctx, 300)
// 0 = highest resolution
// 300 = 5-minute resolution
```

## Error Handling

### Automatic Retry

The archiver does **not** automatically retry failed archiving operations. If archiving fails:

1. Error is logged
2. Job is marked as `MonitoringStatusArchivingFailed` in database
3. Worker continues processing other jobs

### Manual Retry

To re-archive failed jobs, query for jobs with `MonitoringStatusArchivingFailed` and call `TriggerArchiving()` again.

## Performance Considerations

### Single Worker Thread

The archiver uses a single worker goroutine. For high-throughput systems:

- Large channel buffer (128) prevents blocking
- Archiving is typically I/O bound (writing to storage)
- Single worker prevents overwhelming storage backend

### Shutdown Timeout

Recommended timeout values:
- **Development**: 5-10 seconds
- **Production**: 10-30 seconds
- **High-load**: 30-60 seconds

Choose based on:
- Average archiving time per job
- Storage backend latency
- Acceptable shutdown delay

## Monitoring

### Logging

The archiver logs:
- **Info**: Startup, shutdown, successful completions
- **Debug**: Individual job archiving times
- **Error**: Archiving failures with job ID and reason
- **Warn**: Shutdown timeout exceeded

### Metrics

Monitor these signals for archiver health:
- Jobs with `MonitoringStatusArchivingFailed`
- Time from job stop to successful archive
- Shutdown timeout occurrences

## Thread Safety

All exported functions are safe for concurrent use:
- `Start()` - Safe to call once
- `TriggerArchiving()` - Safe from multiple goroutines
- `Shutdown()` - Safe to call once
- `WaitForArchiving()` - Deprecated, but safe

Internal state is protected by:
- Channel synchronization (`archiveChannel`)
- WaitGroup for pending count (`archivePending`)
- Context for cancellation (`shutdownCtx`)

## Files

- **archiveWorker.go**: Worker lifecycle, channel management, shutdown logic
- **archiver.go**: Core archiving logic, metric loading, statistics calculation

## Dependencies

- `internal/repository`: Database operations for job metadata
- `internal/metricdispatch`: Loading metric data from various backends
- `pkg/archive`: Archive backend abstraction (filesystem, S3, SQLite)
- `cc-lib/schema`: Job and metric data structures
