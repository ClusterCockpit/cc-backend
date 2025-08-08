<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
    Card,
    CardHeader,
    CardTitle,
    CardBody,
    Table,
    Progress,
    // Icon,
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    // transformPerNodeDataForRoofline,

  } from "../generic/utils.js";
  import { scaleNumbers, formatTime } from "../generic/units.js";
  import Roofline from "../generic/plots/Roofline.svelte";

  /* Svelte 5 Props */
  let {
    cluster
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const client = getContextClient();

  /* State Init */
  let from = $state(new Date(Date.now() - 5 * 60 * 1000));
  let to = $state(new Date(Date.now()));
  let plotWidths = $state([]);
  // Bar Gauges
  let allocatedNodes = $state({});
  let allocatedAccs = $state({});
  let flopRate = $state({});
  let flopRateUnitPrefix = $state({});
  let flopRateUnitBase = $state({});
  let memBwRate = $state({});
  let memBwRateUnitPrefix = $state({});
  let memBwRateUnitBase = $state({});
  // Plain Infos
  let runningJobs = $state({});
  let activeUsers = $state({});
  let totalAccs = $state({});

  /* Derived */
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
        # Get States for Node Roofline; $sorting unused in backend: Use placeholder
        nodes(filter: $nodeFilter, order: $sorting) {
          count
          items {
            hostname
            cluster
            subCluster
            nodeState
          }
        }
        # totalNodes includes multiples if shared jobs
        jobsStatistics(
          filter: $jobFilter
          page: $paging
          sortBy: TOTALJOBS
          groupBy: SUBCLUSTER
        ) {
          id
          totalJobs
          totalUsers
          totalAccs
        }
      }
    `,
    variables: {
      cluster: cluster,
      metrics: ["flops_any", "mem_bw"], // Fixed names for roofline and status bars
      from: from.toISOString(),
      to: to.toISOString(),
      jobFilter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      nodeFilter: { cluster: { eq: cluster }},
      paging: { itemsPerPage: -1, page: 1 }, // Get all: -1
      sorting: { field: "startTime", type: "col", order: "DESC" }
    },
  }));

  /* Effects */
  $effect(() => {
    if ($initq.data && $statusQuery.data) {
      let subClusters = $initq.data.clusters.find(
        (c) => c.name == cluster,
      ).subClusters;
      for (let subCluster of subClusters) {
        // Allocations
        allocatedNodes[subCluster.name] =
          $statusQuery.data.allocatedNodes.find(
            ({ name }) => name == subCluster.name,
          )?.count || 0;
        allocatedAccs[subCluster.name] =
          $statusQuery.data.jobsStatistics.find(
            ({ id }) => id == subCluster.name,
          )?.totalAccs || 0;
        // Infos
        activeUsers[subCluster.name] =
          $statusQuery.data.jobsStatistics.find(
            ({ id }) => id == subCluster.name,
          )?.totalUsers || 0;
        runningJobs[subCluster.name] =
          $statusQuery.data.jobsStatistics.find(
            ({ id }) => id == subCluster.name,
          )?.totalJobs || 0;
        totalAccs[subCluster.name] =
          (subCluster?.numberOfNodes * subCluster?.topology?.accelerators?.length) || null;
        // Keymetrics
        flopRate[subCluster.name] =
          Math.floor(
            sumUp($statusQuery.data.nodeMetrics, subCluster.name, "flops_any") *
              100,
          ) / 100;
        flopRateUnitPrefix[subCluster.name] = subCluster.flopRateSimd.unit.prefix;
        flopRateUnitBase[subCluster.name] = subCluster.flopRateSimd.unit.base;
        memBwRate[subCluster.name] =
          Math.floor(
            sumUp($statusQuery.data.nodeMetrics, subCluster.name, "mem_bw") * 100,
          ) / 100;
        memBwRateUnitPrefix[subCluster.name] =
          subCluster.memoryBandwidth.unit.prefix;
        memBwRateUnitBase[subCluster.name] = subCluster.memoryBandwidth.unit.base;
      }
    }
  });

  /* Const Functions */
  // New: Sum Up Node Averages
  const sumUp = (data, subcluster, metric) =>
    data.reduce(
      (sum, node) =>
        node.subCluster == subcluster
          ? sum +
            (node.metrics
              .find((m) => m.name == metric)
              ?.metric?.series[0]?.statistics?.avg || 0
            )
          : sum,
      0,
    );

  // Old: SumUp Metric Time Data
  // const sumUp = (data, subcluster, metric) =>
  //   data.reduce(
  //     (sum, node) =>
  //       node.subCluster == subcluster
  //         ? sum +
  //           (node.metrics
  //             .find((m) => m.name == metric)
  //             ?.metric.series.reduce(
  //               (sum, series) => sum + series.data[series.data.length - 1],
  //               0,
  //             ) || 0)
  //         : sum,
  //     0,
  //   );

  /* Functions */
  function transformJobsStatsToData(subclusterData) {
    /* c will contain values from 0 to 1 representing the duration */
    let data = null
    const x = [], y = [], c = [], day = 86400.0

    if (subclusterData) {
      for (let i = 0; i < subclusterData.length; i++) {
        const flopsData = subclusterData[i].stats.find((s) => s.name == "flops_any")
        const memBwData = subclusterData[i].stats.find((s) => s.name == "mem_bw")
            
        const f = flopsData.data.avg
        const m = memBwData.data.avg
        const d = subclusterData[i].duration / day

        const intensity = f / m
        if (Number.isNaN(intensity) || !Number.isFinite(intensity))
            continue

        x.push(intensity)
        y.push(f)
        // Long Jobs > 1 Day: Use max Color
        if (d > 1.0) c.push(1.0)
        else c.push(d)
      }
    } else {
        console.warn("transformJobsStatsToData: metrics for 'mem_bw' and/or 'flops_any' missing!")
    }

    if (x.length > 0 && y.length > 0 && c.length > 0) {
        data = [null, [x, y], c] // for dataformat see roofline.svelte
    }
    return data
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
            // continue // Old: Introduces mismatch between Data and Info Arrays
            intensity = 0.0 // New: Set to Float Zero: Will not show in Log-Plot (Always below render limit)
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

  function transformJobsStatsToInfo(subclusterData) {
    if (subclusterData) {
        return subclusterData.map((sc) => { return {id: sc.id, jobId: sc.jobId, numNodes: sc.numNodes, numAcc: sc?.numAccelerators? sc.numAccelerators : 0, duration: formatTime(sc.duration)} })
    } else {
        console.warn("transformJobsStatsToInfo: jobInfo missing!")
        return []
    }
  }

  function transformNodesStatsToInfo(subClusterData) {
    let result = [];
    if (subClusterData) { //  && $nodesState?.data) {
      // Use Nodes as Returned from CCMS, *NOT* as saved in DB via SlurmState-API!
      for (let j = 0; j < subClusterData.length; j++) {
        // nodesCounts[subClusterData[i].subCluster] = $nodesState.data.nodes.count; // Probably better as own derived!

        const nodeName = subClusterData[j]?.host ? subClusterData[j].host : "unknown"
        const nodeMatch = $statusQuery?.data?.nodes?.items?.find((n) => n.hostname == nodeName && n.subCluster == subClusterData[j].subCluster);
        const nodeState = nodeMatch?.nodeState ? nodeMatch.nodeState : "notindb"
        let numJobs = 0

        if ($statusQuery?.data) {
          const nodeJobs = $statusQuery?.data?.jobs?.items?.filter((job) => job.resources.find((res) => res.hostname == nodeName))
          numJobs = nodeJobs?.length ? nodeJobs.length : 0
        }

        result.push({nodeName: nodeName, nodeState: nodeState, numJobs: numJobs})
      };
    };
    return result
  }

</script>

<!-- Gauges & Roofline per Subcluster-->
{#if $initq.data && $statusQuery.data}
  {#each $initq.data.clusters.find((c) => c.name == cluster).subClusters as subCluster, i}
    <Row cols={{ lg: 3, md: 1 , sm: 1}} class="mb-3 justify-content-center">
      <Col class="px-3">
        <Card class="h-auto mt-1">
          <CardHeader>
            <CardTitle class="mb-0">SubCluster "{subCluster.name}"</CardTitle>
            <span>{subCluster.processorType}</span>
          </CardHeader>
          <CardBody>
            <Table borderless>
              <tr class="py-2">
                <td style="font-size:x-large;">{runningJobs[subCluster.name]} Running Jobs</td>
                <td colspan="2" style="font-size:x-large;">{activeUsers[subCluster.name]} Active Users</td>
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
                  {flopRate[subCluster.name]} 
                  {flopRateUnitPrefix[subCluster.name]}{flopRateUnitBase[subCluster.name]}
                </td>
                <td colspan="2" style="font-size:x-large;">
                  {memBwRate[subCluster.name]} 
                  {memBwRateUnitPrefix[subCluster.name]}{memBwRateUnitBase[subCluster.name]}
                </td>
              </tr>
              <hr class="my-1"/>
              <tr class="py-2">
                <th scope="col">Allocated Nodes</th>
                <td style="min-width: 100px;"
                  ><div class="col">
                    <Progress
                      value={allocatedNodes[subCluster.name]}
                      max={subCluster.numberOfNodes}
                    />
                  </div></td
                >
                <td
                  >{allocatedNodes[subCluster.name]} / {subCluster.numberOfNodes}
                  Nodes</td
                >
              </tr>
              {#if totalAccs[subCluster.name] !== null}
                <tr class="py-2">
                  <th scope="col">Allocated Accelerators</th>
                  <td style="min-width: 100px;"
                    ><div class="col">
                      <Progress
                        value={allocatedAccs[subCluster.name]}
                        max={totalAccs[subCluster.name]}
                      />
                    </div></td
                  >
                  <td
                    >{allocatedAccs[subCluster.name]} / {totalAccs[subCluster.name]}
                    Accelerators</td
                  >
                </tr>
              {/if}
            </Table>
          </CardBody>
        </Card>
      </Col>
      <Col class="px-3 mt-2 mt-lg-0">
        <div bind:clientWidth={plotWidths[i]}>
          {#key $statusQuery?.data?.nodeMetrics}
            <Roofline
              useColors={true}
              allowSizeChange
              width={plotWidths[i] - 10}
              height={300}
              cluster={cluster}
              subCluster={subCluster}
              roofData={transformNodesStatsToData($statusQuery?.data?.nodeMetrics.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
              nodesData={transformNodesStatsToInfo($statusQuery?.data?.nodeMetrics.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
            />
          {/key}
        </div>
      </Col>
      <Col class="px-3 mt-2 mt-lg-0">
        <div bind:clientWidth={plotWidths[i]}>
          {#key $statusQuery?.data?.jobsMetricStats}
            <Roofline
              useColors={true}
              allowSizeChange
              width={plotWidths[i] - 10}
              height={300}
              subCluster={subCluster}
              roofData={transformJobsStatsToData($statusQuery?.data?.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
              jobsData={transformJobsStatsToInfo($statusQuery?.data?.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
            />
          {/key}
        </div>
      </Col>
    </Row>
  {/each}
{:else}
  <Card class="mx-4" body color="warning">Cannot render status tab: No data!</Card>
{/if}
