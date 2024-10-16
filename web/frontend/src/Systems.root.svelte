<!--
    @component Main cluster node status view component; renders overview or list depending on type

    Properties:
    - `displayType String?`: The type of node display ['OVERVIEW' || 'LIST']
    - `cluster String`: The cluster to show status information for
    - `from Date?`: Custom Time Range selection 'from' [Default: null]
    - `to Date?`: Custom Time Range selection 'to' [Default: null]
 -->

<script>
  import { getContext } from "svelte";
  import { queryStore, gql, getContextClient } from "@urql/svelte"
  import {
    Row,
    Col,
    Card,
    Input,
    InputGroup,
    InputGroupText,
    Icon,
    Button,
    Spinner,
  } from "@sveltestrap/sveltestrap";

  import { init, checkMetricsDisabled } from "./generic/utils.js";
  import NodeOverview from "./systems/NodeOverview.svelte";
  import NodeList from "./systems/NodeList.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";
  import TimeSelection from "./generic/select/TimeSelection.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";

  export let displayType;
  export let cluster;
  export let from = null;
  export let to = null;

  const { query: initq } = init();

  console.assert(
    displayType == "OVERVIEW" || displayType == "LIST",
    "Invalid nodes displayType provided!",
  );

  if (from == null || to == null) {
    to = new Date(Date.now());
    from = new Date(to.getTime());
    from.setHours(from.getHours() - 2);
  }

  const initialized = getContext("initialized");
  const ccconfig = getContext("cc-config");
  const globalMetrics = getContext("globalMetrics");
  const displayNodeOverview = (displayType === 'OVERVIEW')

  let hostnameFilter = "";
  let selectedMetric = ccconfig.system_view_selectedMetric || "";
  let selectedMetrics = ccconfig[`node_list_selectedMetrics:${cluster}`] || [ccconfig.system_view_selectedMetric];
  let isMetricsSelectionOpen = false;

  // Todo: Add Idle State Filter (== No allocated Jobs)
  // Todo: NodeList: Mindestens Accelerator Scope ... "Show Detail" Switch?
  // Todo: Review performance // observed high client-side load frequency
  //       Is Svelte {#each} -> <MetricPlot/> -> onMount() related : Cannot be skipped ...
  
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

  $: if (displayNodeOverview) {
    selectedMetrics = [selectedMetric]
  }

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

<!-- ROW1: Tools-->
<Row cols={{ xs: 2, lg: 4 }} class="mb-3">
  {#if $initq.data}
    <!-- List Metric Select Col-->
    {#if !displayNodeOverview}
      <Col>
        <InputGroup>
          <InputGroupText><Icon name="graph-up" /></InputGroupText>
          <InputGroupText class="text-capitalize">Metrics</InputGroupText>
          <Button 
            outline
            color="primary"
            on:click={() => (isMetricsSelectionOpen = true)}
          >
            {selectedMetrics.length} selected
          </Button>
        </InputGroup>
      </Col> 
    {/if}
    <!-- Node Col-->
    <Col>
      <InputGroup>
        <InputGroupText><Icon name="hdd" /></InputGroupText>
        <InputGroupText>Find Node(s)</InputGroupText>
        <Input
          placeholder="Filter hostname ..."
          type="text"
          bind:value={hostnameFilter}
        />
      </InputGroup>
    </Col>
    <!-- Range Col-->
    <Col>
      <TimeSelection bind:from bind:to />
    </Col>
    <!-- Overview Metric Col-->
    {#if displayNodeOverview}
      <Col class="mt-2 mt-lg-0">
        <InputGroup>
          <InputGroupText><Icon name="graph-up" /></InputGroupText>
          <InputGroupText>Metric</InputGroupText>
          <Input type="select" bind:value={selectedMetric}>
            {#each systemMetrics as metric}
              <option value={metric.name}
                >{metric.name} {systemUnits[metric.name] ? "("+systemUnits[metric.name]+")" : ""}</option
              >
            {/each}
          </Input>
        </InputGroup>
      </Col>
    {/if}
    <!-- Refresh Col-->
    <Col class="mt-2 mt-lg-0">
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

<!-- ROW2: Content-->
{#if displayType !== "OVERVIEW" && displayType !== "LIST"}
  <Row>
    <Col>
      <Card body color="danger">Unknown displayList type! </Card>
    </Col>
  </Row>
{:else if $nodesQuery.error}
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
  {#if displayNodeOverview}
    <!-- ROW2-1: Node Overview (Grid Included)-->
    <NodeOverview {cluster} {ccconfig} data={filteredData}/>
  {:else}
    <!-- ROW2-2: Node List (Grid Included)-->
    <NodeList {cluster} {selectedMetrics} {systemUnits} data={filteredData} bind:selectedMetric/>
  {/if}
{/if}

<MetricSelection
  {cluster}
  configName="node_list_selectedMetrics"
  metrics={selectedMetrics}
  bind:isOpen={isMetricsSelectionOpen}
  on:update-metrics={({ detail }) => {
    selectedMetrics = [...detail]
  }}
/>
