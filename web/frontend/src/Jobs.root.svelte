<!--
    @component Main job list component

    Properties:
    - `filterPresets Object?`: Optional predefined filter values [Default: {}]
    - `authlevel Number`: The current users authentication level
    - `roles [Number]`: Enum containing available roles
 -->
 
 <script>
  import { onMount, getContext } from "svelte";
  import {
    Row,
    Col,
    Button,
    Icon,
    Card,
    Spinner,
  } from "@sveltestrap/sveltestrap";
  import { init } from "./generic/utils.js";
  import Filters from "./generic/Filters.svelte";
  import JobList from "./generic/JobList.svelte";
  import TextFilter from "./generic/helper/TextFilter.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";
  import Sorting from "./generic/select/SortSelection.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";

  const { query: initq } = init();

  const ccconfig = getContext("cc-config");

  export let filterPresets = {};
  export let authlevel;
  export let roles;

  let filterComponent; // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
  let jobList,
    matchedJobs = null;
  let sorting = { field: "startTime", type: "col", order: "DESC" },
    isSortingOpen = false,
    isMetricsSelectionOpen = false;
  let metrics = filterPresets.cluster
    ? ccconfig[`plot_list_selectedMetrics:${filterPresets.cluster}`] ||
      ccconfig.plot_list_selectedMetrics
    : ccconfig.plot_list_selectedMetrics;
  let showFootprint = filterPresets.cluster
    ? !!ccconfig[`plot_list_showFootprint:${filterPresets.cluster}`]
    : !!ccconfig.plot_list_showFootprint;
  let selectedCluster = filterPresets?.cluster ? filterPresets.cluster : null;
  let presetProject = filterPresets?.project ? filterPresets.project : ""

  // The filterPresets are handled by the Filters component,
  // so we need to wait for it to be ready before we can start a query.
  // This is also why JobList component starts out with a paused query.
  onMount(() => filterComponent.updateFilters());
</script>

<Row>
  {#if $initq.fetching}
    <Col xs="auto">
      <Spinner />
    </Col>
  {:else if $initq.error}
    <Col xs="auto">
      <Card body color="danger">{$initq.error.message}</Card>
    </Col>
  {/if}
</Row>
<Row>
  <Col xs="auto">
    <Button outline color="primary" on:click={() => (isSortingOpen = true)}>
      <Icon name="sort-up" /> Sorting
    </Button>
    <Button
      outline
      color="primary"
      on:click={() => (isMetricsSelectionOpen = true)}
    >
      <Icon name="graph-up" /> Metrics
    </Button>
    <Button disabled outline
      >{matchedJobs == null ? "Loading..." : `${matchedJobs} jobs`}</Button
    >
  </Col>
  <Col xs="auto">
    <Filters
      {filterPresets}
      bind:this={filterComponent}
      on:update-filters={({ detail }) => {
        selectedCluster = detail.filters[0]?.cluster
          ? detail.filters[0].cluster.eq
          : null;
        jobList.queryJobs(detail.filters);
      }}
    />
  </Col>

  <Col xs="3" style="margin-left: auto;">
    <TextFilter
      {presetProject}
      bind:authlevel
      bind:roles
      on:set-filter={({ detail }) => filterComponent.updateFilters(detail)}
    />
  </Col>
  <Col xs="2">
    <Refresher on:refresh={() => {
      jobList.refreshJobs()
      jobList.refreshAllMetrics()
    }} />
  </Col>
</Row>
<br />
<Row>
  <Col>
    <JobList
      bind:metrics
      bind:sorting
      bind:matchedJobs
      bind:this={jobList}
      bind:showFootprint
    />
  </Col>
</Row>

<Sorting bind:sorting bind:isOpen={isSortingOpen} />

<MetricSelection
  bind:cluster={selectedCluster}
  configName="plot_list_selectedMetrics"
  bind:metrics
  bind:isOpen={isMetricsSelectionOpen}
  bind:showFootprint
  footprintSelect={true}
/>
