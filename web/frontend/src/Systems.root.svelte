<!--
  @component Main cluster node status view component; renders overview or list depending on type

  Properties:
  - `displayType String?`: The type of node display ['OVERVIEW' || 'LIST']
  - `cluster String`: The cluster to show status information for [Default: null]
  - `subCluster String`: The subCluster to show status information for [Default: null]
  - `presetFrom Date?`: Custom Time Range selection 'from' [Default: null]
  - `presetTo Date?`: Custom Time Range selection 'to' [Default: null]
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
  import {
    gql,
    getContextClient,
    mutationStore,
  } from "@urql/svelte";

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
    presetFrom = null,
    presetTo = null,
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const client = getContextClient();
  const stateOptions = ['all', 'allocated', 'idle', 'reserved', 'mixed', 'down', 'unknown', 'notindb'];
  const nowDate = new Date(Date.now());

  /* Var Init */
  let timeoutId = null;

  /* State Init */
  let hostnameFilter = $state("");
  let hoststateFilter = $state("all");
  let pendingHostnameFilter = $state("");
  let isMetricsSelectionOpen = $state(false);

  /* Derived Init Return */
  const thisInit = $derived($initq?.data ? true : false);

  /* Derived States */
  const ccconfig = $derived(thisInit ? getContext("cc-config") : null);
  const globalMetrics = $derived(thisInit ? getContext("globalMetrics") : null);
  const resampleConfig = $derived(thisInit ? getContext("resampling") : null);
  const resampleResolutions = $derived(resampleConfig ? [...resampleConfig.resolutions] : []);
  const resampleDefault = $derived(resampleConfig ? Math.max(...resampleConfig.resolutions) : 0);
  const displayNodeOverview = $derived((displayType === 'OVERVIEW'));

  const systemMetrics = $derived(globalMetrics ? [...globalMetrics.filter((gm) => gm?.availability.find((av) => av.cluster == cluster))] : []);
  const systemUnits = $derived.by(() => {
    const pendingUnits = {};
    if (thisInit && systemMetrics.length > 0) {
      for (let sm of systemMetrics) {
        pendingUnits[sm.name] = (sm?.unit?.prefix ? sm.unit.prefix : "") + (sm?.unit?.base ? sm.unit.base : "")
      };
    }
    return {...pendingUnits};
  });

  let selectedResolution = $derived(resampleDefault);
  let to = $derived(presetTo ? presetTo : new Date(Date.now()));
  let from = $derived(presetFrom ? presetFrom : new Date(nowDate.setHours(nowDate.getHours() - 4)));

  let selectedMetric = $derived.by(() => {
    let configKey = `nodeOverview_selectedMetric`;
    if (cluster) configKey += `:${cluster}`;
    if (subCluster) configKey += `:${subCluster}`;

    if (thisInit) {
      if (ccconfig[configKey]) return ccconfig[configKey]
      else if (systemMetrics.length !== 0) return systemMetrics[0].name
    }
    return ""
  });

  let selectedMetrics = $derived.by(() => {
    let configKey = `nodeList_selectedMetrics`;
    if (cluster) configKey += `:${cluster}`;
    if (subCluster) configKey += `:${subCluster}`;

    if (thisInit) {
      if (ccconfig[configKey]) return ccconfig[configKey]
      else if (systemMetrics.length >= 3) return [systemMetrics[0].name, systemMetrics[1].name, systemMetrics[2].name]
    }
    return []
  });

  /* Effects */
  $effect(() => {
    if (displayNodeOverview) {
      updateOverviewMetric(selectedMetric)
    }
  });

  /* Functions */
  // Wait after input for some time to prevent too many requests
  function updateHostnameFilter() {
    if (timeoutId != null) clearTimeout(timeoutId);
    timeoutId = setTimeout(function () {
      hostnameFilter = pendingHostnameFilter;
    }, 500);
  };

  function updateOverviewMetric(newMetric) {
    let configKey = `nodeOverview_selectedMetric`;
    if (cluster) configKey += `:${cluster}`;
    if (subCluster) configKey += `:${subCluster}`;

    updateConfigurationMutation({
      name: configKey,
      value: JSON.stringify(newMetric),
    }).subscribe((res) => {
      if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  };

  const updateConfigurationMutation = ({ name, value }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($name: String!, $value: String!) {
          updateConfiguration(name: $name, value: $value)
        }
      `,
      variables: { name, value },
    });
  };

</script>

<!-- ROW1: Tools-->
<Row cols={{ xs: 2, lg: !displayNodeOverview ? (resampleConfig ? 6 : 5) : 5 }} class="mb-3">
  {#if thisInit}
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
        <InputGroupText>Node(s)</InputGroupText>
        <Input
          placeholder="Filter hostname ..."
          type="text"
          bind:value={pendingHostnameFilter}
          oninput={updateHostnameFilter}
        />
      </InputGroup>
    </Col>
    <!-- State Col-->
    <Col class="mt-2 mt-lg-0">
      <InputGroup>
        <InputGroupText><Icon name="clipboard2-pulse" /></InputGroupText>
        <InputGroupText>State</InputGroupText>
        <Input type="select" bind:value={hoststateFilter}>
          {#each stateOptions as so}
            <option value={so}>{so.charAt(0).toUpperCase() + so.slice(1)}</option>
          {/each}
        </Input>
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
                >{metric.name} {systemUnits[metric.name] ? "("+systemUnits[metric.name]+")" : ""}</option
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
    <NodeOverview {cluster} {ccconfig} {selectedMetric} {globalMetrics} {from} {to} {hostnameFilter} {hoststateFilter}/>
  {:else}
    <!-- ROW2-2: Node List (Grid Included)-->
    <NodeList {cluster} {subCluster} {ccconfig} {globalMetrics}
      pendingSelectedMetrics={selectedMetrics} {selectedResolution} {hostnameFilter} {hoststateFilter} {from} {to} {systemUnits}/>
  {/if}
{/if}

{#if !displayNodeOverview}
  <MetricSelection
    bind:isOpen={isMetricsSelectionOpen}
    presetMetrics={selectedMetrics}
    {cluster}
    {subCluster}
    {globalMetrics}
    configName="nodeList_selectedMetrics"
    applyMetrics={(newMetrics) => 
      selectedMetrics = [...newMetrics]
    }
  />
{/if}
