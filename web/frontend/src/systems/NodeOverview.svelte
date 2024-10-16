<!--
    @component Cluster Per Node Overview component; renders current state of ONE metric for ALL nodes

    Properties:
    - `ccconfig Object?`: The ClusterCockpit Config Context [Default: null]
    - `data Object?`: The GQL nodeMetrics data [Default: null]
    - `cluster String`: The cluster to show status information for
    - `selectedMetric String?`: The selectedMetric input [Default: ""]
 -->

 <script>
  import { Row, Col, Card } from "@sveltestrap/sveltestrap";
  import MetricPlot from "../generic/plots/MetricPlot.svelte";

  export let ccconfig = null;
  export let data = null;
  export let cluster = "";
  export let selectedMetric = "";

</script>

<!-- PlotGrid flattened into this component -->
<Row cols={{ xs: 1, sm: 2, md: 3, lg: ccconfig.plot_view_plotsPerRow}}>
  {#each data as item (item.host)}
    <Col class="px-1">
      <h4 style="width: 100%; text-align: center;">
        <a
          style="display: block;padding-top: 15px;"
          href="/monitoring/node/{cluster}/{item.host}"
          >{item.host} ({item.subCluster})</a
        >
      </h4>
      {#if item?.disabled[selectedMetric]}
        <Card body class="mx-3" color="info"
          >Metric disabled for subcluster <code
            >{selectedMetric}:{item.subCluster}</code
          ></Card
        >
      {:else}
        <!-- "No Data"-Warning included in MetricPlot-Component -->
        <MetricPlot
          timestep={item.data[0].metric.timestep}
          series={item.data[0].metric.series}
          metric={item.data[0].name}
          {cluster}
          subCluster={item.subCluster}
          forNode
        />
      {/if}
    </Col>
  {/each}
</Row>