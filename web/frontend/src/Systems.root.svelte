<!--
    @component Main cluster metric status view component; renders current state of metrics / nodes

    Properties:
    - `cluster String`: The cluster to show status information for
    - `from Date?`: Custom Time Range selection 'from' [Default: null]
    - `to Date?`: Custom Time Range selection 'to' [Default: null]
 -->

<script>
  import { getContext } from "svelte";
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
  import PlotTable from "./generic/PlotTable.svelte";
  import MetricPlot from "./generic/plots/MetricPlot.svelte";
  import TimeSelection from "./generic/select/TimeSelection.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";

  export let cluster;
  export let from = null;
  export let to = null;

  const { query: initq } = init();

  if (from == null || to == null) {
    to = new Date(Date.now());
    from = new Date(to.getTime());
    from.setHours(from.getHours() - 12);
  }

  const initialized = getContext("initialized");
  const ccconfig = getContext("cc-config");
  const clusters = getContext("clusters");
  const globalMetrics = getContext("globalMetrics");

  let plotHeight = 300;
  let hostnameFilter = "";
  let selectedMetric = ccconfig.system_view_selectedMetric;

  const client = getContextClient();
  $: nodesQuery = queryStore({
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
      from: from.toISOString(),
      to: to.toISOString(),
    },
  });

  let systemMetrics = [];
  let systemUnits = {};
  function loadMetrics(isInitialized) {
    if (!isInitialized) return
    systemMetrics = [...globalMetrics.filter((gm) => gm?.availability.find((av) => av.cluster == cluster))]
    for (let sm of systemMetrics) {
      systemUnits[sm.name] = (sm?.unit?.prefix ? sm.unit.prefix : "") + (sm?.unit?.base ? sm.unit.base : "")
    }
  }

  $: loadMetrics($initialized)

</script>

<Row>
  {#if $initq.error}
    <Card body color="danger">{$initq.error.message}</Card>
  {:else if $initq.fetching}
    <Spinner />
  {:else}
    <!-- Node Col-->
    <Col>
      <InputGroup>
        <InputGroupText><Icon name="hdd" /></InputGroupText>
        <InputGroupText>Find Node</InputGroupText>
        <Input
          placeholder="hostname..."
          type="text"
          bind:value={hostnameFilter}
        />
      </InputGroup>
    </Col>
    <!-- Range Col-->
    <Col>
      <TimeSelection bind:from bind:to />
    </Col>
    <!-- Metric Col-->
    <Col>
      <InputGroup>
        <InputGroupText><Icon name="graph-up" /></InputGroupText>
        <InputGroupText>Metric</InputGroupText>
        <select class="form-select" bind:value={selectedMetric}>
          {#each systemMetrics as metric}
            <option value={metric.name}
              >{metric.name} {systemUnits[metric.name] ? "("+systemUnits[metric.name]+")" : ""}</option
            >
          {/each}
        </select>
      </InputGroup>
    </Col>
    <!-- Refresh Col-->
    <Col>
      <Refresher
        on:refresh={() => {
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
    {#if $nodesQuery.error}
      <Card body color="danger">{$nodesQuery.error.message}</Card>
    {:else if $nodesQuery.fetching || $initq.fetching}
      <Spinner />
    {:else}
      <PlotTable
        let:item
        let:width
        renderFor="systems"
        itemsPerRow={ccconfig.plot_view_plotsPerRow}
        items={$nodesQuery.data.nodeMetrics
          .filter(
            (h) =>
              h.host.includes(hostnameFilter) &&
              h.metrics.some(
                (m) => m.name == selectedMetric && m.scope == "node",
              ),
          )
          .map((h) => ({
            host: h.host,
            subCluster: h.subCluster,
            data: h.metrics.find(
              (m) => m.name == selectedMetric && m.scope == "node",
            ),
            disabled: checkMetricDisabled(
              selectedMetric,
              cluster,
              h.subCluster,
            ),
          }))
          .sort((a, b) => a.host.localeCompare(b.host))}
      >
        <h4 style="width: 100%; text-align: center;">
          <a
            style="display: block;padding-top: 15px;"
            href="/monitoring/node/{cluster}/{item.host}"
            >{item.host} ({item.subCluster})</a
          >
        </h4>
        {#if item.disabled === false && item.data}
          <MetricPlot
            {width}
            height={plotHeight}
            timestep={item.data.metric.timestep}
            series={item.data.metric.series}
            metric={item.data.name}
            cluster={clusters.find((c) => c.name == cluster)}
            subCluster={item.subCluster}
            forNode={true}
          />
        {:else if item.disabled === true && item.data}
          <Card style="margin-left: 2rem;margin-right: 2rem;" body color="info"
            >Metric disabled for subcluster <code
              >{selectedMetric}:{item.subCluster}</code
            ></Card
          >
        {:else}
          <Card
            style="margin-left: 2rem;margin-right: 2rem;"
            body
            color="warning"
            >No dataset returned for <code>{selectedMetric}</code></Card
          >
        {/if}
      </PlotTable>
    {/if}
  </Col>
</Row>
