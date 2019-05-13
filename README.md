# HPCJobDatabase
A standardized interface and reference implementation for HPC job data

# Dependencies

 * Getopt::Long
 * Pod::Usage
 * DateTime::Format::Strptime
 * DBD::SQLite

# Setup

```
sqlite3 jobDB < initDB.sql
```

# Helper Scripts

For all scripts apart from `acQuery.pl` the advice *use the source Luke* holds.

Help text for acQuery:
```
Usage:
       acQuery.pl [options] -- <DB file>

       Help Options:
       --help  Show help text
       --man   Show man page
       --mode <mode>  Set the operation mode
       --user <user_id> Search for jobs of specific user
       --project <project_id> Search for jobs of specific project
       --duration <from> <to>  Specify duration range of jobs
       --numnodes <from> <to>  Specify range for number of nodes of job
       --starttime <from> <to>  Specify range for start time of jobs
       --mem_used <from> <to>  Specify range for average main memory capacity of job
       --mem_bandwidth <from> <to>  Specify range for average main memory bandwidth of job
       --flops_any <from> <to>  Specify range for average flop any rate of job

Options:
    --help Show a brief help information.
    --man Read the manual, with examples
    --mode [012] Specify output mode. Mode can be one of:

            ids - Print list of job ids matching conditions. One job id per
            line.
            query - Print the query string and then exit.
            count - Only output the number of jobs matching the conditions.
            (Default mode)
            list - Output a record of every job matching the conditions.
            stat - Output job statistic for all jobs matching the
            conditions.
            perf - Output job performance footprint statistic for all jobs
            matching the conditions.

    --user Search job for a specific user id.
    
    --project Search job for a specific project.
    
    --duration Specify condition for job duration. This option takes two
    arguments: If both arguments are positive integers the condition is
    duration between first argument and second argument. If the second
    argument is zero condition is duration smaller than first argument. If
    first argument is zero condition is duration larger than second
    argument. Duration can be in seconds, minutes (append m) or hours
    (append h).
    
    --numnodes Specify condition for number of node range of job. This
    option takes two arguments: If both arguments are positive integers the
    condition is number of nodes between first argument and second argument.
    If the second argument is zero condition is number of nodes smaller than
    first argument. If first argument is zero condition is number of nodes
    larger than second argument.
    
    --starttime Specify condition for the starttime of job. This option
    takes two arguments: If both arguments are positive integers the
    condition is start time between first argument and second argument. If
    the second argument is zero condition is start time smaller than first
    argument. If first argument is zero condition is start time larger than
    second argument. Start time must be given as date in the following
    format: %d.%m.%Y/%H:%M.
    
    --mem_used Specify condition for average main memory capacity used by
    job. This option takes two arguments: If both arguments are positive
    integers the condition is memory used is between first argument and
    second argument. If the second argument is zero condition is memory used
    is smaller than first argument. If first argument is zero condition is
    memory used is larger than second argument.
    
    --mem_bandwidth Specify condition for average main memory bandwidth used
    by job. This option takes two arguments: If both arguments are positive
    integers the condition is memory bandwidth is between first argument and
    second argument. If the second argument is zero condition is memory
    bandwidth is smaller than first argument. If first argument is zero
    condition is memory bandwidth is larger than second argument.
    
    --flops_any Specify condition for average flops any of job. This option
    takes two arguments: If both arguments are positive integers the
    condition is flops any is between first argument and second argument. If
    the second argument is zero condition is flops any is smaller than first
    argument. If first argument is zero condition is flops any is larger
    than second argument.
```

# Examples 
