<!-- 
    @component Data row for a single node displaying metric plots

    Properties:
    - `cluster String`: The nodes' cluster
    - `nodeData Object`: The node data object including metric data
    - `selectedMetrics [String]`: The array of selected metrics
 -->

<script>
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import { Card, CardBody, Spinner } from "@sveltestrap/sveltestrap";
  import { maxScope, checkMetricDisabled } from "../../generic/utils.js";
  import MetricPlot from "../../generic/plots/MetricPlot.svelte";
  import NodeInfo from "./NodeInfo.svelte";

  export let cluster;
  export let nodeData;
  export let selectedMetrics;

  const client = getContextClient();
  const paging = { itemsPerPage: 50, page: 1 };
  const sorting = { field: "startTime", type: "col", order: "DESC" };
  const filter = [
    { cluster: { eq: cluster } },
    { node: { contains: nodeData.host } },
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
          user
          project
          exclusive
          resources {
            hostname
            accelerators
          }
        }
        count
      }
    }
  `;

  $: nodeJobsData = queryStore({
    client: client,
    query: nodeJobsQuery,
    variables: { paging, sorting, filter },
  });

  // Helper
  const selectScope = (nodeMetrics) =>
    nodeMetrics.reduce(
      (a, b) =>
        maxScope([a.scope, b.scope]) == a.scope ? b : a,
      nodeMetrics[0],
    );

  const sortAndSelectScope = (allNodeMetrics) =>
    selectedMetrics
      .map((selectedName) => allNodeMetrics.filter((nodeMetric) => nodeMetric.name == selectedName))
      .map((matchedNodeMetrics) => ({
        disabled: false,
        data: matchedNodeMetrics.length > 0 ? selectScope(matchedNodeMetrics) : null,
      }))
      .map((scopedNodeMetric) => {
        if (scopedNodeMetric?.data) {
          return {
            disabled: checkMetricDisabled(
              scopedNodeMetric.data.name,
              cluster,
              nodeData.subCluster,
            ),
            data: scopedNodeMetric.data,
          };
        } else {
          return scopedNodeMetric;
        }
      });

  let refinedData;
  let dataHealth;
  $: if (nodeData?.metrics) {
    refinedData = sortAndSelectScope(nodeData?.metrics)
    dataHealth = refinedData.filter((rd) => rd.disabled === false).map((enabled) => (enabled.data.metric.series.length > 0))
  }

  let extendedLegendData = null;
  $: if ($nodeJobsData?.data) {
    // Build Extended for allocated nodes [Commented: Only Build extended Legend For Shared Nodes]
    if ($nodeJobsData.data.jobs.count >= 1) { // "&& !$nodeJobsData.data.jobs.items[0].exclusive)"
      const accSet = Array.from(new Set($nodeJobsData.data.jobs.items
        .map((i) => i.resources
          .filter((r) => r.hostname === nodeData.host)
          .map((r) => r.accelerators)
        )
      )).flat(2)

      extendedLegendData = {}
      for (const accId of accSet) {
        const matchJob = $nodeJobsData.data.jobs.items.find((i) => i.resources.find((r) => r.accelerators.includes(accId)))
        extendedLegendData[accId] = {
          user: matchJob?.user  ? matchJob?.user  : '-',
          job:  matchJob?.jobId ? matchJob?.jobId : '-',
        }
      }
      // Theoretically extendable for hwthreadIDs
    }
  }
</script>

<tr>
  <td>
    {#if $nodeJobsData.fetching}
      <Card>
        <CardBody class="content-center">
          <Spinner/>
        </CardBody>
      </Card>
    {:else}
      <NodeInfo nodeJobsData={$nodeJobsData.data} {cluster} subCluster={nodeData.subCluster} hostname={nodeData.host} {dataHealth}/>
    {/if}
  </td>
  {#each refinedData as metricData (metricData.data.name)}
    {#key metricData}
      <td>
        {#if metricData?.disabled}
          <Card body class="mx-3" color="info"
            >Metric disabled for subcluster <code
              >{metricData.data.name}:{nodeData.subCluster}</code
            ></Card
          >
        {:else if !!metricData.data?.metric.statisticsSeries}
          <!-- "No Data"-Warning included in MetricPlot-Component -->
            <MetricPlot
              {cluster}
              subCluster={nodeData.subCluster}
              metric={metricData.data.name}
              scope={metricData.data.scope}
              timestep={metricData.data.metric.timestep}
              series={metricData.data.metric.series}
              statisticsSeries={metricData.data?.metric.statisticsSeries}
              useStatsSeries={!!metricData.data?.metric.statisticsSeries}
              height={175}
              forNode
            />
          <div class="my-2"/>
          {#key extendedLegendData}
            <MetricPlot
              {cluster}
              subCluster={nodeData.subCluster}
              metric={metricData.data.name}
              scope={metricData.data.scope}
              timestep={metricData.data.metric.timestep}
              series={metricData.data.metric.series}
              height={175}
              {extendedLegendData}
              forNode
            />
          {/key}
        {:else}
            <MetricPlot
              {cluster}
              subCluster={nodeData.subCluster}
              metric={metricData.data.name}
              scope={metricData.data.scope}
              timestep={metricData.data.metric.timestep}
              series={metricData.data.metric.series}
              height={375}
              forNode
            />
        {/if}
      </td>
    {/key}
  {/each}
</tr>
