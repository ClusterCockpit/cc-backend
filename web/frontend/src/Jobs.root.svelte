<!--
  @component Main job list component

  Properties:
  - `filterPresets Object`: Optional predefined filter values
  - `authlevel Number`: The current users authentication level
  - `roles [Number]`: Enum containing available roles
-->

 <script>
  import { untrack, onMount, getContext } from "svelte";
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

  /* Svelte 5 Props */
  let { 
    filterPresets,
    authlevel,
    roles
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const ccconfig = getContext("cc-config");
  const matchedJobCompareLimit = 500;

  /* State Init */
  let filterComponent = $state(); // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
  let selectedJobs = $state([]);
  let filterBuffer = $state([]);
  let jobList = $state(null);
  let jobCompare = $state(null);
  let matchedListJobs = $state(0);
  let matchedCompareJobs = $state(0);
  let isSortingOpen = $state(false);
  let showCompare = $state(false);
  let isMetricsSelectionOpen = $state(false);
  let sorting = $state({ field: "startTime", type: "col", order: "DESC" });

  /* Derived */
  let presetProject = $derived(filterPresets?.project ? filterPresets.project : "");
  let selectedCluster = $derived(filterPresets?.cluster ? filterPresets.cluster : null);
  let selectedSubCluster = $derived(filterPresets?.partition ? filterPresets.partition : null);
  let metrics = $derived.by(() => {
    if (selectedCluster) {
      if (selectedSubCluster) {
        return ccconfig[`metricConfig_jobListMetrics:${selectedCluster}:${selectedSubCluster}`] ||
          ccconfig[`metricConfig_jobListMetrics:${selectedCluster}`] ||
          ccconfig.metricConfig_jobListMetrics
      }
      return ccconfig[`metricConfig_jobListMetrics:${selectedCluster}`] ||
        ccconfig.metricConfig_jobListMetrics
    }
    return ccconfig.metricConfig_jobListMetrics
  });

  let showFootprint = $derived(selectedCluster
    ? !!ccconfig[`jobList_showFootprint:${selectedCluster}`]
    : !!ccconfig.jobList_showFootprint
  );

  /* Functions */
  function resetJobSelection() {
    if (filterComponent && selectedJobs.length === 0) {
      filterComponent.updateFilters({ dbId: [] });
    };
  };

  /* Reactive Effects */
  $effect(() => {
    // Reactive : Trigger Effect
    selectedJobs.length
    untrack(() => {
      // Unreactive : Apply Reset w/o starting infinite loop
      resetJobSelection()
    });
	});

  /* On Mount */
  // The filterPresets are handled by the Filters component,
  // so we need to wait for it to be ready before we can start a query.
  // This is also why JobList component starts out with a paused query.
  onMount(() => filterComponent.updateFilters());
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
      <Button outline color="primary" onclick={() => (isSortingOpen = true)} disabled={showCompare}>
        <Icon name="sort-up" /> Sorting
      </Button>
      <Button
        outline
        color="primary"
        onclick={() => (isMetricsSelectionOpen = true)}
      >
        <Icon name="graph-up" /> Metrics
      </Button>
    </ButtonGroup>
  </Col>
  <Col lg="5" class="mb-1 mb-lg-0">
    <Filters
      bind:this={filterComponent}
      {filterPresets}
      showFilter={!showCompare}
      matchedJobs={showCompare? matchedCompareJobs: matchedListJobs}
      applyFilters={(detail) => {
        selectedCluster = detail.filters[0]?.cluster
          ? detail.filters[0].cluster.eq
          : null;
        selectedSubCluster = detail.filters[1]?.partition
          ? detail.filters[1].partition.eq
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
    {#if !showCompare}
      <TextFilter
        {presetProject}
        {authlevel}
        {roles}
        {filterBuffer}
        setFilter={(filter) => filterComponent.updateFilters(filter)}
      />
    {/if}
  </Col>
  <Col lg="3" class="mb-1 mb-lg-0 d-inline-flex align-items-start justify-content-end ">
    {#if !showCompare}
      <Refresher presetClass="w-auto" onRefresh={() => {
          jobList.refreshJobs()
          jobList.refreshAllMetrics()
      }} />
    {/if}
    <div class="mx-1"></div>
    <ButtonGroup class="w-50">
      <Button color="primary" disabled={(matchedListJobs >= matchedJobCompareLimit && !(selectedJobs.length != 0)) || $initq.fetching} onclick={() => {
        if (selectedJobs.length != 0) filterComponent.updateFilters({dbId: selectedJobs})
        showCompare = !showCompare
      }} >
        {showCompare ? 'Return to List' : 
           matchedListJobs >= matchedJobCompareLimit && selectedJobs.length == 0
            ? 'Compare Disabled'
            : 'Compare' + (selectedJobs.length != 0 ? ` ${selectedJobs.length} ` : ' ') + 'Jobs'
        }
      </Button>
      {#if !showCompare && selectedJobs.length != 0}
        <Button class="w-auto" color="warning" onclick={() => {
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
        bind:matchedListJobs
        bind:selectedJobs
        {metrics}
        {sorting}
        {showFootprint}
        {filterBuffer}
      />
    {:else}
      <JobCompare
        bind:this={jobCompare}
        bind:matchedCompareJobs
        {metrics}
        {filterBuffer}
      />
    {/if}
  </Col>
</Row>

<Sorting
  bind:isOpen={isSortingOpen}
  presetSorting={sorting}
  applySorting={(newSort) =>
    sorting = {...newSort}
  }
/>

<MetricSelection
    bind:isOpen={isMetricsSelectionOpen}
    bind:showFootprint
    presetMetrics={metrics}
    cluster={selectedCluster}
    subCluster={selectedSubCluster}
    configName="metricConfig_jobListMetrics"
    footprintSelect
    applyMetrics={(newMetrics) => 
      metrics = [...newMetrics]
    }
/>
