<!-- 
    @component Data row for a single node displaying metric plots

    Properties:
    - `job Object`: The job object (GraphQL.Job)
    - `metrics [String]`: Currently selected metrics
    - `plotWidth Number`: Width of the sub-components
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
  {#each sortOrder(nodeData?.data) as metricData}
    <td>
      {#if metricData}
        {#if nodeData?.disabled[metricData.name]}
          <Card style="margin-left: 2rem;margin-right: 2rem;" body color="info"
            >Metric disabled for subcluster <code
              >{metricData.name}:{nodeData.subCluster}</code
            ></Card
          >
        {:else}
          <MetricPlot
            timestep={metricData.metric.timestep}
            series={metricData.metric.series}
            metric={metricData.name}
            {cluster}
            subCluster={nodeData.subCluster}
            forNode={true}
          />
        {/if}
      {:else}
        <Card
          style="margin-left: 2rem;margin-right: 2rem;"
          body
          color="warning"
          >No dataset returned for <code>{metricData.name}</code></Card
        >
      {/if}
    </td>
  {/each}
</tr>
