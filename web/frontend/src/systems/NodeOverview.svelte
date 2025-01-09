<!--
    @component Cluster Per Node Overview component; renders current state of ONE metric for ALL nodes

    Properties:
    - `ccconfig Object?`: The ClusterCockpit Config Context [Default: null]
    - `cluster String`: The cluster to show status information for
    - `selectedMetric String?`: The selectedMetric input [Default: ""]
 -->

 <script>
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import { Row, Col, Card, Spinner } from "@sveltestrap/sveltestrap";
  import { init, checkMetricsDisabled } from "../generic/utils.js";
  import MetricPlot from "../generic/plots/MetricPlot.svelte";

  export let ccconfig = null;
  export let cluster = "";
  export const subCluster = "";
  export let selectedMetrics = null;
  export let hostnameFilter = "";
  export let from = null;
  export let to = null;

  const { query: initq } = init();
  const client = getContextClient();
  const nodeQuery = gql`
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
          scope
          metric {
            timestep
            unit {
              base
              prefix
            }
            series {
              statistics {
                min
                avg
                max
              }
              data
            }
          }
        }
      }
    }
  `

  $: selectedMetric = selectedMetrics[0] ? selectedMetrics[0] : "";

  $: nodesQuery = queryStore({
    client: client,
    query: nodeQuery,
    variables: {
      cluster: cluster,
      metrics: selectedMetrics,
      from: from.toISOString(),
      to: to.toISOString(),
    },
  });

  let rawData = []
  $: if ($initq.data && $nodesQuery?.data) {
    rawData = $nodesQuery?.data?.nodeMetrics.filter((h) => {
      if (h.subCluster === '') { // Exclude nodes with empty subCluster field
        console.warn('subCluster not configured for node', h.host)
        return false
      } else {
        return h.metrics.some(
          (m) => selectedMetrics.includes(m.name) && m.scope == "node",
        )
      }
    })
  }

  let mappedData = []
  $: if (rawData?.length > 0) {
    mappedData = rawData.map((h) => ({
      host: h.host,
      subCluster: h.subCluster,
      data: h.metrics.filter(
        (m) => selectedMetrics.includes(m.name) && m.scope == "node",
      ),
      disabled: checkMetricsDisabled(
        selectedMetrics,
        cluster,
        h.subCluster,
      ),
    }))
    .sort((a, b) => a.host.localeCompare(b.host))
  }

  let filteredData = []
  $: if (mappedData?.length > 0) {
    filteredData = mappedData.filter((h) =>
      h.host.includes(hostnameFilter)
    )
  }
</script>

{#if $nodesQuery.error}
  <Row>
    <Col>
      <Card body color="danger">{$nodesQuery.error.message}</Card>
    </Col>
  </Row>
{:else if $nodesQuery.fetching }
  <Row>
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if filteredData?.length > 0}
  <!-- PlotGrid flattened into this component -->
  <Row cols={{ xs: 1, sm: 2, md: 3, lg: ccconfig.plot_view_plotsPerRow}}>
    {#each filteredData as item (item.host)}
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
{/if}