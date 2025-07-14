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
  // let from = $state(new Date(Date.now() - 5 * 60 * 1000));
  // let to = $state(new Date(Date.now()));
  let plotWidths = $state([]);

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
        console.warn("transformData: metrics for 'mem_bw' and/or 'flops_any' missing!")
    }

    if (x.length > 0 && y.length > 0 && c.length > 0) {
        data = [null, [x, y], c] // for dataformat see roofline.svelte
    }
    return data
  }

  function transformJobsStatsToInfo(subclusterData) {
    if (subclusterData) {
        return subclusterData.map((sc) => { return {id: sc.id, jobId: sc.jobId, numNodes: sc.numNodes, numAcc: sc?.numAccelerators? sc.numAccelerators : 0} })
    } else {
        console.warn("transformData: jobInfo missing!")
        return []
    }
  }

</script>

<!-- Gauges & Roofline per Subcluster-->
{#if $initq.data && $jobRoofQuery.data}
  {#each $initq.data.clusters.find((c) => c.name == cluster).subClusters as subCluster, i}
    <Row cols={{ lg: 2, md: 2 , sm: 1}} class="mb-3 justify-content-center">
      <Col class="px-3 mt-2 mt-lg-0">
        <b>Classic</b>
        <div bind:clientWidth={plotWidths[i]}>
          {#key $jobRoofQuery.data.jobsMetricStats}
            <b>{subCluster.name} Total: {$jobRoofQuery.data.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                ).length} Jobs</b>
            <Roofline
              allowSizeChange
              renderTime
              width={plotWidths[i] - 10}
              height={300}
              subCluster={subCluster}
              data={transformJobsStatsToData($jobRoofQuery?.data?.jobsMetricStats.filter(
                  (data) => data.subCluster == subCluster.name,
                )
              )}
            />
          {/key}
        </div>
      </Col>
      <Col class="px-3 mt-2 mt-lg-0">
        <b>Bubble</b>
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
