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

  /*
    Note 1: "Sorting" as use-case ignored for now, probably default to alphanumerical on hostnames of cluster (handled in frontend at the moment)
    Note 2: Add Idle State Filter (== No allocated Jobs) [Frontend?] : Cannot be handled by CCMS, requires secondary job query and refiltering of visible nodes
  */

  /* Scelte 5 Props */
  let {
    displayType,
    cluster = null,
    subCluster = null,
    fromPreset = null,
    toPreset = null,
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const displayNodeOverview = (displayType === 'OVERVIEW');
  const ccconfig = getContext("cc-config");
  const initialized = getContext("initialized");
  const globalMetrics = getContext("globalMetrics");
  const resampleConfig = getContext("resampling") || null;

  const resampleResolutions = resampleConfig ? [...resampleConfig.resolutions] : [];
  const resampleDefault = resampleConfig ? Math.max(...resampleConfig.resolutions) : 0;
  const nowDate = new Date(Date.now());

  /* State Init */
  let to = $state(toPreset || new Date(Date.now()));
  let from = $state(fromPreset || new Date(nowDate.setHours(nowDate.getHours() - 4)));
  let selectedResolution = $state(resampleConfig ? resampleDefault : 0);
  let hostnameFilter = $state("");
  let pendingHostnameFilter = $state("");
  let isMetricsSelectionOpen = $state(false);
  let selectedMetric = $state(ccconfig.system_view_selectedMetric || "");
  let selectedMetrics = $state((
    ccconfig[`node_list_selectedMetrics:${cluster}:${subCluster}`] ||
    ccconfig[`node_list_selectedMetrics:${cluster}`]
  ) || [ccconfig.system_view_selectedMetric]);

  /* Derived States */
  const systemMetrics = $derived($initialized ? [...globalMetrics.filter((gm) => gm?.availability.find((av) => av.cluster == cluster))] : []);
  const presetSystemUnits = $derived(loadUnits(systemMetrics));

  /* Effects */
  $effect(() => {
    // OnMount: Ping Var, without this, OVERVIEW metric select is empty (reason tbd) 
    systemMetrics
  });

  /* Functions */
  function loadUnits(systemMetrics) {
    let pendingUnits = {};
    if (systemMetrics.length > 0) {
      for (let sm of systemMetrics) {
        pendingUnits[sm.name] = (sm?.unit?.prefix ? sm.unit.prefix : "") + (sm?.unit?.base ? sm.unit.base : "")
      };
    };
    return {...pendingUnits};
  };

  // Wait after input for some time to prevent too many requests
  let timeoutId = null;
  function updateHostnameFilter() {
    if (timeoutId != null) clearTimeout(timeoutId);
    timeoutId = setTimeout(function () {
      hostnameFilter = pendingHostnameFilter;
    }, 500);
  };
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
            onclick={() => (isMetricsSelectionOpen = true)}
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
          oninput={updateHostnameFilter}
        />
      </InputGroup>
    </Col>
    <!-- Range Col-->
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
    <!-- Overview Metric Col-->
    {#if displayNodeOverview}
      <Col class="mt-2 mt-lg-0">
        <InputGroup>
          <InputGroupText><Icon name="graph-up" /></InputGroupText>
          <InputGroupText>Metric</InputGroupText>
          <Input type="select" bind:value={selectedMetric}>
            {#each systemMetrics as metric (metric.name)}
              <option value={metric.name}
                >{metric.name} {presetSystemUnits[metric.name] ? "("+presetSystemUnits[metric.name]+")" : ""}</option
              >
            {:else}
              <option disabled>No available options</option>
            {/each}
          </Input>
        </InputGroup>
      </Col>
    {/if}
  {/if}
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
    <NodeOverview {cluster} {ccconfig} {selectedMetric} {from} {to} {hostnameFilter}/>
  {:else}
    <!-- ROW2-2: Node List (Grid Included)-->
    <NodeList {cluster} {subCluster} {ccconfig} {selectedMetrics} {selectedResolution} {hostnameFilter} {from} {to} {presetSystemUnits}/>
  {/if}
{/if}

{#if !displayNodeOverview}
  <MetricSelection
    bind:isOpen={isMetricsSelectionOpen}
    presetMetrics={selectedMetrics}
    {cluster}
    {subCluster}
    configName="node_list_selectedMetrics"
    applyMetrics={(newMetrics) => 
      selectedMetrics = [...newMetrics]
    }
  />
{/if}
