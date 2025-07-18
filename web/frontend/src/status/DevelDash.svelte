<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
  } from "../generic/utils.js";
  import Roofline from "../generic/plots/Roofline.svelte";
  import NewBubbleRoofline from "../generic/plots/NewBubbleRoofline.svelte";

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
  let nodesCounts = $state({});
  let jobsJounts = $state({});

  /* Derived */
  // Note: nodeMetrics are requested on configured $timestep resolution
  // Result: The latest 5 minutes (datapoints) for each node independent of job
  const jobRoofQuery = $derived(queryStore({
    client: client,
    query: gql`
      query ($filter: [JobFilter!]!, $metrics: [String!]!) {
        jobsMetricStats(filter: $filter, metrics: $metrics) {
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
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      metrics: ["flops_any", "mem_bw"], // Fixed names for job roofline
    },
  }));

  // Optimal new query, does not exist
  // const nodeRoofQuery = $derived(queryStore({
  //   client: client,
  //   query: gql`
  //     query ($filter: [JobFilter!]!, $metrics: [String!]!) {
  //       nodeRoofline(filter: $filter, metrics: $metrics) {
  //         nodeName
  //         nodeState
  //         numJobs
  //         stats {
  //           name
  //           data {
  //             avg
  //           }
  //         }
  //       }
  //     }
  //   `,
  //   variables: {
  //     filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
  //     metrics: ["flops_any", "mem_bw"], // Fixed names for job roofline
  //   },
  // }));

  // Load Required Roofline Data Averages for all nodes of cluster: use for node avg data and name, use secondary (new?) querie(s) for slurmstate and numjobs
  const nodesData = $derived(queryStore({
    client: client,
    query: gql`
      query ($cluster: String!, $metrics: [String!], $from: Time!, $to: Time!) {
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
      }
    `,
    variables: {
      cluster: cluster,
      metrics: ["flops_any", "mem_bw"],
      from: from,
      to: to,
    },
  }));

  // Load for jobcount per node only -- might me required for total running jobs anyways in parent component!
  // Also, think about extra query with only TotalJobCount and Items [Resources, ...some meta infos], not including metric data
  const paging = { itemsPerPage: 1500, page: 1 };
  const sorting = { field: "startTime", type: "col", order: "DESC" };
  const filter = [
    { cluster: { eq: cluster } },
    { state: ["running"] },
  ];
  const nodeJobsQuery = gql`
    query (
      $filter: [JobFilter!]!
      $sorting: OrderByInput!
      $paging: PageRequest!
    ) {
      jobs(filter: $filter, order: $sorting, page: $paging) {
        items {
          jobId
          resources {
            hostname
          }
        }
        count
      }
    }
  `;

  const nodesJobs = $derived(queryStore({
      client: client,
      query: nodeJobsQuery,
      variables: { paging, sorting, filter },
    })
  );

  // Last required query: Node State
  const nodesState = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [NodeFilter!]
        $sorting: OrderByInput
      ) {
        nodes(filter: $filter, order: $sorting) {
          count
          items {
            hostname
            cluster
            subCluster
            nodeState
          }
        }
      }
    `,
    variables: {
      filter: { cluster: { eq: cluster }},
      sorting: sorting // Unused in Backend: Use Placeholder
      // Subcluster filter?
    },
  }));

  /* Function */
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
        return subclusterData.map((sc) => { return {id: sc.id, jobId: sc.jobId, numNodes: sc.numNodes, numAcc: sc?.numAccelerators? sc.numAccelerators : 0} })
    } else {
        console.warn("transformJobsStatsToInfo: jobInfo missing!")
        return []
    }
  }

  function transformNodesStatsToInfo(subClusterData) {
    let result = [];
    if (subClusterData && $nodesState?.data) {
      // Use Nodes as Returned from CCMS, *NOT* as saved in DB via SlurmState-API!
      for (let j = 0; j < subClusterData.length; j++) {
        // nodesCounts[subClusterData[i].subCluster] = $nodesState.data.nodes.count; // Probably better as own derived!

        const nodeName = subClusterData[j]?.host ? subClusterData[j].host : "unknown"
        const nodeMatch = $nodesState.data.nodes.items.find((n) => n.hostname == nodeName && n.subCluster == subClusterData[j].subCluster);
        const nodeState = nodeMatch?.nodeState ? nodeMatch.nodeState : "notindb"
        let numJobs = 0

        if ($nodesJobs?.data) {
          const nodeJobs = $nodesJobs.data.jobs.items.filter((job) => job.resources.find((res) => res.hostname == nodeName))
          numJobs = nodeJobs?.length ? nodeJobs.length : 0
        }

        result.push({nodeName: nodeName, nodeState: nodeState, numJobs: numJobs})
      };
    };
    return result
  }

</script>

<!-- Gauges & Roofline per Subcluster-->
{#if $initq.data && $jobRoofQuery.data}
  {#each $initq.data.clusters.find((c) => c.name == cluster).subClusters as subCluster, i}
    <Row cols={{ lg: 2, md: 2 , sm: 1}} class="mb-3 justify-content-center">
      <Col class="px-3 mt-2 mt-lg-0">
        <b>Bubble Node</b>
        <div bind:clientWidth={plotWidths[i]}>
          {#key $nodesData?.data?.nodeMetrics || $nodesJobs?.data?.jobs}
            <b>{subCluster.name} Total: {$jobRoofQuery.data.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                ).length} Jobs</b>
            <NewBubbleRoofline
              allowSizeChange
              width={plotWidths[i] - 10}
              height={300}
              cluster={cluster}
              subCluster={subCluster}
              roofData={transformNodesStatsToData($nodesData?.data?.nodeMetrics.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
              nodesData={transformNodesStatsToInfo($nodesData?.data?.nodeMetrics.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
            />
          {/key}
        </div>
      </Col>
      <Col class="px-3 mt-2 mt-lg-0">
        <b>Bubble Jobs</b>
        <div bind:clientWidth={plotWidths[i]}>
          {#key $jobRoofQuery.data.jobsMetricStats}
            <b>{subCluster.name} Total: {$jobRoofQuery.data.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                ).length} Jobs</b>
            <NewBubbleRoofline
              allowSizeChange
              width={plotWidths[i] - 10}
              height={300}
              subCluster={subCluster}
              roofData={transformJobsStatsToData($jobRoofQuery?.data?.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
              jobsData={transformJobsStatsToInfo($jobRoofQuery?.data?.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
            />
          {/key}
        </div>
      </Col>
    </Row>
  {/each}
{/if}
