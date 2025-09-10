<!--
  @component Cluster Per Node Overview component; renders current state of ONE metric for ALL nodes

  Properties:
  - `ccconfig Object?`: The ClusterCockpit Config Context [Default: null]
  - `cluster String`: The cluster to show status information for
  - `selectedMetric String?`: The selectedMetric input [Default: ""]
  - `hostnameFilter String?`: The active hostnamefilter [Default: ""]
  - `from Date?`: The selected "from" date [Default: null]
  - `to Date?`: The selected "to" date [Default: null]
-->

 <script>
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import { Row, Col, Card, Spinner } from "@sveltestrap/sveltestrap";
  import { checkMetricDisabled } from "../generic/utils.js";
  import MetricPlot from "../generic/plots/MetricPlot.svelte";

  /* Svelte 5 Props */
  let {
    ccconfig = null,
    cluster = "",
    selectedMetric = "",
    hostnameFilter = "",
    from = null,
    to = null
  } = $props();

  /* Const Init */
  const client = getContextClient();

  /* Derived */
  const nodesQuery = $derived(queryStore({
    client: client,
    query: gql`
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
    `,
    variables: {
      cluster: cluster,
      metrics: [selectedMetric],
      from: from,
      to: to,
    },
  }));

  const mappedData = $derived(handleQueryData($nodesQuery?.data));
  const filteredData = $derived(mappedData.filter((h) => h.host.includes(hostnameFilter)));

  /* Functions */
  function handleQueryData(queryData) {
    let rawData = []
    if (queryData) { 
      rawData = queryData.nodeMetrics.filter((h) => {
        if (h.subCluster !== '') { // Exclude nodes with empty subCluster field
          return h.metrics.some(
            (m) =>  m?.name == selectedMetric && m.scope == "node",
          )
        };
      });
    };
    
    let pendingMapped = [];
    if (rawData.length > 0) {
      pendingMapped = rawData.map((h) => ({
        host: h.host,
        subCluster: h.subCluster,
        data: h.metrics.filter(
          (m) => m?.name == selectedMetric && m.scope == "node",
        ),
        disabled: checkMetricDisabled(
          selectedMetric,
          cluster,
          h.subCluster,
        ),
      }))
      .sort((a, b) => a.host.localeCompare(b.host))
    }
    
    return pendingMapped;
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
        {#key item?.disabled}
          {#if item?.disabled === true}
            <Card body class="mx-3" color="info"
              >Metric disabled for subcluster <code
                >{selectedMetric}:{item.subCluster}</code
              ></Card
            >
          {:else if item?.disabled === false}
            <!-- "No Data"-Warning included in MetricPlot-Component   -->
            <!-- #key: X-axis keeps last selected timerange otherwise -->
            <MetricPlot
              timestep={item.data[0].metric.timestep}
              series={item.data[0].metric.series}
              metric={item.data[0].name}
              {cluster}
              subCluster={item.subCluster}
              forNode
            />
          {:else}
            <Card body class="mx-3"
              >Metric Query Empty: Please Reload Page <code
                >{selectedMetric}:{item.subCluster}</code
              ></Card
            >
          {/if}
        {/key}
      </Col>
    {/each}
  </Row>
{/if}