<!-- 
    @component Data row for a single node displaying metric plots

    Properties:
    - `cluster String`: The nodes' cluster
    - `nodeData Object`: The node data object including metric data
    - `selectedMetrics [String]`: The array of selected metrics
 -->

<script>
  import { Card } from "@sveltestrap/sveltestrap";
  import { maxScope, checkMetricDisabled } from "../../generic/utils.js";
  import MetricPlot from "../../generic/plots/MetricPlot.svelte";
  import NodeInfo from "./NodeInfo.svelte";

  export let cluster;
  export let nodeData;
  export let selectedMetrics;

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
    // Check data for series, skip disabled
    dataHealth = refinedData.filter((rd) => rd.disabled === false).map((enabled) => (enabled.data.metric.series.length > 0))
  }
</script>

<tr>
  <td>
    <NodeInfo {cluster} subCluster={nodeData.subCluster} hostname={nodeData.host} {dataHealth}/>
  </td>
  {#each refinedData as metricData (metricData.data.name)}
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
        <MetricPlot
          {cluster}
          subCluster={nodeData.subCluster}
          metric={metricData.data.name}
          scope={metricData.data.scope}
          timestep={metricData.data.metric.timestep}
          series={metricData.data.metric.series}
          height={175}
          forNode
        />
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
