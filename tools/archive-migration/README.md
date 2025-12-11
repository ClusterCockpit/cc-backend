# Archive Migration Tool

## Overview

The `archive-migration` tool migrates job archives from old schema versions to the current schema version. It handles schema changes such as the `exclusive` → `shared` field transformation and adds/removes fields as needed.

## Features

- **Parallel Processing**: Uses worker pool for fast migration
- **Dry-Run Mode**: Preview changes without modifying files
- **Safe Transformations**: Applies well-defined schema transformations
- **Progress Reporting**: Shows real-time migration progress
- **Error Handling**: Continues on individual failures, reports at end

## Schema Transformations

### Exclusive → Shared

Converts the old `exclusive` integer field to the new `shared` string field:

- `0` → `"multi_user"`
- `1` → `"none"`
- `2` → `"single_user"`

### Missing Fields

Adds fields required by current schema:

- `submitTime`: Defaults to `startTime` if missing
- `energy`: Defaults to `0.0`
- `requestedMemory`: Defaults to `0`
- `shared`: Defaults to `"none"` if still missing after transformation

### Deprecated Fields

Removes fields no longer in schema:

- `mem_used_max`, `flops_any_avg`, `mem_bw_avg`
- `load_avg`, `net_bw_avg`, `net_data_vol_total`
- `file_bw_avg`, `file_data_vol_total`

## Usage

### Build

```bash
cd ./tools/archive-migration
go build
```

### Dry Run (Preview Changes)

```bash
./archive-migration --archive /path/to/archive --dry-run
```

### Migrate Archive

```bash
# IMPORTANT: Backup your archive first!
cp -r /path/to/archive /path/to/archive-backup

# Run migration
./archive-migration --archive /path/to/archive
```

### Command-Line Options

- `--archive <path>`: Path to job archive (required)
- `--dry-run`: Preview changes without modifying files
- `--workers <n>`: Number of parallel workers (default: 4)
- `--loglevel <level>`: Logging level: debug, info, warn, err, fatal, crit (default: info)
- `--logdate`: Add timestamps to log messages

## Examples

```bash
# Preview what would change
./archive-migration --archive ./var/job-archive --dry-run

# Migrate with verbose logging
./archive-migration --archive ./var/job-archive --loglevel debug

# Migrate with 8 workers for faster processing
./archive-migration --archive ./var/job-archive --workers 8
```

## Safety

> [!CAUTION]
> **Always backup your archive before running migration!**

The tool modifies `meta.json` files in place. While transformations are designed to be safe, unexpected issues could occur. Follow these safety practices:

1. **Always run with `--dry-run` first** to preview changes
2. **Backup your archive** before migration
3. **Test on a copy** of your archive first
4. **Verify results** after migration

## Verification

After migration, verify the archive:

```bash
# Use archive-manager to check the archive
cd ../archive-manager
./archive-manager -s /path/to/migrated-archive

# Or validate specific jobs
./archive-manager -s /path/to/migrated-archive --validate
```

## Troubleshooting

### Migration Failures

If individual jobs fail to migrate:

- Check the error messages for specific files
- Examine the failing `meta.json` files manually
- Fix invalid JSON or unexpected field types
- Re-run migration (already-migrated jobs will be processed again)

### Performance

For large archives:

- Increase `--workers` for more parallelism
- Use `--loglevel warn` to reduce log output
- Monitor disk I/O if migration is slow

## Technical Details

The migration process:

1. Walks archive directory recursively
2. Finds all `meta.json` files
3. Distributes jobs to worker pool
4. For each job:
   - Reads JSON file
   - Applies transformations in order
   - Writes back migrated data (if not dry-run)
5. Reports statistics and errors

Transformations are idempotent - running migration multiple times is safe (though not recommended for performance).
