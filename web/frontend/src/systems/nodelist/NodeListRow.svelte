<!-- 
  @component Data row for a single node displaying metric plots

  Properties:
  - `cluster String`: The nodes' cluster
  - `nodeData Object`: The node data object including metric data
  - `selectedMetrics [String]`: The array of selected metrics
  - `globalMetrics [Obj]`: Includes the backend supplied availabilities for cluster and subCluster
-->

<script>
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import uPlot from "uplot";
  import { Card, CardBody, Spinner } from "@sveltestrap/sveltestrap";
  import { maxScope, checkMetricDisabled, scramble, scrambleNames } from "../../generic/utils.js";
  import MetricPlot from "../../generic/plots/MetricPlot.svelte";
  import NodeInfo from "./NodeInfo.svelte";

  /* Svelte 5 Props */
  let {
    cluster,
    nodeData,
    selectedMetrics,
    globalMetrics
  } = $props();

  /* Var Init*/
  // svelte-ignore state_referenced_locally
  let plotSync = uPlot.sync(`nodeMetricStack-${nodeData.host}`);

  /* Const Init */
  const client = getContextClient();
  const paging = { itemsPerPage: 50, page: 1 };
  const sorting = { field: "startTime", type: "col", order: "DESC" };
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
          shared
          resources {
            hostname
            accelerators
          }
        }
        count
      }
    }
  `;

  /* Derived */
  const filter = $derived([
    { cluster: { eq: cluster } },
    { node: { contains: nodeData.host } },
    { state: ["running"] },
  ]);
  const nodeJobsData = $derived(queryStore({
      client: client,
      query: nodeJobsQuery,
      variables: { paging, sorting, filter },
    })
  );

  const extendedLegendData = $derived($nodeJobsData?.data ? buildExtendedLegend() : null);
  const refinedData = $derived(nodeData?.metrics ? sortAndSelectScope(selectedMetrics, nodeData.metrics) : []);
  const dataHealth = $derived(refinedData.filter((rd) => rd.disabled === false).map((enabled) => (enabled?.data?.metric?.series?.length > 0)));

  /* Functions */
  function sortAndSelectScope(metricList = [], nodeMetrics = []) {
    const pendingData = [];
    metricList.forEach((metricName) => {
      const pendingMetric = {
        name: metricName,
        disabled: checkMetricDisabled(
          globalMetrics,
          metricName,
          cluster,
          nodeData.subCluster,
        ),
        data: null
      };
      const scopesData = nodeMetrics.filter((nodeMetric) => nodeMetric.name == metricName)
      if (scopesData.length > 0) pendingMetric.data = selectScope(scopesData)
      pendingData.push(pendingMetric)
    });
    return pendingData;
  };

  const selectScope = (nodeMetrics) =>
    nodeMetrics.reduce(
      (a, b) =>
        maxScope([a.scope, b.scope]) == a.scope ? b : a,
      nodeMetrics[0],
    );

  function buildExtendedLegend() {
    let pendingExtendedLegendData = null
    // Build Extended for allocated nodes [Commented: Only Build extended Legend For Shared Nodes]
    if ($nodeJobsData.data.jobs.count >= 1) { 
      const accSet = Array.from(new Set($nodeJobsData.data.jobs.items
        .map((i) => i.resources
          .filter((r) => (r.hostname === nodeData.host) && r?.accelerators)
          .map((r) => r?.accelerators)
        )
      )).flat(2)

      pendingExtendedLegendData = {};
      for (const accId of accSet) {
        const matchJob = $nodeJobsData?.data?.jobs?.items?.find((i) => i?.resources?.find((r) => r?.accelerators?.includes(accId))) || null
        const matchUser = matchJob?.user ? matchJob.user : null
        pendingExtendedLegendData[accId] = {
          user: (scrambleNames && matchUser)
            ? scramble(matchUser) 
            : (matchUser ? matchUser : '-'),
          job:  matchJob?.jobId ? matchJob.jobId : '-',
        }
      }
      // Theoretically extendable for hwthreadIDs
    }
    return pendingExtendedLegendData;
  }

  /* Inspect */
  // $inspect(selectedMetrics).with((type, selectedMetrics) => {
  //   console.log(type, 'selectedMetrics', selectedMetrics)
	// });

  // $inspect(nodeData).with((type, nodeData) => {
  //   console.log(type, 'nodeData', nodeData)
	// });

  // $inspect(refinedData).with((type, refinedData) => {
  //   console.log(type, 'refinedData', refinedData)
	// });

  // $inspect(dataHealth).with((type, dataHealth) => {
  //   console.log(type, 'dataHealth', dataHealth)
	// });

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
      <NodeInfo
        {cluster}
        {dataHealth}
        nodeJobsData={$nodeJobsData.data}
        subCluster={nodeData.subCluster}
        hostname={nodeData.host}
        hoststate={nodeData?.state? nodeData.state: 'notindb'}/>
    {/if}
  </td>
  {#each refinedData as metricData, i (metricData?.data?.name || i)}
    <td>
      {#if metricData?.disabled}
        <Card body class="mx-2" color="info">
          <p>No dataset(s) returned for <b>{selectedMetrics[i]}</b></p>
          <p class="mb-1">Metric has been disabled for subcluster <b>{nodeData.subCluster}</b>.</p>
        </Card>
      {:else if !metricData?.data}
        <Card body class="mx-2" color="warning">
          <p>No dataset(s) returned for <b>{selectedMetrics[i]}</b></p>
          <p class="mb-1">Metric was not found in metric store for cluster <b>{cluster}</b>.</p>
        </Card>
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
            {plotSync}
            forNode
          />
        <div class="my-2"></div>
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
            {plotSync}
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
  {/each}
</tr>
