# scope-aggregation Specification

## Purpose

Defines how the series of a metric are combined from its native scope into a coarser requested scope: which sources each combined series covers, and which id it carries, so that every series can be attributed to the core, socket or node it stands for.

## Requirements

### Requirement: Aggregated series carry the id of their scope
When the series of a metric are combined into a coarser requested scope, each resulting series SHALL carry the id of the scope instance it represents: no id at node scope, the core id at core scope, and the socket id at socket scope. It SHALL NOT carry the id of one of its sources. This SHALL hold for every native scope, including device metrics aggregated to node scope.

#### Scenario: Node series of a filesystem metric
- **WHEN** a job requests `fs_read_bw`, native scope `filesystem`, at scope `node` on a node that declares `/home` and `/scratch`
- **THEN** the node series of that node carries no id

#### Scenario: Node series of an accelerator metric
- **WHEN** a job with accelerators `0` and `1` on a node requests an accelerator metric at scope `node`
- **THEN** the node series of that node carries no id

#### Scenario: Hardware-thread metric at core scope
- **WHEN** a node has cores `0` to `3` with two hardware threads each (core `1` = hardware threads `2` and `3`), and a job on the whole node requests a `hwthread` metric at scope `core`
- **THEN** the result holds four series with ids `0`, `1`, `2` and `3`, and the series with id `1` combines hardware threads `2` and `3`

#### Scenario: Hardware-thread metric at socket scope
- **WHEN** a node has sockets `0` (hardware threads `0` to `3`) and `1` (hardware threads `4` to `7`), and a job on the whole node requests a `hwthread` metric at scope `socket`
- **THEN** the result holds two series with ids `0` and `1`

#### Scenario: Memory-domain metric at socket scope
- **WHEN** a job requests a `memoryDomain` metric at scope `socket` on a node whose memory domains `0` and `1` belong to sockets `0` and `1`
- **THEN** each series carries the id of its socket, not of a memory domain

### Requirement: Unaggregated series keep their source id
When a metric is returned at its native scope, each series SHALL carry the id of its source instance: the hardware thread, core, memory domain, socket, or device instance.

#### Scenario: Hardware-thread metric at its native scope
- **WHEN** a job on hardware threads `0` to `3` requests a `hwthread` metric at scope `hwthread`
- **THEN** the result holds four series with ids `0`, `1`, `2` and `3`

#### Scenario: Filesystem metric at its native scope
- **WHEN** a job requests `fs_read_bw` at scope `filesystem` on a node that declares `/home` and `/scratch`
- **THEN** the two series carry the ids `/home` and `/scratch`

### Requirement: Core metrics aggregate to socket scope by their cores
When a metric whose native scope is `core` is requested at scope `socket`, the system SHALL return one series per socket that holds at least one of the job's cores. Each series SHALL combine exactly the cores of that socket, identified as cores, not as hardware threads.

#### Scenario: Core metric at socket scope on a node with SMT
- **WHEN** a node has cores `0` and `1` on socket `0` and cores `2` and `3` on socket `1`, each core with two hardware threads, and a job on the whole node requests a `core` metric at scope `socket`
- **THEN** the result holds two series: id `0` combining cores `0` and `1`, and id `1` combining cores `2` and `3`

#### Scenario: Job on one socket
- **WHEN** a job holds only the hardware threads of socket `0` on that node and requests a `core` metric at scope `socket`
- **THEN** the result holds one series, with id `0`, combining cores `0` and `1`

### Requirement: Ids are the same on every path
The ids of aggregated and unaggregated series SHALL be the same in job metric data, scoped job statistics and node-list metric data, and for both the internal metric store and an external cc-metric-store.

#### Scenario: Scoped job statistics at core scope
- **WHEN** the scoped statistics of the job from "Hardware-thread metric at core scope" are requested at scope `core`
- **THEN** the statistics entries carry the ids `0`, `1`, `2` and `3`

#### Scenario: Node list of a filesystem metric
- **WHEN** a node list requests `fs_read_bw` at scopes `node` and `filesystem`
- **THEN** each node's node-scope series carries no id, and its filesystem series carry the mount points

### Requirement: Archived jobs keep their stored ids
Series ids SHALL be stored with the job data when a job is archived. Loading an archived job SHALL return the ids it was archived with, whatever rule produced them.

#### Scenario: Job archived before the change
- **WHEN** a job archived with the id of a source on its node-scope series is loaded
- **THEN** the series carries that stored id unchanged
