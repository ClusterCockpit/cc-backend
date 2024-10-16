<!-- 
    @component Data row for a single node displaying metric plots

    Properties:
    - `cluster String`: The nodes' cluster
    - `nodeData Object`: The node data object including metric data
    - `selectedMetrics [String]`: The array of selected metrics
 -->

<script>
  import { Card } from "@sveltestrap/sveltestrap";
  import MetricPlot from "../../generic/plots/MetricPlot.svelte";
  import NodeInfo from "./NodeInfo.svelte";

  export let cluster;
  export let nodeData;
  export let selectedMetrics;

  const sortOrder = (nodeMetrics) =>
    selectedMetrics.map((name) => nodeMetrics.find((nodeMetric) => nodeMetric.name == name));
</script>

<tr>
  <td>
    <NodeInfo {cluster} subCluster={nodeData.subCluster} hostname={nodeData.host} />
  </td>
  {#each sortOrder(nodeData?.data) as metricData (metricData.name)}
    <td>
      {#if nodeData?.disabled[metricData.name]}
        <Card body class="mx-3" color="info"
          >Metric disabled for subcluster <code
            >{metricData.name}:{nodeData.subCluster}</code
          ></Card
        >
      {:else}
        <!-- "No Data"-Warning included in MetricPlot-Component -->
        <MetricPlot
          timestep={metricData.metric.timestep}
          series={metricData.metric.series}
          metric={metricData.name}
          {cluster}
          subCluster={nodeData.subCluster}
          forNode
        />
      {/if}
    </td>
  {/each}
</tr>
