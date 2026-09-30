# Spec Delta

## Purpose

Provides the footprint, energy footprint and total energy of running jobs by
computing them on demand from the metric store, so that users and API clients
always see current values without a background worker persisting them.

## ADDED Requirements

### Requirement: Live footprint for running jobs
The system SHALL compute the footprint of a running job on demand from the
metric store that serves the job's cluster and subcluster. The footprint SHALL
contain one value per footprint metric configured for the job's subcluster, keyed
`<metric>_<stat>`, where `<stat>` is the footprint statistic (`avg`, `min` or
`max`) configured for that metric, with a subcluster-specific setting taking
precedence over the global metric setting. The computation SHALL cover the time
range from the job's start time to the time of the request. Both the internal
metric store and an external cc-metric-store SHALL be supported.

#### Scenario: Footprint of a running job is current
- **WHEN** a client requests the footprint of a running job that has run longer
  than the short-job cutoff
- **THEN** the system returns footprint values computed from metric data up to
  the time of the request, subject to the freshness bound

#### Scenario: Subcluster-specific statistic is honoured
- **WHEN** a metric is configured with footprint statistic `avg` globally and
  `max` for the job's subcluster
- **THEN** the running job's footprint contains `<metric>_max` and not
  `<metric>_avg`

### Requirement: Footprint statistic aggregation across nodes
The job-level footprint value SHALL be derived from the per-node statistics of
the job's nodes: `avg` SHALL be the mean of the node averages over the job's
nodes, `min` SHALL be the smallest node minimum, and `max` SHALL be the largest
node maximum. Values SHALL be rounded to two decimal places. Nodes without data
for a metric SHALL NOT contribute to its `min` or `max`.

#### Scenario: Minimum of positive values
- **WHEN** a running job has two nodes whose minima for a metric are 12.5 and 8.25
- **THEN** the `min` footprint value for that metric is 8.25, not 0

#### Scenario: Average over nodes
- **WHEN** a running job has two nodes whose averages for a metric are 100 and 50
- **THEN** the `avg` footprint value for that metric is 75

### Requirement: Live energy footprint and total energy for running jobs
The system SHALL compute the energy footprint and total energy of a running job on
demand. For each energy metric of the job's subcluster that is configured as a
power metric, the energy in kWh SHALL be the job's average power per node
multiplied by the number of nodes and the elapsed run time in hours, divided by
1000, rounded to two decimal places. The total energy SHALL be the sum over all
energy metrics. Energy metrics configured as `energy` (not power) SHALL continue
to report 0.

#### Scenario: Energy grows with run time
- **WHEN** a client requests the total energy of the same running job twice,
  more than the freshness bound apart, while the job draws constant non-zero
  power
- **THEN** the second value is larger than the first

#### Scenario: Energy footprint is present for running jobs
- **WHEN** a client requests the energy footprint of a running job on a
  subcluster with power metrics configured for energy
- **THEN** the response contains one non-null entry per configured energy metric

### Requirement: Short-job cutoff
The system SHALL NOT compute a live footprint, energy footprint or total energy for
a running job whose elapsed run time is shorter than the configured
`main.short-running-jobs-duration`. For such a job the footprint and energy
footprint SHALL be empty and the total energy SHALL be 0; they SHALL NOT be
reported as zero-valued metric entries.

#### Scenario: Job below the cutoff
- **WHEN** a job has been running for 2 minutes and the cutoff is 5 minutes
- **THEN** its footprint and energy footprint are empty and no metric-store
  request is made for it

#### Scenario: Job crosses the cutoff
- **WHEN** the same job has been running for 6 minutes
- **THEN** its footprint is computed live

### Requirement: Freshness bound
Live values SHALL be recomputed at most once per job within a freshness interval
of 60 seconds. All requests within that interval SHALL observe the same values
for a job, so that values used for sorting and filtering match the values
displayed.

#### Scenario: Concurrent requests share one computation
- **WHEN** several requests for the footprint of the same running job arrive
  within the freshness interval
- **THEN** the metric store is queried at most once for that job and all
  requests receive identical values

### Requirement: Degradation when the metric store fails
When the metric store cannot deliver statistics for a running job, the system
SHALL return an empty footprint and energy footprint and a total energy of 0 for that
job, SHALL log the failure, and SHALL NOT fail the surrounding request. Metrics
for which only some data is missing SHALL be omitted rather than reported as 0.

#### Scenario: Metric store unavailable while listing jobs
- **WHEN** a client lists running jobs including their footprint and the metric
  store is unreachable
- **THEN** the job list is returned with empty footprints and no GraphQL or HTTP
  error is raised for the footprint field

### Requirement: Jobs between stop and archiving
A job that has stopped but whose archiving has not finished SHALL be displayed with
a live footprint, energy footprint and total energy until the archiver has
persisted the final values.

#### Scenario: Viewing a job right after it stopped
- **WHEN** a client views a job that stopped a few seconds ago and whose
  archiving is still in progress
- **THEN** the job's footprint is shown with values computed from the metric
  store rather than empty

### Requirement: Final values persisted at archiving only
The system SHALL NOT persist footprint, energy footprint or total energy values
while a job is running. The final values SHALL be persisted when the job is
archived, computed from the archived job data, as before this change.

#### Scenario: No background writes for running jobs
- **WHEN** jobs are running and no job stops
- **THEN** the persisted footprint and energy columns of those jobs are not
  updated

#### Scenario: Final footprint after archiving
- **WHEN** a job has been stopped and archived
- **THEN** its footprint is served from the persisted values and not from the
  metric store

### Requirement: Exposure through GraphQL and REST
The GraphQL `Job` fields `footprint`, `energyFootprint` and `energy`, and the job
representations returned by the REST endpoints `GET /api/jobs/`,
`GET /api/jobs/{id}` and `POST /api/jobs/{id}`, SHALL carry the live values for
running jobs and the persisted values for finished jobs. GraphQL SHALL compute live
values only when one of these fields is requested.

#### Scenario: REST listing of running jobs
- **WHEN** an API client calls `GET /api/jobs/?state=running`
- **THEN** every returned job above the short-job cutoff carries a non-empty
  `footprint` computed live

#### Scenario: GraphQL query without footprint fields
- **WHEN** a GraphQL client lists running jobs without selecting `footprint`,
  `energyFootprint` or `energy`
- **THEN** no live footprint computation is triggered for those jobs

### Requirement: Deprecated footprint worker configuration
The `cron.footprint-worker` configuration key SHALL still be accepted so that
existing configurations load unchanged. It SHALL have no effect, and the system
SHALL log a deprecation warning at startup when it is set.

#### Scenario: Existing configuration with footprint-worker
- **WHEN** cc-backend starts with `"cron": {"footprint-worker": "10m"}`
- **THEN** the configuration loads without error, a deprecation warning is
  logged, and no footprint update task is scheduled
