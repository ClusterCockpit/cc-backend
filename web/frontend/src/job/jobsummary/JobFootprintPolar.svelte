<!--
    @component Job Footprint Polar Plot component; Displays queried job metric statistics polar plot.

    Properties:
    - `job Object`: The GQL job object
 -->

 <script>
    import { getContext } from "svelte";
    import { 
      queryStore,
      gql,
      getContextClient 
    } from "@urql/svelte";
    import {
      Card,
      CardBody,
      Spinner
    } from "@sveltestrap/sveltestrap";
    import { findJobFootprintThresholds } from "../../generic/utils.js";
    import Polar from "../../generic/plots/Polar.svelte";

    /* Svelte 5 Props */
    let { job } = $props();
  
    /* Const Init */
    // Metric Names Configured To Be Footprints For (sub)Cluster
    const clusterFootprintMetrics = getContext("clusters")
    .find((c) => c.name == job.cluster)?.subClusters
    .find((sc) => sc.name == job.subCluster)?.footprint || []

    // Get Scaled Peak Threshold Based on Footprint Type ([min, max, avg]) and Job Exclusivity
    const polarMetrics = getContext("globalMetrics").reduce((pms, gm) => {
    if (clusterFootprintMetrics.includes(gm.name)) {
        const fmt = findJobFootprintThresholds(job, gm.footprint, getContext("getMetricConfig")(job.cluster, job.subCluster, gm.name));
        pms.push({ name: gm.name, peak: fmt ? fmt.peak : null });
    }
    return pms;
    }, [])

    // Pull All Series For Footprint Metrics Statistics Only On Node Scope
    const client = getContextClient();
    const polarQuery = gql`
      query ($dbid: ID!, $selectedMetrics: [String!]!) {
        jobStats(id: $dbid, metrics: $selectedMetrics) {
          name
          data {
            min
            avg
            max
          }
        }
      }
    `;

    /* Derived */
    const polarData = $derived(queryStore({
      client: client,
      query: polarQuery,
      variables:{ dbid: job.id, selectedMetrics: clusterFootprintMetrics },
    }));
</script>

<CardBody>
  {#if $polarData.fetching}
    <Spinner />
  {:else if $polarData.error}
    <Card body color="danger">{$polarData.error.message}</Card>
  {:else}
    <Polar
      {polarMetrics}
      polarData={$polarData.data.jobStats}
    />
  {/if}
</CardBody>