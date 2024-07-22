<script>
  import { init, checkMetricDisabled } from "./utils.js";
  import {
    Row,
    Col,
    InputGroup,
    InputGroupText,
    Icon,
    Spinner,
    Card,
  } from "@sveltestrap/sveltestrap";
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import TimeSelection from "./filters/TimeSelection.svelte";
  import Refresher from "./joblist/Refresher.svelte";
  import PlotTable from "./PlotTable.svelte";
  import MetricPlot from "./plots/MetricPlot.svelte";
  import { getContext } from "svelte";

  export let cluster;
  export let hostname;
  export let from = null;
  export let to = null;

  const { query: initq } = init();

  if (from == null || to == null) {
    to = new Date(Date.now());
    from = new Date(to.getTime());
    from.setMinutes(from.getMinutes() - 30);
  }

  const initialized = getContext("initialized")
  const globalMetrics = getContext("globalMetrics")
  const ccconfig = getContext("cc-config");
  const clusters = getContext("clusters");
  const client = getContextClient();
  const nodeMetricsQuery = gql`
    query ($cluster: String!, $nodes: [String!], $from: Time!, $to: Time!) {
      nodeMetrics(cluster: $cluster, nodes: $nodes, from: $from, to: $to) {
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
  `;

  $: nodeMetricsData = queryStore({
    client: client,
    query: nodeMetricsQuery,
    variables: {
      cluster: cluster,
      nodes: [hostname],
      from: from.toISOString(),
      to: to.toISOString(),
    },
  });

  let itemsPerPage = ccconfig.plot_list_jobsPerPage;
  let page = 1;
  let paging = { itemsPerPage, page };
  let sorting = { field: "startTime", type: "col", order: "DESC" };
  $: filter = [
    { cluster: { eq: cluster } },
    { node: { contains: hostname } },
    { state: ["running"] },
  ];

  const nodeJobsQuery = gql`
    query (
      $filter: [JobFilter!]!
      $sorting: OrderByInput!
      $paging: PageRequest!
    ) {
      jobs(filter: $filter, order: $sorting, page: $paging) {
        count
      }
    }
  `;

  $: nodeJobsData = queryStore({
    client: client,
    query: nodeJobsQuery,
    variables: { paging, sorting, filter },
  });

  let systemUnits = {};
  function loadUnits(isInitialized) {
    if (!isInitialized) return
    const systemMetrics = [...globalMetrics.filter((gm) => gm?.availability.find((av) => av.cluster == cluster))]
    for (let sm of systemMetrics) {
      systemUnits[sm.name] = (sm?.unit?.prefix ? sm.unit.prefix : "") + (sm?.unit?.base ? sm.unit.base : "")
    }
  }

  $: loadUnits($initialized)
</script>

<Row>
  {#if $initq.error}
    <Card body color="danger">{$initq.error.message}</Card>
  {:else if $initq.fetching}
    <Spinner />
  {:else}
    <Col>
      <InputGroup>
        <InputGroupText><Icon name="hdd" /></InputGroupText>
        <InputGroupText>{hostname} ({cluster})</InputGroupText>
      </InputGroup>
    </Col>
    <Col>
      {#if $nodeJobsData.fetching}
        <Spinner />
      {:else if $nodeJobsData.data}
        Currently running jobs on this node: {$nodeJobsData.data.jobs.count}
        [
        <a
          href="/monitoring/jobs/?cluster={cluster}&state=running&node={hostname}"
          target="_blank">View in Job List</a
        > ]
      {:else}
        No currently running jobs.
      {/if}
    </Col>
    <Col>
      <Refresher
        on:reload={() => {
          const diff = Date.now() - to;
          from = new Date(from.getTime() + diff);
          to = new Date(to.getTime() + diff);
        }}
      />
    </Col>
    <Col>
      <TimeSelection bind:from bind:to />
    </Col>
  {/if}
</Row>
<br />
<Row>
  <Col>
    {#if $nodeMetricsData.error}
      <Card body color="danger">{$nodeMetricsData.error.message}</Card>
    {:else if $nodeMetricsData.fetching || $initq.fetching}
      <Spinner />
    {:else}
      <PlotTable
        let:item
        let:width
        renderFor="node"
        itemsPerRow={ccconfig.plot_view_plotsPerRow}
        items={$nodeMetricsData.data.nodeMetrics[0].metrics
          .map((m) => ({
            ...m,
            disabled: checkMetricDisabled(
              m.name,
              cluster,
              $nodeMetricsData.data.nodeMetrics[0].subCluster,
            ),
          }))
          .sort((a, b) => a.name.localeCompare(b.name))}
      >
        <h4 style="text-align: center; padding-top:15px;">
          {item.name}
          {systemUnits[item.name] ? "(" + systemUnits[item.name] + ")" : ""}
        </h4>
        {#if item.disabled === false && item.metric}
          <MetricPlot
            {width}
            height={300}
            metric={item.name}
            timestep={item.metric.timestep}
            cluster={clusters.find((c) => c.name == cluster)}
            subCluster={$nodeMetricsData.data.nodeMetrics[0].subCluster}
            series={item.metric.series}
            resources={[{ hostname: hostname }]}
            forNode={true}
          />
        {:else if item.disabled === true && item.metric}
          <Card style="margin-left: 2rem;margin-right: 2rem;" body color="info"
            >Metric disabled for subcluster <code
              >{item.name}:{$nodeMetricsData.data.nodeMetrics[0]
                .subCluster}</code
            ></Card
          >
        {:else}
          <Card
            style="margin-left: 2rem;margin-right: 2rem;"
            body
            color="warning"
            >No dataset returned for <code>{item.name}</code></Card
          >
        {/if}
      </PlotTable>
    {/if}
  </Col>
</Row>
