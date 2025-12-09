<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import {
    getContext
  } from "svelte"
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    scramble,
    scrambleNames,
    convert2uplot
  } from "./generic/utils.js";
  import {
    formatDurationTime,
    formatNumber,
  } from "./generic/units.js";
  import {
    Row,
    Col,
    Card,
    CardTitle,
    CardHeader,
    CardBody,
    Spinner,
    Table,
    Progress,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import Roofline from "./generic/plots/Roofline.svelte";
  import Pie, { colors } from "./generic/plots/Pie.svelte";
  import Stacked from "./generic/plots/Stacked.svelte";
  // import Histogram from "./generic/plots/Histogram.svelte";
  import DoubleMetric from "./generic/plots/DoubleMetricPlot.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
  } = $props();

  /*Const Init */
  const { query: initq } = init();
  const client = getContextClient();
  const useCbColors = getContext("cc-config")?.plotConfiguration_colorblindMode || false

  /* States */
  let pagingState = $state({page: 1, itemsPerPage: 10}) // Top 10
  let from = $state(new Date(Date.now() - (5 * 60 * 1000)));
  let clusterFrom = $state(new Date(Date.now() - (8 * 60 * 60 * 1000)));
  let to = $state(new Date(Date.now()));
  let stackedFrom = $state(Math.floor(Date.now() / 1000) - 14400);
  let colWidthStates = $state(0);
  let colWidthRoof = $state(0);
  let colWidthTotals = $state(0);
  let colWidthStacked = $state(0);

  /* Derived */
  // States for Stacked charts
  const statesTimed = $derived(queryStore({
    client: client,
    query: gql`
      query ($filter: [NodeFilter!], $typeNode: String!, $typeHealth: String!) {
        nodeStates: nodeStatesTimed(filter: $filter, type: $typeNode) {
          state
          counts
          times
        }
        healthStates: nodeStatesTimed(filter: $filter, type: $typeHealth) {
          state
          counts
          times
        }
      }
    `,
    variables: {
      filter: { cluster: { eq: presetCluster }, timeStart: 1760096999}, // DEBUG VALUE, use StackedFrom
      typeNode: "node",
      typeHealth: "health"
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
        $metrics: [String!]
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
          metrics: $metrics
          from: $from
          to: $to
        ) {
          host
          subCluster
          metrics {
            name
            metric {
              series {
                statistics {
                  avg
                }
              }
            }
          }
        }
        # Running Job Metric Average for Rooflines
        jobsMetricStats(filter: $jobFilter, metrics: $metrics) {
          id
          jobId
          duration
          numNodes
          numAccelerators
          subCluster
          stats {
            name
            data {
              avg
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
          groupBy: SUBCLUSTER
        ) {
          id
          totalJobs
          totalUsers
          totalCores
          totalAccs
        }
        # TEST
        clusterMetrics(
          cluster: $cluster
          metrics: $metrics
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
      metrics: ["flops_any", "mem_bw"], // Fixed names for roofline and status bars
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

  const topJobsQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
      ) {
        jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALJOBS
          groupBy: PROJECT
        ) {
          id
          totalJobs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: presetCluster} }],
      paging: pagingState // Top 10
    },
    requestPolicy: "network-only"
  }));

  // Note: nodeMetrics are requested on configured $timestep resolution
  const nodeStatusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $selectedHistograms: [String!]
        $numDurationBins: String
      ) {
        jobsStatistics(filter: $filter, metrics: $selectedHistograms, numDurationBins: $numDurationBins) {
          histNumCores {
            count
            value
          }
          histNumAccs {
            count
            value
          }
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: presetCluster } }],
      selectedHistograms: [], // No Metrics requested for node hardware stats - Empty Array can be used for refresh
      numDurationBins: "1h", // Hardcode or selector?
    },
    requestPolicy: "network-only"
  }));

  const clusterInfo = $derived.by(() => {
    if ($initq?.data?.clusters) {
      let rawInfos = {};
      let subClusters = $initq?.data?.clusters?.find((c) => c.name == presetCluster)?.subClusters || [];
      for (let subCluster of subClusters) {
        // Allocations
        if (!rawInfos['allocatedNodes']) rawInfos['allocatedNodes'] = $statusQuery?.data?.allocatedNodes?.find(({ name }) => name == subCluster.name)?.count || 0;
        else rawInfos['allocatedNodes'] += $statusQuery?.data?.allocatedNodes?.find(({ name }) => name == subCluster.name)?.count || 0;

        if (!rawInfos['allocatedCores']) rawInfos['allocatedCores'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalCores || 0;
        else rawInfos['allocatedCores'] += $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalCores || 0;

        if (!rawInfos['allocatedAccs']) rawInfos['allocatedAccs'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalAccs || 0;
        else rawInfos['allocatedAccs'] += $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalAccs || 0;

        // Infos
        if (!rawInfos['processorTypes']) rawInfos['processorTypes'] = subCluster?.processorType ? new Set([subCluster.processorType]) : new Set([]);
        else rawInfos['processorTypes'].add(subCluster.processorType);

        if (!rawInfos['activeUsers']) rawInfos['activeUsers'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalUsers || 0;
        else rawInfos['activeUsers'] += $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalUsers || 0;

        if (!rawInfos['runningJobs']) rawInfos['runningJobs'] = $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalJobs || 0;
        else rawInfos['runningJobs'] += $statusQuery?.data?.jobsStatistics?.find(({ id }) => id == subCluster.name)?.totalJobs || 0;

        if (!rawInfos['totalNodes']) rawInfos['totalNodes'] = subCluster?.numberOfNodes || 0;
        else rawInfos['totalNodes'] += subCluster?.numberOfNodes || 0;

        if (!rawInfos['totalCores']) rawInfos['totalCores'] = (subCluster?.socketsPerNode * subCluster?.coresPerSocket * subCluster?.numberOfNodes) || 0;
        else rawInfos['totalCores'] += (subCluster?.socketsPerNode * subCluster?.coresPerSocket * subCluster?.numberOfNodes) || 0;

        if (!rawInfos['totalAccs']) rawInfos['totalAccs'] = (subCluster?.numberOfNodes * subCluster?.topology?.accelerators?.length) || 0;
        else rawInfos['totalAccs'] += (subCluster?.numberOfNodes * subCluster?.topology?.accelerators?.length) || 0;
          
        // Units (Set Once)
        if (!rawInfos['flopRateUnit']) rawInfos['flopRateUnit'] = subCluster.flopRateSimd.unit.prefix + subCluster.flopRateSimd.unit.base
        if (!rawInfos['memBwRateUnit']) rawInfos['memBwRateUnit'] = subCluster.memoryBandwidth.unit.prefix + subCluster.memoryBandwidth.unit.base

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

      return rawInfos
    } else {
      return {};
    }
  });

  const refinedStateData = $derived.by(() => {
    return $statusQuery?.data?.nodeStates.
      filter((e) => ['allocated', 'reserved', 'idle', 'mixed','down', 'unknown'].includes(e.state)).
      sort((a, b) => b.count - a.count)
  });

  /* Functions */
  function legendColors(targetIdx, useAltColors) {
    // Reuses first color if targetIdx overflows
    let c;
      if (useCbColors) {
        c = [...colors['colorblind']];
      } else if (useAltColors) {
        c = [...colors['alternative']];
      } else {
        c = [...colors['default']];
      }
    return  c[(c.length + targetIdx) % c.length];
  }

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

  /* Inspect */
  $inspect(clusterInfo).with((type, clusterInfo) => {
    console.log(type, 'clusterInfo', clusterInfo)
	});

  $inspect($statusQuery?.data?.clusterMetrics).with((type, clusterMetrics) => {
    console.log(type, 'clusterMetrics', clusterMetrics)
	});

</script>

<Card style="height: 98vh;">
  <CardHeader class="text-center">
    <h3 class="mb-0">{presetCluster.charAt(0).toUpperCase() + presetCluster.slice(1)} Dashboard</h3>
  </CardHeader>
  <CardBody>
    {#if $statusQuery.fetching || $statesTimed.fetching || $topJobsQuery.fetching || $nodeStatusQuery.fetching}
      <Row class="justify-content-center">
        <Col xs="auto">
          <Spinner />
        </Col>
      </Row>

    {:else if $statusQuery.error || $statesTimed.error || $topJobsQuery.error || $nodeStatusQuery.error}
      <Row cols={{xs:1, md:2}}>
        {#if $statusQuery.error}
          <Col>
            <Card color="danger">Error Requesting StatusQuery: {$statusQuery.error.message}</Card>
          </Col>
        {/if}
        {#if $statesTimed.error}
          <Col>
            <Card color="danger">Error Requesting StatesTimed: {$statesTimed.error.message}</Card>
          </Col>
        {/if}
        {#if $topJobsQuery.error}
          <Col>
            <Card color="danger">Error Requesting TopJobsQuery: {$topJobsQuery.error.message}</Card>
          </Col>
        {/if}
        {#if $nodeStatusQuery.error}
          <Col>
            <Card color="danger">Error Requesting NodeStatusQuery: {$nodeStatusQuery.error.message}</Card>
          </Col>
        {/if}
      </Row>

    {:else}
      <Row cols={{xs:1, md:2, xl: 3}}>
        <Col> <!-- Info Card -->
          <Card class="h-auto mt-1">
            <CardHeader>
              <CardTitle class="mb-0">Cluster "{presetCluster.charAt(0).toUpperCase() + presetCluster.slice(1)}"</CardTitle>
              <span>{[...clusterInfo?.processorTypes].toString()}</span>
            </CardHeader>
            <CardBody>
              <Table borderless>
                <tr class="py-2">
                  <td style="font-size:x-large;">{clusterInfo?.runningJobs} Running Jobs</td>
                  <td colspan="2" style="font-size:x-large;">{clusterInfo?.activeUsers} Active Users</td>
                </tr>
                <hr class="my-1"/>
                <tr class="pt-2">
                  <td style="font-size: large;">
                    Flop Rate (<span style="cursor: help;" title="Flops[Any] = (Flops[Double] x 2) + Flops[Single]">Any</span>)
                  </td>
                  <td colspan="2" style="font-size: large;">
                    Memory BW Rate
                  </td>
                </tr>
                <tr class="pb-2">
                  <td style="font-size:x-large;">
                    {clusterInfo?.flopRate} 
                    {clusterInfo?.flopRateUnit}
                  </td>
                  <td colspan="2" style="font-size:x-large;">
                    {clusterInfo?.memBwRate} 
                    {clusterInfo?.memBwRateUnit}
                  </td>
                </tr>
                <hr class="my-1"/>
                <tr class="py-2">
                  <th scope="col">Allocated Nodes</th>
                  <td style="min-width: 100px;"
                    ><div class="col">
                      <Progress
                        value={clusterInfo?.allocatedNodes}
                        max={clusterInfo?.totalNodes}
                      />
                    </div></td
                  >
                  <td
                    >{clusterInfo?.allocatedNodes} / {clusterInfo?.totalNodes}
                    Nodes</td
                  >
                </tr>
                <tr class="py-2">
                  <th scope="col">Allocated Cores</th>
                  <td style="min-width: 100px;"
                    ><div class="col">
                      <Progress
                        value={clusterInfo?.allocatedCores}
                        max={clusterInfo?.totalCores}
                      />
                    </div></td
                  >
                  <td
                    >{formatNumber(clusterInfo?.allocatedCores)} / {formatNumber(clusterInfo?.totalCores)}
                    Cores</td
                  >
                </tr>
                {#if clusterInfo?.totalAccs !== 0}
                  <tr class="py-2">
                    <th scope="col">Allocated Accelerators</th>
                    <td style="min-width: 100px;"
                      ><div class="col">
                        <Progress
                          value={clusterInfo?.allocatedAccs}
                          max={clusterInfo?.totalAccs}
                        />
                      </div></td
                    >
                    <td
                      >{clusterInfo?.allocatedAccs} / {clusterInfo?.totalAccs}
                      Accelerators</td
                    >
                  </tr>
                {/if}
              </Table>
            </CardBody>
          </Card>
        </Col>
        <Col> <!-- Pie Last States -->
          <Row>
            <Col class="px-3 mt-2 mt-lg-0">
              <div bind:clientWidth={colWidthStates}>
                {#key refinedStateData}
                  <h4 class="text-center">
                    Current Node States
                  </h4>
                  <Pie
                    useAltColors
                    canvasId="hpcpie-slurm"
                    size={colWidthStates * 0.75}
                    sliceLabel="Nodes"
                    quantities={refinedStateData.map(
                      (sd) => sd.count,
                    )}
                    entities={refinedStateData.map(
                      (sd) => sd.state,
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
                    <th>Current State</th>
                    <th>Nodes</th>
                  </tr>
                  {#each refinedStateData as sd, i}
                    <tr>
                      <td><Icon name="circle-fill" style="color: {legendColors(i, true)};"/></td>
                      <td>{sd.state}</td>
                      <td>{sd.count}</td>
                    </tr>
                  {/each}
                </Table>
              {/key}
            </Col>
          </Row>
        </Col>
        <Col> <!-- General Cluster Info Card? -->
          <!-- TODO -->
           <Card>
            <CardHeader>
              <CardTitle>Infos</CardTitle>
            </CardHeader>
            <CardBody>
              Contents
            </CardBody>
           </Card>
        </Col>
        <Col> <!-- Nodes Roofline -->
          <div bind:clientWidth={colWidthRoof}>
            {#key $statusQuery?.data?.nodeMetrics}
              <Roofline
                useColors={false}
                useLegend={false}
                allowSizeChange
                width={colWidthRoof - 10}
                height={300}
                cluster={presetCluster}
                subCluster={clusterInfo?.roofData ? clusterInfo.roofData : null}
                roofData={transformNodesStatsToData($statusQuery?.data?.nodeMetrics)}
                nodesData={transformNodesStatsToInfo($statusQuery?.data?.nodeMetrics)}
              />
            {/key}
          </div>
        </Col>
        <Col> <!-- Resources/Job Histogram OR Total Cluster Metric in Time SUMS-->
          <div bind:clientWidth={colWidthTotals}>
            <DoubleMetric
              width={colWidthTotals}
              timestep={$statusQuery?.data?.clusterMetrics[0]?.timestep || 60}
              numNodes={$statusQuery?.data?.clusterMetrics?.nodeCount || 0}
              metricData={$statusQuery?.data?.clusterMetrics?.metrics || []}
              cluster={presetCluster}
            />
          </div>
          <!-- {#if clusterInfo?.totalAccs == 0}
            <Histogram
              data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumCores)}
              title="Number of Cores Distribution"
              xlabel="Allocated Cores"
              xunit="Nodes"
              ylabel="Number of Jobs"
              yunit="Jobs"
              height="275"
              enableFlip
            />
          {:else}
            <Histogram
              data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumAccs)}
              title="Number of Accelerators Distribution"
              xlabel="Allocated Accs"
              xunit="Accs"
              ylabel="Number of Jobs"
              yunit="Jobs"
              height="275"
              enableFlip
            />
          {/if} -->
        </Col>
        <Col> <!-- Stacked SchedState -->
          <div bind:clientWidth={colWidthStacked}>
            {#key $statesTimed?.data?.nodeStates}
              <Stacked
                data={$statesTimed?.data?.nodeStates}
                width={colWidthStacked * 0.95}
                xlabel="Time"
                ylabel="Nodes"
                yunit = "#Count"
                title = "Node States"
                stateType = "Node"
              />
            {/key}
          </div>
        </Col>
      </Row>
    {/if}
  </CardBody>
</Card>
