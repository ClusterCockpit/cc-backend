# Spec Delta

## Purpose

Lets the system query, aggregate and archive metrics measured per device attached to a node (accelerators, filesystem mount points, network interfaces), so that each device's data can be told apart and still be summarized per node.

## ADDED Requirements

### Requirement: Device scopes are native metric scopes
The system SHALL accept `accelerator`, `filesystem` and `network` as the native scope of a metric in the cluster configuration. It SHALL accept them wherever a requested metric scope is an input, including the GraphQL `scopes` arguments of job and node metric queries. A metric with one of these native scopes is a device metric. A device metric SHALL NOT be converted to or from any CPU scope (`hwthread`, `core`, `memoryDomain`, `socket`).

#### Scenario: Filesystem metric configured
- **WHEN** a cluster configuration declares a metric `fs_read_bw` with scope `filesystem`
- **THEN** the configuration is accepted and the metric is treated as a device metric

#### Scenario: Network scope requested through GraphQL
- **WHEN** a client requests job metrics with `scopes: [network]`
- **THEN** the request is accepted and not rejected as an invalid scope

### Requirement: Device instances come from the declaration
For a filesystem or network metric, the system SHALL query exactly the instances declared for that device type in the topology of the subcluster the node belongs to (`topology.filesystems`, `topology.networks`), identified by their `id`. For an accelerator metric of a job, the system SHALL query the accelerators allocated to the job on each node. For an accelerator metric of a node outside a job context, it SHALL query all accelerators declared in the topology. Each returned device series SHALL carry the instance id as its series id and the node as its hostname.

#### Scenario: Declared mount points are queried
- **WHEN** a subcluster declares filesystems `/home` and `/scratch`, and a job on two of its nodes requests `fs_read_bw` at scope `filesystem`
- **THEN** the result holds up to four series, one per node and mount point, each with the mount point as its id

#### Scenario: Undeclared instance is not queried
- **WHEN** the metric store holds data for mount point `/tmp` but the subcluster declares only `/home` and `/scratch`
- **THEN** no series for `/tmp` is returned, and `/tmp` does not contribute to the node total

#### Scenario: Job accelerators are the allocated ones
- **WHEN** a job was allocated accelerator `0` of a node that has accelerators `0` to `3`, and the job requests an accelerator metric at scope `accelerator`
- **THEN** only the series of accelerator `0` is returned for that node

### Requirement: Device metrics at their own scope
When the requested scope equals the device metric's native scope, the system SHALL return one series per queried node and instance, without aggregation.

#### Scenario: Per-mount series
- **WHEN** a job requests `fs_read_bw` at scope `filesystem`
- **THEN** each series holds the unaggregated data of one mount point on one node

### Requirement: Device metrics aggregate to node scope
When node scope is requested for a device metric, the system SHALL return one series per node. That series SHALL aggregate the node's queried instances according to the metric's configured aggregation: `sum` adds the instances, and `avg` averages them.

#### Scenario: Node total of a filesystem metric
- **WHEN** a node's `/home` reads 1 GB/s and its `/scratch` reads 2 GB/s, `fs_read_bw` has aggregation `sum`, and node scope is requested
- **THEN** the node series for that node reads 3 GB/s

### Requirement: Other requested scopes yield no device data
For a device metric, a request for any scope other than its native scope or node scope SHALL yield no series and SHALL NOT be an error. This covers every CPU scope and every other device scope. For a metric whose native scope is not a device scope, a request for a device scope SHALL yield no series and SHALL NOT be an error. In particular, it SHALL NOT fall back to the metric's native scope.

#### Scenario: CPU scope requested for a filesystem metric
- **WHEN** a job requests `fs_read_bw` at scope `core`
- **THEN** no `fs_read_bw` series is returned and the request succeeds

#### Scenario: Different device scope requested
- **WHEN** a job requests `fs_read_bw` at scope `accelerator`
- **THEN** no `fs_read_bw` series is returned and the request succeeds

#### Scenario: Device scope requested for a CPU metric
- **WHEN** a job requests `flops_any`, whose native scope is `hwthread`, at scope `accelerator` or `filesystem`
- **THEN** no `flops_any` series is returned for that scope, in particular no hardware-thread series

### Requirement: Missing device data is not an error
When a subcluster declares no instances for a device metric's scope, the system SHALL return no series for that metric at that scope and at node scope, and SHALL NOT report an error. When a declared instance has no data for a metric, for example a metric that only one filesystem type provides, the system SHALL omit that instance's series and return the others.

#### Scenario: Subcluster without filesystems
- **WHEN** a job runs on a subcluster that declares no filesystems and requests `fs_read_bw` at scopes `node` and `filesystem`
- **THEN** no `fs_read_bw` series is returned and the request succeeds

#### Scenario: Metric only one filesystem type provides
- **WHEN** a subcluster declares `/home` (nfs) and `/scratch` (lustre), and only `/scratch` has data for `fs_open`
- **THEN** the request for `fs_open` at scope `filesystem` returns the `/scratch` series and no error

### Requirement: Node views use the node's subcluster
Node metric queries (node view, node list, systems view) SHALL resolve device instances from the topology of each node's subcluster. This SHALL work both when the subcluster is given and when it is looked up per node.

#### Scenario: Mixed subclusters in a node list
- **WHEN** a node list covers nodes of two subclusters that declare different filesystems, and requests a filesystem metric at scope `filesystem`
- **THEN** each node's series are the mount points declared for that node's own subcluster

### Requirement: Archived jobs include device scopes
When a job is archived, the system SHALL store device metrics at node scope. For the filesystem and network scopes, it SHALL also store them at the device scope whenever the job's subcluster declares instances of that type. This SHALL hold regardless of the number of nodes of the job. The job's metric statistics and footprint SHALL be derived from the node-scope series. Device-scope data SHALL be stored as an ordinary scope entry of the metric in the job data, next to its node scope.

#### Scenario: Large job keeps per-mount data
- **WHEN** a job on 64 nodes of a subcluster that declares filesystems is archived
- **THEN** the archived job data holds `fs_read_bw` at scopes `node` and `filesystem`, and the job's `fs_read_bw` statistics describe the node series

#### Scenario: Large GPU job does not archive CPU-scope data
- **WHEN** a job on 16 nodes with accelerators is archived
- **THEN** metrics whose native scope is `hwthread` are archived at node scope only, and accelerator metrics at scopes `node` and `accelerator`

### Requirement: Job data holds no metric groups
Archived job data and job statistics SHALL consist only of named metrics, each mapping scopes to data. A job data document with a top-level array value, such as the former `filesystems` array, SHALL be rejected with a decode error.

#### Scenario: Legacy filesystems array
- **WHEN** an archive backend reads job data with a top-level `"filesystems": [ ... ]` entry
- **THEN** loading that job's data fails with a decode error
