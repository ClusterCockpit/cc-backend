# Job Tagging Configuration

ClusterCockpit provides automatic job tagging functionality to classify and
categorize jobs based on configurable rules. The tagging system consists of two
main components:

1. **Application Detection** - Identifies which application a job is running
2. **Job Classification** - Analyzes job performance characteristics and applies classification tags

## Directory Structure

```
configs/tagger/
├── apps/              # Application detection patterns
│   ├── vasp.txt
│   ├── gromacs.txt
│   └── ...
└── jobclasses/        # Job classification rules
    ├── parameters.json
    ├── lowUtilization.json
    ├── highload.json
    └── ...
```

## Activating Tagger Rules

### Step 1: Copy Configuration Files

To activate tagging, review, adapt, and copy the configuration files from
`configs/tagger/` to `var/tagger/`:

```bash
# From the cc-backend root directory
mkdir -p var/tagger
cp -r configs/tagger/apps var/tagger/
cp -r configs/tagger/jobclasses var/tagger/
```

### Step 2: Enable Tagging in Configuration

Add or set the following configuration key in the `main` section of your `config.json`:

```json
{
  "enable-job-taggers": true
}
```

**Important**: Automatic tagging is disabled by default. You must explicitly
enable it by setting `enable-job-taggers: true` in the main configuration file.

### Step 3: Restart cc-backend

The tagger system automatically loads configuration from `./var/tagger/` at
startup. After copying the files and enabling the feature, restart cc-backend:

```bash
./cc-backend -server
```

### Step 4: Verify Configuration Loaded

Check the logs for messages indicating successful configuration loading:

```
[INFO] Setup file watch for ./var/tagger/apps
[INFO] Setup file watch for ./var/tagger/jobclasses
```

## How Tagging Works

### Automatic Tagging

When `enable-job-taggers` is set to `true` in the configuration, tags are
automatically applied when:

- **Job Start**: Application detection runs immediately when a job starts
- **Job Stop**: Job classification runs when a job completes

The system analyzes job metadata and metrics to determine appropriate tags.

**Note**: Automatic tagging only works for jobs that start or stop after the
feature is enabled. Existing jobs are not automatically retagged.

### Manual Tagging (Retroactive)

To apply tags to existing jobs in the database, use the `-apply-tags` command
line option:

```bash
./cc-backend -apply-tags
```

This processes all jobs in the database and applies current tagging rules. This
is useful when:

- You have existing jobs that were created before tagging was enabled
- You've added new tagging rules and want to apply them to historical data
- You've modified existing rules and want to re-evaluate all jobs

### Hot Reload

The tagger system watches the configuration directories for changes. You can
modify or add rules without restarting `cc-backend`:

- Changes to `var/tagger/apps/*` are detected automatically
- Changes to `var/tagger/jobclasses/*` are detected automatically

## Application Detection

Application detection identifies which software a job is running by matching
patterns in the job script.

### Configuration Format

Application patterns are stored in text files under `var/tagger/apps/`. Each
file contains one or more regular expression patterns (one per line) that match
against the job script.

**Example: `apps/vasp.txt`**

```
vasp
VASP
```

### How It Works

1. When a job starts, the system retrieves the job script from metadata
2. Each line in the app files is treated as a regex pattern
3. Patterns are matched case-insensitively against the lowercased job script
4. If a match is found, a tag of type `app` with the filename (without extension) is applied
5. Only the first matching application is tagged

### Adding New Applications

1. Create a new file in `var/tagger/apps/` (e.g., `tensorflow.txt`)
2. Add regex patterns, one per line:

   ```
   tensorflow
   tf\.keras
   import tensorflow
   ```

3. The file is automatically detected and loaded

**Note**: The tag name will be the filename without the `.txt` extension (e.g., `tensorflow`).

## Job Classification

Job classification analyzes completed jobs based on their metrics and properties
to identify performance issues or characteristics.

### Configuration Format

Job classification rules are defined in JSON files under
`var/tagger/jobclasses/`. Each rule file defines:

- **Metrics required**: Which job metrics to analyze
- **Requirements**: Pre-conditions that must be met
- **Variables**: Computed values used in the rule
- **Rule expression**: Boolean expression that determines if the rule matches
- **Hint template**: Message displayed when the rule matches

### Parameters File

`jobclasses/parameters.json` defines shared threshold values used across multiple rules:

```json
{
  "lowcpuload_threshold_factor": 0.9,
  "highmemoryusage_threshold_factor": 0.9,
  "job_min_duration_seconds": 600.0,
  "sampling_interval_seconds": 30.0
}
```

### Rule File Structure

**Example: `jobclasses/lowUtilization.json`**

```json
{
  "name": "Low resource utilization",
  "tag": "lowutilization",
  "parameters": ["job_min_duration_seconds"],
  "metrics": ["flops_any", "mem_bw"],
  "requirements": [
    "job.shared == \"none\"",
    "job.duration > job_min_duration_seconds"
  ],
  "variables": [
    {
      "name": "mem_bw_perc",
      "expr": "1.0 - (mem_bw.avg / mem_bw.limits.peak)"
    }
  ],
  "rule": "flops_any.avg < flops_any.limits.alert",
  "hint": "Average flop rate {{.flops_any.avg}} falls below threshold {{.flops_any.limits.alert}}"
}
```

#### Field Descriptions

| Field          | Description                                                                   |
| -------------- | ----------------------------------------------------------------------------- |
| `name`         | Human-readable description of the rule                                        |
| `tag`          | Tag identifier applied when the rule matches                                  |
| `parameters`   | List of parameter names from `parameters.json` to include in rule environment |
| `metrics`      | List of metrics required for evaluation (must be present in job data)         |
| `requirements` | Boolean expressions that must all be true for the rule to be evaluated        |
| `variables`    | Named expressions computed before evaluating the main rule                    |
| `rule`         | Boolean expression that determines if the job matches this classification     |
| `hint`         | Go template string for generating a user-visible message                      |

### Expression Environment

Expressions in `requirements`, `variables`, and `rule` have access to:

**Job Properties:**

- `job.shared` - Shared node allocation type
- `job.duration` - Job runtime in seconds
- `job.numCores` - Number of CPU cores
- `job.numNodes` - Number of nodes
- `job.jobState` - Job completion state
- `job.numAcc` - Number of accelerators
- `job.smt` - SMT setting

**Metric Statistics (for each metric in `metrics`):**

- `<metric>.min` - Minimum value
- `<metric>.max` - Maximum value
- `<metric>.avg` - Average value
- `<metric>.limits.peak` - Peak limit from cluster config
- `<metric>.limits.normal` - Normal threshold
- `<metric>.limits.caution` - Caution threshold
- `<metric>.limits.alert` - Alert threshold

**Parameters:**

- All parameters listed in the `parameters` field

**Variables:**

- All variables defined in the `variables` array

### Expression Language

Rules use the [expr](https://github.com/expr-lang/expr) language for expressions. Supported operations:

- **Arithmetic**: `+`, `-`, `*`, `/`, `%`, `^`
- **Comparison**: `==`, `!=`, `<`, `<=`, `>`, `>=`
- **Logical**: `&&`, `||`, `!`
- **Functions**: Standard math functions (see expr documentation)

### Hint Templates

Hints use Go's `text/template` syntax. Variables from the evaluation environment are accessible:

```
{{.flops_any.avg}}          # Access metric average
{{.job.duration}}            # Access job property
{{.my_variable}}             # Access computed variable
```

### Adding New Classification Rules

1. Create a new JSON file in `var/tagger/jobclasses/` (e.g., `memoryLeak.json`)
2. Define the rule structure:

   ```json
   {
     "name": "Memory Leak Detection",
     "tag": "memory_leak",
     "parameters": ["memory_leak_slope_threshold"],
     "metrics": ["mem_used"],
     "requirements": ["job.duration > 3600"],
     "variables": [
       {
         "name": "mem_growth",
         "expr": "(mem_used.max - mem_used.min) / job.duration"
       }
     ],
     "rule": "mem_growth > memory_leak_slope_threshold",
     "hint": "Memory usage grew by {{.mem_growth}} per second"
   }
   ```

3. Add any new parameters to `parameters.json`
4. The file is automatically detected and loaded

## Configuration Paths

The tagger system reads from these paths (relative to cc-backend working directory):

- **Application patterns**: `./var/tagger/apps/`
- **Job classification rules**: `./var/tagger/jobclasses/`

These paths are defined as constants in the source code and cannot be changed without recompiling.

## Troubleshooting

### Tags Not Applied

1. **Check tagging is enabled**: Verify `enable-job-taggers: true` is set in `config.json`

2. **Check configuration exists**:

   ```bash
   ls -la var/tagger/apps
   ls -la var/tagger/jobclasses
   ```

3. **Check logs for errors**:

   ```bash
   ./cc-backend -server -loglevel debug
   ```

4. **Verify file permissions**: Ensure cc-backend can read the configuration files

5. **For existing jobs**: Use `./cc-backend -apply-tags` to retroactively tag jobs

### Rules Not Matching

1. **Enable debug logging**: Set `loglevel: debug` to see detailed rule evaluation
2. **Check requirements**: Ensure all requirements in the rule are satisfied
3. **Verify metrics exist**: Classification rules require job metrics to be available
4. **Check metric names**: Ensure metric names match those in your cluster configuration

### File Watch Not Working

If changes to configuration files aren't detected:

1. Restart cc-backend to reload all configuration
2. Check filesystem supports file watching (network filesystems may not)
3. Check logs for file watch setup messages

## Best Practices

1. **Start Simple**: Begin with basic rules and refine based on results
2. **Use Requirements**: Filter out irrelevant jobs early with requirements
3. **Test Incrementally**: Add one rule at a time and verify behavior
4. **Document Rules**: Use descriptive names and clear hint messages
5. **Share Parameters**: Define common thresholds in `parameters.json` for consistency
6. **Version Control**: Keep your `var/tagger/` configuration in version control
7. **Backup Before Changes**: Test new rules on a copy before deploying to production

## Examples

### Simple Application Detection

**File: `var/tagger/apps/python.txt`**

```
python
python3
\.py
```

This detects jobs running Python scripts.

### Complex Classification Rule

**File: `var/tagger/jobclasses/cpuImbalance.json`**

```json
{
  "name": "CPU Load Imbalance",
  "tag": "cpu_imbalance",
  "parameters": ["core_load_imbalance_threshold_factor"],
  "metrics": ["cpu_load"],
  "requirements": ["job.numCores > 1", "job.duration > 600"],
  "variables": [
    {
      "name": "load_variance",
      "expr": "(cpu_load.max - cpu_load.min) / cpu_load.avg"
    }
  ],
  "rule": "load_variance > core_load_imbalance_threshold_factor",
  "hint": "CPU load varies by {{printf \"%.1f%%\" (load_variance * 100)}} across cores"
}
```

This detects jobs where CPU load is unevenly distributed across cores.

## Reference

### Configuration Options

**Main Configuration (`config.json`)**:

- `enable-job-taggers` (boolean, default: `false`) - Enables automatic job tagging system
  - Must be set to `true` to activate automatic tagging on job start/stop events
  - Does not affect the `-apply-tags` command line option

**Command Line Options**:

- `-apply-tags` - Apply all tagging rules to existing jobs in the database
  - Works independently of `enable-job-taggers` configuration
  - Useful for retroactively tagging jobs or re-evaluating with updated rules

### Default Configuration Location

The example configurations are provided in:

- `configs/tagger/apps/` - Example application patterns (16 applications)
- `configs/tagger/jobclasses/` - Example classification rules (3 rules)

Copy these to `var/tagger/` and customize for your environment.

### Tag Types

- `app` - Application tags (e.g., "vasp", "gromacs")
- `jobClass` - Classification tags (e.g., "lowutilization", "highload")

Tags can be queried and filtered in the ClusterCockpit UI and API.
