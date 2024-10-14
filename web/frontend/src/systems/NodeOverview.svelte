<!--
    @component Cluster Per Node Overview component; renders current state of ONE metric for ALL nodes

    Properties:
    - `ccconfig Object?`: The ClusterCockpit Config Context [Default: null]
    - `data Object?`: The GQL nodeMetrics data [Default: null]
    - `cluster String`: The cluster to show status information for
    - `selectedMetric String?`: The selectedMetric input [Default: ""]
 -->

 <script>
  import { getContext } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";
  import PlotGrid from "../generic/PlotGrid.svelte";
  import MetricPlot from "../generic/plots/MetricPlot.svelte";

  export let ccconfig = null;
  export let data = null;
  export let cluster = "";
  export let selectedMetric = "";

  const clusters = getContext("clusters");
</script>

<PlotGrid
  let:item
  renderFor="systems"
  itemsPerRow={ccconfig.plot_view_plotsPerRow}
  items={data}
>
  <h4 style="width: 100%; text-align: center;">
    <a
      style="display: block;padding-top: 15px;"
      href="/monitoring/node/{cluster}/{item.host}"
      >{item.host} ({item.subCluster})</a
    >
  </h4>
  {#if item?.data}
    {#if item?.disabled[selectedMetric]}
      <Card style="margin-left: 2rem;margin-right: 2rem;" body color="info"
        >Metric disabled for subcluster <code
          >{selectedMetric}:{item.subCluster}</code
        ></Card
      >
    {:else}
      <MetricPlot
        timestep={item.data.metric.timestep}
        series={item.data.metric.series}
        metric={item.data.name}
        cluster={clusters.find((c) => c.name == cluster)}
        subCluster={item.subCluster}
        forNode={true}
      />
    {/if}
  {:else}
    <Card
      style="margin-left: 2rem;margin-right: 2rem;"
      body
      color="warning"
      >No dataset returned for <code>{selectedMetric}</code></Card
    >
  {/if}
</PlotGrid>
