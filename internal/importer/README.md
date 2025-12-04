# Importer Package

The `importer` package provides functionality for importing job data into the ClusterCockpit database from archived job files.

## Overview

This package supports two primary import workflows:

1. **Bulk Database Initialization** - Reinitialize the entire job database from archived jobs
2. **Individual Job Import** - Import specific jobs from metadata/data file pairs

Both workflows enrich job metadata by calculating performance footprints and energy consumption metrics before persisting to the database.

## Main Entry Points

### InitDB()

Reinitializes the job database from all archived jobs.

```go
if err := importer.InitDB(); err != nil {
    log.Fatal(err)
}
```

This function:
- Flushes existing job, tag, and jobtag tables
- Iterates through all jobs in the configured archive
- Enriches each job with calculated metrics
- Inserts jobs into the database in batched transactions (100 jobs per batch)
- Continues on individual job failures, logging errors

**Use Case**: Initial database setup or complete database rebuild from archive.

### HandleImportFlag(flag string)

Imports jobs from specified file pairs.

```go
// Format: "<meta.json>:<data.json>[,<meta2.json>:<data2.json>,...]"
flag := "/path/to/meta.json:/path/to/data.json"
if err := importer.HandleImportFlag(flag); err != nil {
    log.Fatal(err)
}
```

This function:
- Parses the comma-separated file pairs
- Validates metadata and job data against schemas (if validation enabled)
- Enriches each job with footprints and energy metrics
- Imports jobs into both the archive and database
- Fails fast on the first error

**Use Case**: Importing specific jobs from external sources or manual job additions.

## Job Enrichment

Both import workflows use `enrichJobMetadata()` to calculate:

### Performance Footprints

Performance footprints are calculated from metric averages based on the subcluster configuration:

```go
job.Footprint["mem_used_avg"] = 45.2  // GB
job.Footprint["cpu_load_avg"] = 0.87   // percentage
```

### Energy Metrics

Energy consumption is calculated from power metrics using the formula:

```
Energy (kWh) = (Power (W) × Duration (s) / 3600) / 1000
```

For each energy metric:
```go
job.EnergyFootprint["acc_power"] = 12.5  // kWh
job.Energy = 150.2  // Total energy in kWh
```

**Note**: Energy calculations for metrics with unit "energy" (Joules) are not yet implemented.

## Data Validation

### SanityChecks(job *schema.Job)

Validates job metadata before database insertion:

- Cluster exists in configuration
- Subcluster is valid (assigns if needed)
- Job state is valid
- Resources and user fields are populated
- Node counts and hardware thread counts are positive
- Resource count matches declared node count

## Normalization Utilities

The package includes utilities for normalizing metric values to appropriate SI prefixes:

### Normalize(avg float64, prefix string)

Adjusts values and SI prefixes for readability:

```go
factor, newPrefix := importer.Normalize(2048.0, "M")  
// Converts 2048 MB → ~2.0 GB
// Returns: factor for conversion, "G"
```

This is useful for automatically scaling metrics (e.g., memory, storage) to human-readable units.

## Dependencies

- `github.com/ClusterCockpit/cc-backend/internal/repository` - Database operations
- `github.com/ClusterCockpit/cc-backend/pkg/archive` - Job archive access
- `github.com/ClusterCockpit/cc-lib/schema` - Job schema definitions
- `github.com/ClusterCockpit/cc-lib/ccLogger` - Logging
- `github.com/ClusterCockpit/cc-lib/ccUnits` - SI unit handling

## Error Handling

- **InitDB**: Continues processing on individual job failures, logs errors, returns summary
- **HandleImportFlag**: Fails fast on first error, returns immediately
- Both functions log detailed error context for debugging

## Performance

- **Transaction Batching**: InitDB processes jobs in batches of 100 for optimal database performance
- **Tag Caching**: Tag IDs are cached during import to minimize database queries
- **Progress Reporting**: InitDB prints progress updates during bulk operations
