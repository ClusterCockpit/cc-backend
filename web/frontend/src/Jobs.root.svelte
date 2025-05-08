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
    ButtonGroup,
    Icon,
    Card,
    Spinner,
  } from "@sveltestrap/sveltestrap";
  import { init } from "./generic/utils.js";
  import Filters from "./generic/Filters.svelte";
  import JobList from "./generic/JobList.svelte";
  import JobCompare from "./generic/JobCompare.svelte";
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
  let filterBuffer = [];
  let selectedJobs = [];
  let jobList,
      jobCompare,
    matchedListJobs,
    matchedCompareJobs = null;
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
  let showCompare = false;

  // The filterPresets are handled by the Filters component,
  // so we need to wait for it to be ready before we can start a query.
  // This is also why JobList component starts out with a paused query.
  onMount(() => filterComponent.updateFilters());

  $: if (filterComponent && selectedJobs.length == 0) {
    filterComponent.updateFilters({dbId: []})
  }
</script>

<!-- ROW1: Status-->
{#if $initq.fetching}
  <Row class="mb-3">
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if $initq.error}
  <Row class="mb-3">
    <Col>
      <Card body color="danger">{$initq.error.message}</Card>
    </Col>
  </Row>
{/if}

<!-- ROW2: Tools-->
<Row cols={{ xs: 1, md: 2, lg: 5}} class="mb-3">
  <Col lg="2" class="mb-2 mb-lg-0">
    <ButtonGroup class="w-100">
      <Button outline color="primary" on:click={() => (isSortingOpen = true)} disabled={showCompare}>
        <Icon name="sort-up" /> Sorting
      </Button>
      <Button
        outline
        color="primary"
        on:click={() => (isMetricsSelectionOpen = true)}
      >
        <Icon name="graph-up" /> Metrics
      </Button>
    </ButtonGroup>
  </Col>
  <Col lg="4" class="mb-1 mb-lg-0">
    <Filters
      showFilter={!showCompare}
      {filterPresets}
      matchedJobs={showCompare? matchedCompareJobs: matchedListJobs}
      bind:this={filterComponent}
      on:update-filters={({ detail }) => {
        selectedCluster = detail.filters[0]?.cluster
          ? detail.filters[0].cluster.eq
          : null;
        filterBuffer = [...detail.filters]
        if (showCompare) {
          jobCompare.queryJobs(detail.filters);
        } else {
          jobList.queryJobs(detail.filters);
        }
      }}
    />
  </Col>
  <Col lg="2" class="mb-2 mb-lg-0">
    <TextFilter
      {presetProject}
      bind:authlevel
      bind:roles
      on:set-filter={({ detail }) => filterComponent.updateFilters(detail)}
    />
  </Col>
  <Col lg="2" class="mb-1 mb-lg-0">
    <Refresher on:refresh={() => {
      if (showCompare) {
        jobCompare.refreshJobs()
        jobCompare.refreshAllMetrics()
      } else {
        jobList.refreshJobs()
        jobList.refreshAllMetrics()
      }
    }} />
  </Col>
  <Col lg="2" class="mb-2 mb-lg-0">
    <ButtonGroup class="w-100">
      <Button color="primary" disabled={matchedListJobs >= 500 && !(selectedJobs.length != 0)} on:click={() => {
        if (selectedJobs.length != 0) filterComponent.updateFilters({dbId: selectedJobs})
        showCompare = !showCompare
      }} >
        {showCompare ? 'Return to List' : 
        'Compare Jobs' + (selectedJobs.length != 0 ? ` (${selectedJobs.length} selected)` : matchedListJobs >= 500 ? ` (Too Many)` : ``)}
      </Button>
      {#if !showCompare && selectedJobs.length != 0}
        <Button color="warning" on:click={() => {
          selectedJobs = [] // Only empty array, filters handled by reactive reset
        }}>
        Clear
        </Button>
      {/if}
    </ButtonGroup>
  </Col>
</Row>

<!-- ROW3: Job List / Job Compare-->
<Row>
  <Col>
    {#if !showCompare}
      <JobList
        bind:this={jobList}
        bind:metrics
        bind:sorting
        bind:matchedListJobs
        bind:showFootprint
        bind:selectedJobs
        {filterBuffer}
      />
    {:else}
      <JobCompare
        bind:this={jobCompare}
        bind:metrics
        bind:matchedCompareJobs
        {filterBuffer}
      />
    {/if}
  </Col>
</Row>

<Sorting bind:sorting bind:isOpen={isSortingOpen}/>

<MetricSelection
  bind:cluster={selectedCluster}
  configName="plot_list_selectedMetrics"
  bind:metrics
  bind:isOpen={isMetricsSelectionOpen}
  bind:showFootprint
  footprintSelect
/>
