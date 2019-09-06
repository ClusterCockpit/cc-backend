# HPCJobDatabase
A standardized interface and reference implementation for HPC job data.
The DB and json schema specification is available in the [wiki](https://github.com/RRZE-HPC/HPCJobDatabase/wiki).

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
       --hasprofile <true|false>  Only show jobs with timerseries metric data
       --mode <mode>  Set the operation mode
       --user <user_id> Search for jobs of specific user
       --project <project_id> Search for jobs of specific project
       --numnodes <from> <to>  Specify range for number of nodes of job
       --starttime <from> <to>  Specify range for start time of jobs
       --duration <from> <to>  Specify duration range of jobs
       --mem_used <from> <to>  Specify range for average main memory capacity of job
       --mem_bandwidth <from> <to>  Specify range for average main memory bandwidth of job
       --flops_any <from> <to>  Specify range for average flop any rate of job

Options:
    --help Show a brief help information.

    --man Read the manual, with examples

    --hasprofile [true|false] Only show jobs with or without timerseries
    metric data

    --mode [ids|query|count|list|stat|perf] Specify output mode. Mode can be
    one of:

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

Query jobs with conditions:

```
[HPCJobDatabase] ./acQuery.pl   --duration 20h 24h  --starttime 01.08.2018/12:00 01.03.2019/12:00
COUNT 6476
```

Query jobs from alternative database file (default is jobDB):

```
[HPCJobDatabase] ./acQuery.pl  --project project_30   --starttime 01.08.2018/12:00 01.03.2019/12:00 -- jobDB-anon-emmy
COUNT 21560
```

Get job statistics output:

```
[HPCJobDatabase] ./acQuery.pl --project project_30  --mode stat --duration 0 20h  --starttime 01.08.2018/12:00 01.03.2019/12:00  -- jobDB-anon-emmy
=================================
Job count: 747
Total walltime [h]: 16334 
Total node hours [h]: 78966 

Histogram: Number of nodes
nodes   count
1       54      ****
2       1
3       1
4       36      ****
5       522     *******
6       118     *****
7       15      ***

Histogram: Walltime
hours   count
20      250     ******
21      200     ******
22      114     *****
23      183     ******
```

Get job performance statistics:

```
[HPCJobDatabase] ./acQuery.pl --project project_30  --mode perf --duration 0 20h --numnodes 1 4  --starttime 01.08.2018/12:00 01.03.2019/12:00  -- jobDB-anon-emmy
=================================
Job count: 92
Jobs with performance profile: 48
Total walltime [h]: 2070 
Total node hours [h]: 4332 

Histogram: Mem used
Mem     count
2       3       **
3       4       **
18      2       *
19      3       **
20      2       *
21      1
22      2       *
23      5       **
24      2       *
25      1
26      1
27      3       **
29      1
30      2       *
31      1
34      1
35      1
36      1
41      1
42      2       *
43      2       *
44      1
49      1
50      2       *
51      1
52      1
53      1

Histogram: Memory bandwidth
BW      count
1       1
2       9       ***
3       1
4       1
5       4       **
6       2       *
7       10      ***
8       9       ***
9       11      ***

Histogram: Flops any
flops   count
1       3       **
2       1
3       4       **
4       3       **
5       9       ***
6       10      ***
7       11      ***
85      1
225     1
236     1
240     2       *
244     2       *
```
