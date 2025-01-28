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
  import {
    Row,
    Col,
    Card,
    Input,
    InputGroup,
    InputGroupText,
    Icon,
    Button,
  } from "@sveltestrap/sveltestrap";

  import { init } from "./generic/utils.js";
  import NodeOverview from "./systems/NodeOverview.svelte";
  import NodeList from "./systems/NodeList.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";
  import TimeSelection from "./generic/select/TimeSelection.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";

  export let displayType;
  export let cluster;
  export let subCluster = "";
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
    from.setHours(from.getHours() - 12);
  }

  const initialized = getContext("initialized");
  const ccconfig = getContext("cc-config");
  const globalMetrics = getContext("globalMetrics");
  const displayNodeOverview = (displayType === 'OVERVIEW')

  const resampleConfig = getContext("resampling") || null;
  const resampleResolutions = resampleConfig ? [...resampleConfig.resolutions] : [];
  const resampleDefault = resampleConfig ? Math.max(...resampleConfig.resolutions) : 0;
  let selectedResolution = resampleConfig ? resampleDefault : 0;

  let hostnameFilter = "";
  let pendingHostnameFilter = "";
  let selectedMetric = ccconfig.system_view_selectedMetric || "";
  let selectedMetrics = ccconfig[`node_list_selectedMetrics:${cluster}`] || [ccconfig.system_view_selectedMetric];
  let isMetricsSelectionOpen = false;

  /*
    Note 1: "Sorting" as use-case ignored for now, probably default to alphanumerical on hostnames of cluster (handled in frontend at the moment)
    Note 2: Add Idle State Filter (== No allocated Jobs) [Frontend?] : Cannot be handled by CCMS, requires secondary job query and refiltering of visible nodes
  */

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

  $: { // Wait after input for some time to prevent too many requests
    setTimeout(function () {
      hostnameFilter = pendingHostnameFilter;
    }, 500);
  }
</script>

<!-- ROW1: Tools-->
<Row cols={{ xs: 2, lg: !displayNodeOverview ? (resampleConfig ? 5 : 4) : 4 }} class="mb-3">
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
      {#if resampleConfig}
        <Col>
          <InputGroup>
            <InputGroupText><Icon name="plus-slash-minus" /></InputGroupText>
            <InputGroupText>Resolution</InputGroupText>
            <Input type="select" bind:value={selectedResolution}>
              {#each resampleResolutions as res}
                <option value={res}
                  >{res} sec</option
                >
              {/each}
            </Input>
          </InputGroup>
        </Col>
      {/if}
    {/if}
    <!-- Node Col-->
    <Col class="mt-2 mt-lg-0">
      <InputGroup>
        <InputGroupText><Icon name="hdd" /></InputGroupText>
        <InputGroupText>Find Node(s)</InputGroupText>
        <Input
          placeholder="Filter hostname ..."
          type="text"
          bind:value={pendingHostnameFilter}
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
{:else}
  {#if displayNodeOverview}
    <!-- ROW2-1: Node Overview (Grid Included)-->
    <NodeOverview {cluster} {subCluster} {ccconfig} {selectedMetrics} {from} {to} {hostnameFilter}/>
  {:else}
    <!-- ROW2-2: Node List (Grid Included)-->
    <NodeList {cluster} {subCluster} {ccconfig} {selectedMetrics} {selectedResolution} {hostnameFilter} {from} {to} {systemUnits}/>
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
