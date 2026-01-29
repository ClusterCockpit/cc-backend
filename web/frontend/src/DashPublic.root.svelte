<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
  } from "./generic/utils.js";
  import {
    formatNumber,
    scaleNumber
  } from "./generic/units.js";
  import {
    Row,
    Col,
    Card,
    CardHeader,
    CardBody,
    Spinner,
    Table,
    Progress,
    Icon,
    Button,
    Badge
  } from "@sveltestrap/sveltestrap";
  import Roofline from "./generic/plots/Roofline.svelte";
  import Pie, { colors } from "./generic/plots/Pie.svelte";
  import Stacked from "./generic/plots/Stacked.svelte";
  import DoubleMetric from "./generic/plots/DoubleMetricPlot.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
  } = $props();

  /*Const Init */
  const { query: initq } = init();
  const client = getContextClient();
  // const useCbColors = getContext("cc-config")?.plotConfiguration_colorblindMode || false

  /* States */
  let from = $state(new Date(Date.now() - (5 * 60 * 1000)));
  let clusterFrom = $state(new Date(Date.now() - (8 * 60 * 60 * 1000)));
  let to = $state(new Date(Date.now()));
  let stackedFrom = $state(Math.floor(Date.now() / 1000) - 14400);
  let colWidthStates = $state(0);

  /* Derived */
  // States for Stacked charts
  const statesTimed = $derived(queryStore({
    client: client,
    query: gql`
      query ($filter: [NodeFilter!], $type: String!) {
        nodeStatesTimed(filter: $filter, type: $type) {
          state
          counts
          times
        }
      }
    `,
    variables: {
      filter: { cluster: { eq: presetCluster }, timeStart: stackedFrom},
      type: "node",
    },
    requestPolicy: "network-only"
  }));

  // Note: nodeMetrics are requested on configured $timestep resolution
  // Result: The latest 5 minutes (datapoints) for each node independent of job
  const statusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $cluster: String!
        $nmetrics: [String!]
        $cmetrics: [String!]
        $from: Time!
        $to: Time!
        $clusterFrom: Time!
        $jobFilter: [JobFilter!]!
        $nodeFilter: [NodeFilter!]!
        $paging: PageRequest!
        $sorting: OrderByInput!
      ) {
        # Node 5 Minute Averages for Roofline
        nodeMetrics(
          cluster: $cluster
          metrics: $nmetrics
          from: $from
          to: $to
        ) {
          host
          subCluster
          metrics {
            name
            metric {
              unit {
                base
                prefix
              }
              series {
                statistics {
                  avg
                }
              }
            }
          }
        }
        # Get Jobs for Per-Node Counts
        jobs(filter: $jobFilter, order: $sorting, page: $paging) {
          items {
            jobId
            resources {
              hostname
            }
          }
          count
        }
        # Only counts shared nodes once 
        allocatedNodes(cluster: $cluster) {
          name
          count
        }
        # Get Current States fir Pie Charts
        nodeStates(filter: $nodeFilter) {
          state
          count
        }
        # Get States for Node Roofline; $sorting unused in backend: Use placeholder
        nodes(filter: $nodeFilter, order: $sorting) {
          count
          items {
            hostname
            cluster
            subCluster
            schedulerState
          }
        }
        # totalNodes includes multiples if shared jobs: Info-Card Data
        jobsStatistics(
          filter: $jobFilter
          page: $paging
          sortBy: TOTALJOBS
          groupBy: CLUSTER
        ) {
          id
          totalJobs
          totalUsers
          totalCores
          totalAccs
        }
        # ClusterMetrics for doubleMetricPlot
        clusterMetrics(
          cluster: $cluster
          metrics: $cmetrics
          from: $clusterFrom
          to: $to
        ) {
          nodeCount
          metrics {
            name
            unit {
              prefix
              base
            }
            timestep
            data
          }
        }
      }
    `,
    variables: {
      cluster: presetCluster,
      nmetrics: ["flops_any", "mem_bw", "cpu_power", "acc_power"], // Metrics For Roofline and Stats
      cmetrics: ["flops_any", "mem_bw"], // Metrics For Cluster Plot
      from: from.toISOString(),
      clusterFrom: clusterFrom.toISOString(),
      to: to.toISOString(),
      jobFilter: [{ state: ["running"] }, { cluster: { eq: presetCluster } }],
      nodeFilter: { cluster: { eq: presetCluster }},
      paging: { itemsPerPage: -1, page: 1 }, // Get all: -1
      sorting: { field: "startTime", type: "col", order: "DESC" }
    },
    requestPolicy: "network-only"
  }));

  const clusterInfo = $derived.by(() => {
    let rawInfos = {};
    if ($initq?.data?.clusters) {
      // Grouped By Cluster
      if (!rawInfos['allocatedCores']) rawInfos['allocatedCores'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == presetCluster)?.totalCores || 0;
      if (!rawInfos['allocatedAccs']) rawInfos['allocatedAccs'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == presetCluster)?.totalAccs || 0;
      if (!rawInfos['activeUsers']) rawInfos['activeUsers'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == presetCluster)?.totalUsers || 0;
      if (!rawInfos['runningJobs']) rawInfos['runningJobs'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == presetCluster)?.totalJobs || 0;

      // Collected By Subcluster
      let subClusters = $initq?.data?.clusters?.find((c) => c.name == presetCluster)?.subClusters || [];
      for (let subCluster of subClusters) {
        // Allocations
        if (!rawInfos['allocatedNodes']) rawInfos['allocatedNodes'] = $statusQuery?.data?.allocatedNodes?.find(({ name }) => name == subCluster.name)?.count || 0;
        else rawInfos['allocatedNodes'] += $statusQuery?.data?.allocatedNodes?.find(({ name }) => name == subCluster.name)?.count || 0;

        // Infos
        if (!rawInfos['processorTypes']) rawInfos['processorTypes'] = subCluster?.processorType ? new Set([subCluster.processorType]) : new Set([]);
        else rawInfos['processorTypes'].add(subCluster.processorType);

        if (!rawInfos['totalNodes']) rawInfos['totalNodes'] = subCluster?.numberOfNodes || 0;
        else rawInfos['totalNodes'] += subCluster?.numberOfNodes || 0;

        if (!rawInfos['totalCores']) rawInfos['totalCores'] = (subCluster?.socketsPerNode * subCluster?.coresPerSocket * subCluster?.numberOfNodes) || 0;
        else rawInfos['totalCores'] += (subCluster?.socketsPerNode * subCluster?.coresPerSocket * subCluster?.numberOfNodes) || 0;

        if (!rawInfos['totalAccs']) rawInfos['totalAccs'] = (subCluster?.numberOfNodes * subCluster?.topology?.accelerators?.length) || 0;
        else rawInfos['totalAccs'] += (subCluster?.numberOfNodes * subCluster?.topology?.accelerators?.length) || 0;
          
        // Units (Set Once)
        if (!rawInfos['flopRateUnitBase']) rawInfos['flopRateUnitBase'] = subCluster.flopRateSimd.unit.base
        if (!rawInfos['memBwRateUnitBase']) rawInfos['memBwRateUnitBase'] = subCluster.memoryBandwidth.unit.base
        if (!rawInfos['flopRateUnitPrefix']) rawInfos['flopRateUnitPrefix'] = subCluster.flopRateSimd.unit.prefix
        if (!rawInfos['memBwRateUnitPrefix']) rawInfos['memBwRateUnitPrefix'] = subCluster.memoryBandwidth.unit.prefix

        // Get Maxima For Roofline Knee Render
        if (!rawInfos['roofData']) {
          rawInfos['roofData'] = {
            flopRateScalar: {value: subCluster.flopRateScalar.value},
            flopRateSimd: {value: subCluster.flopRateSimd.value},
            memoryBandwidth: {value: subCluster.memoryBandwidth.value}
          };
        } else { 
          rawInfos['roofData']['flopRateScalar']['value'] = Math.max(rawInfos['roofData']['flopRateScalar']['value'], subCluster.flopRateScalar.value)
          rawInfos['roofData']['flopRateSimd']['value'] = Math.max(rawInfos['roofData']['flopRateSimd']['value'], subCluster.flopRateSimd.value)
          rawInfos['roofData']['memoryBandwidth']['value'] = Math.max(rawInfos['roofData']['memoryBandwidth']['value'], subCluster.memoryBandwidth.value)
        }
      }

      // Get Idle Infos after Sums
      if (!rawInfos['idleNodes']) rawInfos['idleNodes'] = rawInfos['totalNodes'] - rawInfos['allocatedNodes'];
      if (!rawInfos['idleCores']) rawInfos['idleCores'] = rawInfos['totalCores'] - rawInfos['allocatedCores'];
      if (!rawInfos['idleAccs']) rawInfos['idleAccs'] = rawInfos['totalAccs'] - rawInfos['allocatedAccs'];

      // Keymetrics (Data on Cluster-Scope)
      let rawFlops = $statusQuery?.data?.nodeMetrics?.reduce((sum, node) =>
        sum + (node.metrics.find((m) => m.name == 'flops_any')?.metric?.series[0]?.statistics?.avg || 0),
        0, // Initial Value
      ) || 0;
      rawInfos['flopRate'] = Math.floor((rawFlops * 100) / 100)

      let rawMemBw = $statusQuery?.data?.nodeMetrics?.reduce((sum, node) =>
        sum + (node.metrics.find((m) => m.name == 'mem_bw')?.metric?.series[0]?.statistics?.avg || 0),
        0, // Initial Value
      ) || 0;
      rawInfos['memBwRate'] = Math.floor((rawMemBw * 100) / 100)

      let rawCpuPwr = $statusQuery?.data?.nodeMetrics?.reduce((sum, node) =>
        sum + (node.metrics.find((m) => m.name == 'cpu_power')?.metric?.series[0]?.statistics?.avg || 0),
        0, // Initial Value
      ) || 0;
      rawInfos['cpuPwr'] = Math.floor((rawCpuPwr * 100) / 100)

      let rawCpuUnit = $statusQuery?.data?.nodeMetrics[0]?.metrics.find((m) => m.name == 'cpu_power')?.metric?.unit || null
      if (!rawInfos['cpuPwrUnitBase'])   rawInfos['cpuPwrUnitBase'] = rawCpuUnit ? rawCpuUnit.base : ''
      if (!rawInfos['cpuPwrUnitPrefix']) rawInfos['cpuPwrUnitPrefix'] = rawCpuUnit ? rawCpuUnit.prefix : ''

      let rawGpuPwr = $statusQuery?.data?.nodeMetrics?.reduce((sum, node) =>
        sum + (node.metrics.find((m) => m.name == 'acc_power')?.metric?.series[0]?.statistics?.avg || 0),
        0, // Initial Value
      ) || 0;
      rawInfos['gpuPwr'] = Math.floor((rawGpuPwr * 100) / 100)

      let rawGpuUnit = $statusQuery?.data?.nodeMetrics[0]?.metrics.find((m) => m.name == 'acc_power')?.metric?.unit || null
      if (!rawInfos['gpuPwrUnitBase']) rawInfos['gpuPwrUnitBase'] = rawGpuUnit ? rawGpuUnit.base : ''
      if (!rawInfos['gpuPwrUnitPrefix']) rawInfos['gpuPwrUnitPrefix'] = rawGpuUnit ? rawGpuUnit.prefix : ''
    }
    return rawInfos;
  });

  const refinedStateData = $derived.by(() => {
    return $statusQuery?.data?.nodeStates.
      filter((e) => ['allocated', 'reserved', 'idle', 'mixed','down', 'unknown'].includes(e.state)).
      sort((a, b) => b.count - a.count)
  });

  const sortedClusterMetrics = $derived($statusQuery?.data?.clusterMetrics?.metrics.sort((a, b) => b.name.localeCompare(a.name)));

  /* Functions */
  function transformNodesStatsToData(subclusterData) {
    let data = null
    const x = [], y = []

    if (subclusterData) {
      for (let i = 0; i < subclusterData.length; i++) {
        const flopsData = subclusterData[i].metrics.find((s) => s.name == "flops_any")
        const memBwData = subclusterData[i].metrics.find((s) => s.name == "mem_bw")

        const f = flopsData.metric.series[0].statistics.avg
        const m = memBwData.metric.series[0].statistics.avg

        let intensity = f / m
        if (Number.isNaN(intensity) || !Number.isFinite(intensity)) {
            intensity = 0.0 // Set to Float Zero: Will not show in Log-Plot (Always below render limit)
        }

        x.push(intensity)
        y.push(f)
      }
    } else {
        // console.warn("transformNodesStatsToData: metrics for 'mem_bw' and/or 'flops_any' missing!")
    }

    if (x.length > 0 && y.length > 0) {
        data = [null, [x, y]] // for dataformat see roofline.svelte
    }
    return data
  }

  function transformNodesStatsToInfo(subClusterData) {
    let result = [];
    if (subClusterData) { //  && $nodesState?.data) {
      // Use Nodes as Returned from CCMS, *NOT* as saved in DB via SlurmState-API!
      for (let j = 0; j < subClusterData.length; j++) {
        const nodeName = subClusterData[j]?.host ? subClusterData[j].host : "unknown"
        const nodeMatch = $statusQuery?.data?.nodes?.items?.find((n) => n.hostname == nodeName && n.subCluster == subClusterData[j].subCluster);
        const schedulerState = nodeMatch?.schedulerState ? nodeMatch.schedulerState : "notindb"
        let numJobs = 0

        if ($statusQuery?.data) {
          const nodeJobs = $statusQuery?.data?.jobs?.items?.filter((job) => job.resources.find((res) => res.hostname == nodeName))
          numJobs = nodeJobs?.length ? nodeJobs.length : 0
        }

        result.push({nodeName: nodeName, schedulerState: schedulerState, numJobs: numJobs})
      };
    };
    return result
  }

</script>

<Row>
  <Col>
    <Refresher
      hideSelector
      initially={60}
      onRefresh={(interval) => {
        from = new Date(Date.now() - 5 * 60 * 1000);
        to = new Date(Date.now());
        clusterFrom = new Date(Date.now() - (8 * 60 * 60 * 1000))

        if (interval) stackedFrom += Math.floor(interval / 1000);
        else stackedFrom += 1 // Workaround: TimeSelection not linked, just trigger new data on manual refresh
      }}
    />
  </Col>
</Row>

{#if $statusQuery.fetching || $statesTimed.fetching}
  <Row class="justify-content-center">
    <Col xs="auto">
      <Spinner />
    </Col>
  </Row>

{:else if $statusQuery.error || $statesTimed.error}
  <Row class="mb-2">
    <Col class="d-flex justify-content-end">
      <Button color="secondary" href="/">
        <Icon name="x"/>
      </Button>
    </Col>
  </Row>
  <Row cols={{xs:1, md:2}}>
    {#if $statusQuery.error}
      <Col>
        <Card color="danger"><CardBody>Error Requesting Status Data: {$statusQuery.error.message}</CardBody></Card>
      </Col>
    {/if}
    {#if $statesTimed.error}
      <Col>
        <Card color="danger"><CardBody>Error Requesting Node Scheduler States: {$statesTimed.error.message}</CardBody></Card>
      </Col>
    {/if}
  </Row>

{:else}
  <!-- View Supposed to be Viewed at Max Viewport Size -->
  <div class="align-content-center p-2">
    <Row cols={{xs:1, md:2}} style="height: 24vh; margin-bottom: 1rem;">
      <Col> <!-- General Cluster Info Card -->
          <Card class="h-100">
          <CardHeader>
            <Row>
              <Col xs="11" class="text-center">
                <h2 class="mb-0">Cluster {presetCluster.charAt(0).toUpperCase() + presetCluster.slice(1)}</h2>
              </Col>
              <Col xs="1" class="d-flex justify-content-end">
                <Button color="light" href="/">
                  <Icon name="x"/>
                </Button>
              </Col>
            </Row>
          </CardHeader>
          <CardBody>
            <h4>CPU(s)</h4><p><strong>{[...clusterInfo?.processorTypes].join(', ')}</strong></p>
          </CardBody>
          </Card>
      </Col>

      <Col> <!-- Utilization Info Card -->
        <Card class="h-100">
          <CardBody>
            <Row class="mb-1">
              <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                <Badge color="primary" style="font-size:x-large;margin-right:0.25rem;">
                  {clusterInfo?.runningJobs}
                </Badge>
                <div style="font-size:large;">
                  Running Jobs
                </div>
              </Col>
              <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                <Badge color="primary" style="font-size:x-large;margin-right:0.25rem;">
                  {clusterInfo?.activeUsers}
                </Badge>
                <div style="font-size:large;">
                  Active Users
                </div>
              </Col>
              <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                <Badge color="primary" style="font-size:x-large;margin-right:0.25rem;">
                  {clusterInfo?.allocatedNodes}
                </Badge>
                <div style="font-size:large;">
                  Active Nodes
                </div>
              </Col>
            </Row>
            <Row class="mt-1 mb-2">
              <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                <Badge color="secondary" style="font-size:x-large;margin-right:0.25rem;">
                  {scaleNumber(clusterInfo?.flopRate, clusterInfo?.flopRateUnitPrefix)}{clusterInfo?.flopRateUnitBase}
                </Badge>
                <div style="font-size:large;">
                  Total Flop Rate
                </div>
              </Col>
              <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                <Badge color="secondary" style="font-size:x-large;margin-right:0.25rem;">
                  {scaleNumber(clusterInfo?.memBwRate, clusterInfo?.memBwRateUnitPrefix)}{clusterInfo?.memBwRateUnitBase}
                </Badge>
                <div style="font-size:large;">
                  Total Memory Bandwidth
                </div>
              </Col>
              {#if clusterInfo?.totalAccs !== 0}
                <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                  <Badge color="secondary" style="font-size:x-large;margin-right:0.25rem;">
                    {scaleNumber(clusterInfo?.gpuPwr, clusterInfo?.gpuPwrUnitPrefix)}{clusterInfo?.gpuPwrUnitBase}
                  </Badge>
                  <div style="font-size:large;">
                    Total GPU Power
                  </div>
                </Col>
              {:else}
                <Col xs={4} class="d-inline-flex align-items-center justify-content-center">
                  <Badge color="secondary" style="font-size:x-large;margin-right:0.25rem;">
                    {scaleNumber(clusterInfo?.cpuPwr, clusterInfo?.cpuPwrUnitPrefix)}{clusterInfo?.cpuPwrUnitBase}
                  </Badge>
                  <div style="font-size:large;">
                    Total CPU Power
                  </div>
                </Col>
              {/if}
            </Row>
            <Row class="my-1 align-items-baseline">
              <Col xs={2} style="font-size:large;">
                Active Cores
              </Col>
              <Col xs={8}>
                <Progress multi max={clusterInfo?.totalCores} style="height:2.5rem;font-size:x-large;">
                  <Progress bar color="success" value={clusterInfo?.allocatedCores} title={`${clusterInfo?.allocatedCores} active`}>{formatNumber(clusterInfo?.allocatedCores)}</Progress>
                  <Progress bar color="light" value={clusterInfo?.idleCores} title={`${clusterInfo?.idleCores} idle`}>{formatNumber(clusterInfo?.idleCores)}</Progress>
                </Progress>
              </Col>
              <Col xs={2} style="font-size:large;">
                Idle Cores
              </Col>
            </Row>
            {#if clusterInfo?.totalAccs !== 0}
              <Row class="my-1 align-items-baseline">
                <Col xs={2} style="font-size:large;">
                  Active GPU
                </Col>
                <Col xs={8}>
                  <Progress multi max={clusterInfo?.totalAccs} style="height:2.5rem;font-size:x-large;">
                    <Progress bar color="success" value={clusterInfo?.allocatedAccs} title={`${clusterInfo?.allocatedAccs} active`}>{formatNumber(clusterInfo?.allocatedAccs)}</Progress>
                    <Progress bar color="light" value={clusterInfo?.idleAccs} title={`${clusterInfo?.idleAccs} idle`}>{formatNumber(clusterInfo?.idleAccs)}</Progress>
                  </Progress>
                </Col>
                <Col xs={2} style="font-size:large;">
                  Idle GPU
                </Col>
              </Row>
            {/if}
          </CardBody>
        </Card>
      </Col>
    </Row>

    <Row cols={{xs:1, md:2}} style="height: 34vh; margin-bottom: 1rem;">
      <!-- Total Cluster Metric in Time SUMS-->
      <Col class="text-center">
        <h5 class="mt-2 mb-0">
          Cluster Utilization (
          <span style="color: #0000ff;">
            {`${sortedClusterMetrics[0]?.name} (${sortedClusterMetrics[0]?.unit?.prefix}${sortedClusterMetrics[0]?.unit?.base})`}
          </span>,
          <span style="color: #ff0000;">
            {`${sortedClusterMetrics[1]?.name} (${sortedClusterMetrics[1]?.unit?.prefix}${sortedClusterMetrics[1]?.unit?.base})`}
          </span>
          )
        </h5>
        <div>
          {#key $statusQuery?.data?.clusterMetrics}
            <DoubleMetric
              timestep={$statusQuery?.data?.clusterMetrics[0]?.timestep || 60}
              numNodes={$statusQuery?.data?.clusterMetrics?.nodeCount || 0}
              metricData={sortedClusterMetrics || []}
              height={250}
              publicMode
            />
          {/key}
        </div>
      </Col>

      <Col> <!-- Nodes Roofline -->
        <div>
          {#key $statusQuery?.data?.nodeMetrics}
            <Roofline
              colorBackground
              useColors={false}
              useLegend={false}
              allowSizeChange
              cluster={presetCluster}
              subCluster={clusterInfo?.roofData ? clusterInfo.roofData : null}
              roofData={transformNodesStatsToData($statusQuery?.data?.nodeMetrics)}
              nodesData={transformNodesStatsToInfo($statusQuery?.data?.nodeMetrics)}
              fixTitle="Node Utilization"
              yMinimum={1.0}
              height={280}
            />
          {/key}
        </div>
      </Col>
    </Row>

    <Row cols={{xs:1, md:2}} style="height: 34vh;">
      <Col> <!-- Pie Last States -->
        <Row>
          {#if refinedStateData.length > 0}
            <Col class="px-3 mt-2 mt-lg-0">
              <div bind:clientWidth={colWidthStates}>
                {#key refinedStateData}
                  <Pie
                    canvasId="hpcpie-slurm"
                    size={colWidthStates * 0.66}
                    sliceLabel="Nodes"
                    quantities={refinedStateData.map(
                      (sd) => sd.count,
                    )}
                    entities={refinedStateData.map(
                      (sd) => sd.state,
                    )}
                    fixColors={refinedStateData.map(
                      (sd) => colors['nodeStates'][sd.state],
                    )}
                  />
                {/key}
              </div>
            </Col>
            <Col class="px-4 py-2">
              {#key refinedStateData}
                <Table>
                  <tr class="mb-2">
                    <th></th>
                    <th class="h4">State</th>
                    <th class="h4">Count</th>
                  </tr>
                  {#each refinedStateData as sd, i}
                    <tr>
                      <td><Icon name="circle-fill" style="color: {colors['nodeStates'][sd.state]}; font-size: 30px;"/></td>
                      <td class="h5">{sd.state.charAt(0).toUpperCase() + sd.state.slice(1)}</td>
                      <td class="h5">{sd.count}</td>
                    </tr>
                  {/each}
                </Table>
              {/key}
            </Col>
          {:else}
            <Col>
              <Card body color="warning" class="mx-4 my-2"
                >Cannot render state status: No state data returned for <code>Pie Chart</code></Card
              >
            </Col>
          {/if}
        </Row>
      </Col>

      <Col> <!-- Stacked SchedState -->
        <div>
          {#key $statesTimed?.data?.nodeStatesTimed}
            <Stacked
              data={$statesTimed?.data?.nodeStatesTimed}
              height={250}
              ylabel="Nodes"
              yunit = "#Count"
              title = "Cluster Status"
              stateType = "Node"
            />
          {/key}
        </div>
      </Col>
    </Row>
  </div>
{/if}
