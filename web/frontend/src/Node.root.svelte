<!--
    @component System-View subcomponent; renders all current metrics for specified node

    Properties:
    - `cluster String`: Currently selected cluster
    - `hostname String`: Currently selected host (== node)
    - `from Date?`: Custom Time Range selection 'from' [Default: null]
    - `to Date?`: Custom Time Range selection 'to' [Default: null]
 -->

<script>
  import { 
    getContext,
  } from "svelte";
  import {
    Row,
    Col,
    Input,
    InputGroup,
    InputGroupText,
    Icon,
    Spinner,
    Card,
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    checkMetricDisabled,
  } from "./generic/utils.js";
  import PlotGrid from "./generic/PlotGrid.svelte";
  import MetricPlot from "./generic/plots/MetricPlot.svelte";
  import TimeSelection from "./generic/select/TimeSelection.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";

  /* Svelte 5 Props */
  let {
    cluster,
    hostname,
    presetFrom = null,
    presetTo = null,
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const initialized = getContext("initialized")
  const globalMetrics = getContext("globalMetrics")
  const ccconfig = getContext("cc-config");
  const clusters = getContext("clusters");
  const nowEpoch = Date.now();
  const paging = { itemsPerPage: 50, page: 1 };
  const sorting = { field: "startTime", type: "col", order: "DESC" };
  const filter = [
    { cluster: { eq: cluster } },
    { node: { contains: hostname } },
    { state: ["running"] },
  ];
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

  /* State Init */
  let from = $state(presetFrom ? presetFrom : new Date(nowEpoch - (4 * 3600 * 1000)));
  let to = $state(presetTo ? presetTo : new Date(nowEpoch));
  let systemUnits = $state({});

  /* Derived */
  const nodeMetricsData = $derived(queryStore({
      client: client,
      query: nodeMetricsQuery,
      variables: {
        cluster: cluster,
        nodes: [hostname],
        from: from?.toISOString(),
        to: to?.toISOString(),
      },
    })
  );

  const nodeJobsData = $derived(queryStore({
      client: client,
      query: nodeJobsQuery,
      variables: { paging, sorting, filter },
    })
  );

  /* Effect */
  $effect(() => {
    loadUnits($initialized);
  });

  /* Functions */
  function loadUnits(isInitialized) {
    if (!isInitialized) return
    const systemMetrics = [...globalMetrics.filter((gm) => gm?.availability.find((av) => av.cluster == cluster))]
    for (let sm of systemMetrics) {
      systemUnits[sm.name] = (sm?.unit?.prefix ? sm.unit.prefix : "") + (sm?.unit?.base ? sm.unit.base : "")
    }
  }
</script>

<Row cols={{ xs: 2, lg: 4 }}>
  {#if $initq.error}
    <Card body color="danger">{$initq.error.message}</Card>
  {:else if $initq.fetching}
    <Spinner />
  {:else}
    <!-- Node Col -->
    <Col>
      <InputGroup>
        <InputGroupText><Icon name="hdd" /></InputGroupText>
        <InputGroupText>Selected Node</InputGroupText>
        <Input style="background-color: white;"type="text" value="{hostname} [{cluster} ({$nodeMetricsData?.data ? $nodeMetricsData.data.nodeMetrics[0].subCluster : ''})]" disabled/>
      </InputGroup>
    </Col>
    <!-- Time Col -->
    <Col>
      <TimeSelection
        presetFrom={from}
        presetTo={to}
        applyTime={(newFrom, newTo) => {
          from = newFrom;
          to = newTo;
        }}
      />
    </Col>
    <!-- Concurrent Col -->
    <Col class="mt-2 mt-lg-0">
      {#if $nodeJobsData.fetching}
        <Spinner />
      {:else if $nodeJobsData.data}
        <InputGroup>
          <InputGroupText><Icon name="activity" /></InputGroupText>
          <InputGroupText>Activity</InputGroupText>
          <Input style="background-color: white;" type="text" value="{$nodeJobsData.data.jobs.count} Jobs" disabled/>
          <a title="Show jobs running on this node" href="/monitoring/jobs/?cluster={cluster}&state=running&node={hostname}" target="_blank" class="btn btn-outline-secondary" role="button" aria-disabled="true">
            <Icon name="view-list" /> Show List
          </a>
        </InputGroup>
      {:else}
        <InputGroup>
          <InputGroupText><Icon name="activity" /></InputGroupText>
          <InputGroupText>Activity</InputGroupText>
          <Input type="text" value="No running jobs." disabled />
        </InputGroup>
      {/if}
    </Col>
    <!-- Refresh Col-->
    <Col class="mt-2 mt-lg-0">
      <Refresher
        onRefresh={() => {
          const diff = Date.now() - to;
          from = new Date(from.getTime() + diff);
          to = new Date(to.getTime() + diff);
        }}
      />
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
      <!-- Note: Ignore '#snippet' Error in IDE -->
      {#snippet gridContent(item)}
        <h4 style="text-align: center; padding-top:15px;">
          {item.name}
          {systemUnits[item.name] ? "(" + systemUnits[item.name] + ")" : ""}
        </h4>
        {#if item.disabled === false && item.metric}
          <MetricPlot
            metric={item.name}
            timestep={item.metric.timestep}
            cluster={clusters.find((c) => c.name == cluster)}
            subCluster={$nodeMetricsData.data.nodeMetrics[0].subCluster}
            series={item.metric.series}
            forNode
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
      {/snippet}

      <PlotGrid
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
        itemsPerRow={ccconfig.plot_view_plotsPerRow}
        {gridContent}
      />
    {/if}
  </Col>
</Row>
