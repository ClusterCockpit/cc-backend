<!--
    @component Main filter component; handles filter object on sub-component changes before dispatching it

    Properties:
    - `menuText String?`: Optional text to show in the dropdown menu [Default: null]
    - `filterPresets Object?`: Optional predefined filter values [Default: {}]
    - `disableClusterSelection Bool?`: Is the selection disabled [Default: false]
    - `startTimeQuickSelect Bool?`: Render startTime quick selections [Default: false]
    - `matchedJobs Number?`: Number of jobs matching the filter [Default: -2]

    Events:
    - `update-filters, {filters: [Object]?}`: The detail's 'filters' prop are new filter items to be applied
    
    Functions:
    - `void updateFilters (additionalFilters: Object?)`: Handles new filters from nested components, triggers upstream update event
 -->

<script>
  import { createEventDispatcher } from "svelte";
  import {
    DropdownItem,
    DropdownMenu,
    DropdownToggle,
    Button,
    ButtonGroup,
    ButtonDropdown,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import Tag from "./helper/Tag.svelte";
  import Info from "./filters/InfoBox.svelte";
  import Cluster from "./filters/Cluster.svelte";
  import JobStates, { allJobStates } from "./filters/JobStates.svelte";
  import StartTime from "./filters/StartTime.svelte";
  import Tags from "./filters/Tags.svelte";
  import Duration from "./filters/Duration.svelte";
  import Energy from "./filters/Energy.svelte";
  import Resources from "./filters/Resources.svelte";
  import Statistics from "./filters/Stats.svelte";

  const dispatch = createEventDispatcher();

  export let menuText = null;
  export let filterPresets = {};
  export let disableClusterSelection = false;
  export let startTimeQuickSelect = false;
  export let matchedJobs = -2;

  const startTimeSelectOptions = [
    { range: "", rangeLabel: "No Selection"},
    { range: "last6h", rangeLabel: "Last 6hrs"},
    { range: "last24h", rangeLabel: "Last 24hrs"},
    { range: "last7d", rangeLabel: "Last 7 days"},
    { range: "last30d", rangeLabel: "Last 30 days"}
  ];

  const nodeMatchLabels = {
    eq: "",
    contains: " Contains",
  }

  let filters = {
    projectMatch: filterPresets.projectMatch || "contains",
    userMatch: filterPresets.userMatch || "contains",
    jobIdMatch: filterPresets.jobIdMatch || "eq",
    nodeMatch: filterPresets.nodeMatch || "eq",

    cluster: filterPresets.cluster || null,
    partition: filterPresets.partition || null,
    states:
      filterPresets.states || filterPresets.state
        ? [filterPresets.state].flat()
        : allJobStates,
    startTime: filterPresets.startTime || { from: null, to: null, range: ""},
    tags: filterPresets.tags || [],
    duration: filterPresets.duration || {
      lessThan: null,
      moreThan: null,
      from: null,
      to: null,
    },
    dbId: filterPresets.dbId || [],
    jobId: filterPresets.jobId || "",
    arrayJobId: filterPresets.arrayJobId || null,
    user: filterPresets.user || "",
    project: filterPresets.project || "",
    jobName: filterPresets.jobName || "",

    node: filterPresets.node || null,
    energy: filterPresets.energy || { from: null, to: null },
    numNodes: filterPresets.numNodes || { from: null, to: null },
    numHWThreads: filterPresets.numHWThreads || { from: null, to: null },
    numAccelerators: filterPresets.numAccelerators || { from: null, to: null },

    stats: filterPresets.stats || [],
  };

  let isClusterOpen = false,
    isJobStatesOpen = false,
    isStartTimeOpen = false,
    isTagsOpen = false,
    isDurationOpen = false,
    isEnergyOpen = false,
    isResourcesOpen = false,
    isStatsOpen = false,
    isNodesModified = false,
    isHwthreadsModified = false,
    isAccsModified = false;

  // Can be called from the outside to trigger a 'update' event from this component.
  export function updateFilters(additionalFilters = null) {
    if (additionalFilters != null)
      for (let key in additionalFilters) filters[key] = additionalFilters[key];

    let items = [];
    if (filters.cluster) items.push({ cluster: { eq: filters.cluster } });
    if (filters.node) items.push({ node: { [filters.nodeMatch]: filters.node } });
    if (filters.partition) items.push({ partition: { eq: filters.partition } });
    if (filters.states.length != allJobStates.length)
      items.push({ state: filters.states });
    if (filters.startTime.from || filters.startTime.to)
      items.push({
        startTime: { from: filters.startTime.from, to: filters.startTime.to },
      });
    if (filters.startTime.range)
      items.push({
        startTime: { range: filters.startTime.range },
      });
    if (filters.tags.length != 0) items.push({ tags: filters.tags });
    if (filters.duration.from || filters.duration.to)
      items.push({
        duration: { from: filters.duration.from, to: filters.duration.to },
      });
    if (filters.duration.lessThan)
      items.push({ duration: { from: 0, to: filters.duration.lessThan } });
    if (filters.duration.moreThan)
      items.push({ duration: { from: filters.duration.moreThan, to: 604800 } }); // 7 days to include special jobs with long runtimes
    if (filters.energy.from || filters.energy.to)
      items.push({
        energy: { from: filters.energy.from, to: filters.energy.to },
      });
    if (filters.dbId.length != 0)
      items.push({ dbId: filters.dbId });
    if (filters.jobId)
      items.push({ jobId: { [filters.jobIdMatch]: filters.jobId } });
    if (filters.arrayJobId != null)
      items.push({ arrayJobId: filters.arrayJobId });
    if (filters.numNodes.from != null || filters.numNodes.to != null) {
      items.push({
        numNodes: { from: filters.numNodes.from, to: filters.numNodes.to },
      });
      isNodesModified = true;
    }
    if (filters.numHWThreads.from != null || filters.numHWThreads.to != null) {
      items.push({
        numHWThreads: {
          from: filters.numHWThreads.from,
          to: filters.numHWThreads.to,
        },
      });
      isHwthreadsModified = true;
    }
    if (filters.numAccelerators.from != null || filters.numAccelerators.to != null) {
      items.push({
        numAccelerators: {
          from: filters.numAccelerators.from,
          to: filters.numAccelerators.to,
        },
      });
      isAccsModified = true;
    }
    if (filters.user)
      items.push({ user: { [filters.userMatch]: filters.user } });
    if (filters.project)
      items.push({ project: { [filters.projectMatch]: filters.project } });
    if (filters.jobName) items.push({ jobName: { contains: filters.jobName } });
    if (filters.stats.length != 0)
      items.push({ metricStats: filters.stats.map((st) => { return { metricName: st.field, range: { from: st.from, to: st.to }} }) });

    dispatch("update-filters", { filters: items });
    changeURL();
    return items;
  }

  function changeURL() {
    const dateToUnixEpoch = (rfc3339) => Math.floor(Date.parse(rfc3339) / 1000);
    let opts = [];
    if (filters.cluster) opts.push(`cluster=${filters.cluster}`);
    if (filters.node) opts.push(`node=${filters.node}`);
    if (filters.node && filters.nodeMatch != "eq") // "eq" is default-case
      opts.push(`nodeMatch=${filters.nodeMatch}`);
    if (filters.partition) opts.push(`partition=${filters.partition}`);
    if (filters.states.length != allJobStates.length)
      for (let state of filters.states) opts.push(`state=${state}`);
    if (filters.startTime.from && filters.startTime.to)
      opts.push(
        `startTime=${dateToUnixEpoch(filters.startTime.from)}-${dateToUnixEpoch(filters.startTime.to)}`,
      );
    if (filters.startTime.range) {
        opts.push(`startTime=${filters.startTime.range}`)
    }
    if (filters.dbId.length != 0) {
      for (let dbi of filters.dbId) {
        opts.push(`dbId=${dbi}`);
      }
    }
    if (filters.jobId.length != 0)
      if (filters.jobIdMatch != "in") {
        opts.push(`jobId=${filters.jobId}`);
      } else {
        for (let singleJobId of filters.jobId)
          opts.push(`jobId=${singleJobId}`);
      }
    if (filters.jobIdMatch != "eq")
      opts.push(`jobIdMatch=${filters.jobIdMatch}`); // "eq" is default-case
    for (let tag of filters.tags) opts.push(`tag=${tag}`);
    if (filters.duration.from && filters.duration.to)
      opts.push(`duration=${filters.duration.from}-${filters.duration.to}`);
    if (filters.duration.lessThan)
      opts.push(`duration=0-${filters.duration.lessThan}`);
    if (filters.duration.moreThan)
      opts.push(`duration=${filters.duration.moreThan}-604800`);
    if (filters.energy.from && filters.energy.to)
      opts.push(`energy=${filters.energy.from}-${filters.energy.to}`);
    if (filters.numNodes.from && filters.numNodes.to)
      opts.push(`numNodes=${filters.numNodes.from}-${filters.numNodes.to}`);
    if (filters.numHWThreads.from && filters.numHWThreads.to)
      opts.push(`numHWThreads=${filters.numHWThreads.from}-${filters.numHWThreads.to}`);
    if (filters.numAccelerators.from && filters.numAccelerators.to)
      opts.push(`numAccelerators=${filters.numAccelerators.from}-${filters.numAccelerators.to}`);
    if (filters.user.length != 0)
      if (filters.userMatch != "in") {
        opts.push(`user=${filters.user}`);
      } else {
        for (let singleUser of filters.user) opts.push(`user=${singleUser}`);
      }
    if (filters.userMatch != "contains") // "contains" is default-case
      opts.push(`userMatch=${filters.userMatch}`);
    if (filters.project) opts.push(`project=${filters.project}`);
    if (filters.project && filters.projectMatch != "contains") // "contains" is default-case
     opts.push(`projectMatch=${filters.projectMatch}`);
    if (filters.jobName) opts.push(`jobName=${filters.jobName}`);
    if (filters.arrayJobId) opts.push(`arrayJobId=${filters.arrayJobId}`);
    if (filters.stats.length != 0)
      for (let stat of filters.stats) {
          opts.push(`stat=${stat.field}-${stat.from}-${stat.to}`);
      }
    if (opts.length == 0 && window.location.search.length <= 1) return;

    let newurl = `${window.location.pathname}?${opts.join("&")}`;
    window.history.replaceState(null, "", newurl);
  }
</script>

<!-- Dropdown-Button -->
<ButtonGroup>
  <ButtonDropdown class="cc-dropdown-on-hover mb-1" style="{(matchedJobs >= -1) ? '' : 'margin-right: 0.5rem;'}">
    <DropdownToggle outline caret color="success">
      <Icon name="sliders" />
      Filters
    </DropdownToggle>
    <DropdownMenu>
      <DropdownItem header>Manage Filters</DropdownItem>
      {#if menuText}
        <DropdownItem disabled>{menuText}</DropdownItem>
        <DropdownItem divider />
      {/if}
      <DropdownItem on:click={() => (isClusterOpen = true)}>
        <Icon name="cpu" /> Cluster/Partition
      </DropdownItem>
      <DropdownItem on:click={() => (isJobStatesOpen = true)}>
        <Icon name="gear-fill" /> Job States
      </DropdownItem>
      <DropdownItem on:click={() => (isStartTimeOpen = true)}>
        <Icon name="calendar-range" /> Start Time
      </DropdownItem>
      <DropdownItem on:click={() => (isDurationOpen = true)}>
        <Icon name="stopwatch" /> Duration
      </DropdownItem>
      <DropdownItem on:click={() => (isTagsOpen = true)}>
        <Icon name="tags" /> Tags
      </DropdownItem>
      <DropdownItem on:click={() => (isResourcesOpen = true)}>
        <Icon name="hdd-stack" /> Resources
      </DropdownItem>
      <DropdownItem on:click={() => (isEnergyOpen = true)}>
        <Icon name="lightning-charge-fill" /> Energy
      </DropdownItem>
      <DropdownItem on:click={() => (isStatsOpen = true)}>
        <Icon name="bar-chart" on:click={() => (isStatsOpen = true)} /> Statistics
      </DropdownItem>
      {#if startTimeQuickSelect}
        <DropdownItem divider />
        <DropdownItem disabled>Start Time Quick Selection</DropdownItem>
        {#each startTimeSelectOptions.filter((stso) => stso.range !== "") as { rangeLabel, range }}
          <DropdownItem
            on:click={() => {
              filters.startTime.from = null
              filters.startTime.to = null
              filters.startTime.range = range;
              updateFilters();
            }}
          >
            <Icon name="calendar-range" />
            {rangeLabel}
          </DropdownItem>
        {/each}
      {/if}
    </DropdownMenu>
  </ButtonDropdown>
  {#if matchedJobs >= -1}
    <Button class="mb-1" style="margin-right: 0.5rem;" disabled outline>
      {matchedJobs == -1 ? 'Loading ...' : `${matchedJobs} jobs`}
    </Button>
  {/if}
</ButtonGroup>

<!-- SELECTED FILTER PILLS -->
{#if filters.cluster}
  <Info icon="cpu" on:click={() => (isClusterOpen = true)}>
    {filters.cluster}
    {#if filters.partition}
      ({filters.partition})
    {/if}
  </Info>
{/if}

{#if filters.states.length != allJobStates.length}
  <Info icon="gear-fill" on:click={() => (isJobStatesOpen = true)}>
    {filters.states.join(", ")}
  </Info>
{/if}

{#if filters.startTime.from || filters.startTime.to}
  <Info icon="calendar-range" on:click={() => (isStartTimeOpen = true)}>
    {new Date(filters.startTime.from).toLocaleString()} - {new Date(
      filters.startTime.to,
    ).toLocaleString()}
  </Info>
{/if}

{#if filters.startTime.range}
  <Info icon="calendar-range" on:click={() => (isStartTimeOpen = true)}>
    {startTimeSelectOptions.find((stso) => stso.range === filters.startTime.range).rangeLabel }
  </Info>
{/if}

{#if filters.duration.from || filters.duration.to}
  <Info icon="stopwatch" on:click={() => (isDurationOpen = true)}>
    {Math.floor(filters.duration.from / 3600)}h:{Math.floor(
      (filters.duration.from % 3600) / 60,
    )}m -
    {Math.floor(filters.duration.to / 3600)}h:{Math.floor(
      (filters.duration.to % 3600) / 60,
    )}m
  </Info>
{/if}

{#if filters.duration.lessThan}
  <Info icon="stopwatch" on:click={() => (isDurationOpen = true)}>
    Duration less than {Math.floor(
      filters.duration.lessThan / 3600,
    )}h:{Math.floor((filters.duration.lessThan % 3600) / 60)}m
  </Info>
{/if}

{#if filters.duration.moreThan}
  <Info icon="stopwatch" on:click={() => (isDurationOpen = true)}>
    Duration more than {Math.floor(
      filters.duration.moreThan / 3600,
    )}h:{Math.floor((filters.duration.moreThan % 3600) / 60)}m
  </Info>
{/if}

{#if filters.tags.length != 0}
  <Info icon="tags" on:click={() => (isTagsOpen = true)}>
    {#each filters.tags as tagId}
      {#key tagId}
        <Tag id={tagId} clickable={false} />
      {/key}
    {/each}
  </Info>
{/if}

{#if filters.numNodes.from != null || filters.numNodes.to != null || filters.numHWThreads.from != null || filters.numHWThreads.to != null || filters.numAccelerators.from != null || filters.numAccelerators.to != null}
  <Info icon="hdd-stack" on:click={() => (isResourcesOpen = true)}>
    {#if isNodesModified}
      Nodes: {filters.numNodes.from} - {filters.numNodes.to}
    {/if}
    {#if isNodesModified && isHwthreadsModified},
    {/if}
    {#if isHwthreadsModified}
      HWThreads: {filters.numHWThreads.from} - {filters.numHWThreads.to}
    {/if}
    {#if (isNodesModified || isHwthreadsModified) && isAccsModified},
    {/if}
    {#if isAccsModified}
      Accelerators: {filters.numAccelerators.from} - {filters.numAccelerators.to}
    {/if}
  </Info>
{/if}

{#if filters.node != null}
  <Info icon="hdd-stack" on:click={() => (isResourcesOpen = true)}>
    Node{nodeMatchLabels[filters.nodeMatch]}: {filters.node}
  </Info>
{/if}

{#if filters.energy.from || filters.energy.to}
  <Info icon="lightning-charge-fill" on:click={() => (isEnergyOpen = true)}>
    Total Energy: {filters.energy.from} - {filters.energy.to}
  </Info>
{/if}

{#if filters.stats.length > 0}
  <Info icon="bar-chart" on:click={() => (isStatsOpen = true)}>
    {filters.stats
      .map((stat) => `${stat.field}: ${stat.from} - ${stat.to}`)
      .join(", ")}
  </Info>
{/if}

<Cluster
  {disableClusterSelection}
  bind:isOpen={isClusterOpen}
  bind:cluster={filters.cluster}
  bind:partition={filters.partition}
  on:set-filter={() => updateFilters()}
/>

<JobStates
  bind:isOpen={isJobStatesOpen}
  bind:states={filters.states}
  on:set-filter={() => updateFilters()}
/>

<StartTime
  bind:isOpen={isStartTimeOpen}
  bind:from={filters.startTime.from}
  bind:to={filters.startTime.to}
  bind:range={filters.startTime.range}
  {startTimeSelectOptions}
  on:set-filter={() => updateFilters()}
/>

<Duration
  bind:isOpen={isDurationOpen}
  bind:lessThan={filters.duration.lessThan}
  bind:moreThan={filters.duration.moreThan}
  bind:from={filters.duration.from}
  bind:to={filters.duration.to}
  on:set-filter={() => updateFilters()}
/>

<Tags
  bind:isOpen={isTagsOpen}
  bind:tags={filters.tags}
  on:set-filter={() => updateFilters()}
/>

<Resources
  cluster={filters.cluster}
  bind:isOpen={isResourcesOpen}
  bind:numNodes={filters.numNodes}
  bind:numHWThreads={filters.numHWThreads}
  bind:numAccelerators={filters.numAccelerators}
  bind:namedNode={filters.node}
  bind:nodeMatch={filters.nodeMatch}
  bind:isNodesModified
  bind:isHwthreadsModified
  bind:isAccsModified
  on:set-filter={() => updateFilters()}
/>

<Statistics
  bind:isOpen={isStatsOpen}
  bind:stats={filters.stats}
  on:set-filter={() => updateFilters()}
/>

<Energy
  bind:isOpen={isEnergyOpen}
  bind:energy={filters.energy}
  on:set-filter={() => updateFilters()}
/>

<style>
  :global(.cc-dropdown-on-hover:hover .dropdown-menu) {
    display: block;
    margin-top: 0px;
    padding-top: 0px;
    transform: none !important;
  }
</style>
