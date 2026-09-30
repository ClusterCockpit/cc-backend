# footprint-queries Specification

## Purpose

Defines how job queries that filter, sort or aggregate on footprint and energy
values are answered, given that running jobs have live values and finished jobs
have persisted ones, and rejects queries that would mix the two.

## Requirements

### Requirement: Footprint features
A job query SHALL be treated as using a footprint feature when it contains any of:
a sort on a footprint field, a sort on the `energy` column, a `metricStats`
filter, an `energy` filter, or a request for metric histograms. All other queries
SHALL be answered as before this change, regardless of their state filter.

#### Scenario: Mixed-state query without footprint features
- **WHEN** a client lists jobs of all states sorted by start time without any
  footprint or energy filter
- **THEN** the query is answered as before, including running and finished jobs

### Requirement: Effective state set
The effective state set of a query SHALL be the intersection of the state
filters of all its filter entries. A query with no state filter in any entry
SHALL have as its effective state set every job state except `running` when it
uses a footprint feature. In this spec, "finished jobs" means jobs in any state
other than `running`.

#### Scenario: Intersection of state filters
- **WHEN** a query has one filter entry with states `running` and `failed` and
  another with state `running`
- **THEN** its effective state set is `running` only

#### Scenario: No state filter with a footprint feature
- **WHEN** a query sorted by a footprint field has no state filter
- **THEN** only finished jobs are returned

### Requirement: Finished-job footprint queries
When the effective state set does not contain `running`, footprint features
SHALL be evaluated on the persisted footprint and energy values.

#### Scenario: Filter finished jobs by footprint
- **WHEN** a client filters completed and failed jobs by `flops_any_avg < 100`
- **THEN** the result contains exactly the finished jobs whose persisted
  `flops_any_avg` is below 100

### Requirement: Running-job footprint queries
When the effective state set is exactly `running`, footprint features SHALL be
evaluated on the live values defined by the `running-job-footprint` capability.
Job lists, job counts, grouped statistics and histograms for the same filter
SHALL agree with each other: the job count SHALL equal the number of jobs
reachable by paging through the list, and grouped statistics SHALL cover exactly
those jobs.

#### Scenario: Filter running jobs by footprint
- **WHEN** a client filters running jobs by `flops_any_avg < 100`
- **THEN** the result contains exactly the running jobs whose live
  `flops_any_avg` is below 100, and the reported count equals the number of
  those jobs

#### Scenario: Sort running jobs by footprint
- **WHEN** a client lists running jobs sorted by `mem_bw_avg` descending
- **THEN** the jobs are ordered by their live `mem_bw_avg` value, highest first

#### Scenario: Sort running jobs by energy
- **WHEN** a client lists running jobs sorted by total energy
- **THEN** the jobs are ordered by their live total energy

#### Scenario: Grouped statistics with a footprint filter
- **WHEN** the user list view requests per-user statistics for running jobs
  filtered by `flops_any_avg < 100`
- **THEN** each user's job count equals the number of that user's running jobs
  whose live `flops_any_avg` is below 100

### Requirement: Mixed-state footprint queries are rejected
When a query uses a footprint feature and its effective state set contains
`running` together with at least one other state, the system SHALL reject the
query with an error stating that footprint queries cannot mix running and
finished jobs. This SHALL apply to GraphQL and REST alike.

#### Scenario: Running and completed with footprint sort
- **WHEN** a client lists jobs with states `running` and `completed` sorted by a
  footprint field
- **THEN** the request fails with the mixed-state error and no jobs are returned

### Requirement: Jobs without a footprint value
In footprint filters, a job without a value for the filtered metric (below the
short-job cutoff, no metric data, or not yet archived) SHALL NOT match any range.
In footprint sorts, such jobs SHALL be ordered the same way in the running and
finished paths: first in ascending order and last in descending order.

#### Scenario: Short running job and a footprint filter
- **WHEN** running jobs are filtered by `flops_any_avg <= 1000000` and one job
  is below the short-job cutoff
- **THEN** that job is not part of the result

### Requirement: Deterministic paging of running-job footprint sorts
Running-job footprint sorts SHALL break ties by job id so that the order is
total. Within the freshness interval, consecutive pages of the same query SHALL
neither repeat nor skip jobs.

#### Scenario: Paging with equal values
- **WHEN** several running jobs have the same live sort value and a client
  fetches two consecutive pages within the freshness interval
- **THEN** every job appears on at most one page and none of the matching jobs is
  skipped

### Requirement: Access control for live computation
Live values for a footprint query SHALL be computed only for jobs the requesting
user is allowed to see.

#### Scenario: Regular user sorts running jobs
- **WHEN** a user without admin or support role sorts running jobs by a
  footprint field
- **THEN** live values are computed only for that user's own jobs

### Requirement: Running-job metric histograms
Metric histograms for running jobs SHALL bin the same live footprint values that
footprint filters use, using the footprint statistic configured for each metric.

#### Scenario: Histogram uses the footprint statistic
- **WHEN** `mem_bw` is configured with footprint statistic `avg` and the
  running-jobs view shows a histogram of `mem_bw` for a multi-node job
- **THEN** that job is binned by its live `mem_bw_avg` footprint value (the mean
  over its nodes), not by the sum of its node averages

#### Scenario: Histogram covers the same jobs as the filter
- **WHEN** the running-jobs view shows a histogram of `flops_any` for a filter
- **THEN** the histogram's total job count equals the number of jobs in that
  filter that have a live `flops_any` footprint value not above the metric's peak
