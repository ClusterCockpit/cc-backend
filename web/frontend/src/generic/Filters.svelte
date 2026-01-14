<!--
  @component Main filter component; handles filter object on sub-component changes before dispatching it

  Properties:
  - `menuText String?`: Optional text to show in the dropdown menu [Default: null]
  - `filterPresets Object?`: Optional predefined filter values [Default: {}]
  - `disableClusterSelection Bool?`: Is the selection disabled [Default: false]
  - `startTimeQuickSelect Bool?`: Render startTime quick selections [Default: false]
  - `matchedJobs Number?`: Number of jobs matching the filter [Default: -2]
  - `showFilter Func`: If the filter component should be rendered in addition to total count info [Default: true]
  - `applyFilters Func`: The callback function to apply current filter selection
  
  Functions:
  - `void updateFilters (additionalFilters: Object, force: Bool)`:
    Handles new filters from nested components, triggers upstream update event. 
    'additionalFilters' usually added to existing selection, but can be forced to overwrite instead.
-->

<script>
  import {
    DropdownItem,
    DropdownMenu,
    DropdownToggle,
    Button,
    ButtonGroup,
    ButtonDropdown,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import Info from "./filters/InfoBox.svelte";
  import Cluster from "./filters/Cluster.svelte";
  import JobStates, { allJobStates, mapSharedStates } from "./filters/JobStates.svelte";
  import StartTime, { startTimeSelectOptions } from "./filters/StartTime.svelte";
  import Duration from "./filters/Duration.svelte";
  import Tags from "./filters/Tags.svelte";
  import Tag from "./helper/Tag.svelte";
  import Resources from "./filters/Resources.svelte";
  import Energy from "./filters/Energy.svelte";
  import Statistics from "./filters/Stats.svelte";

  /* Svelte 5 Props */
  let {  
    menuText = null,
    filterPresets = {},
    disableClusterSelection = false,
    startTimeQuickSelect = false,
    matchedJobs = -2,
    showFilter = true,
    applyFilters
  } = $props();

  /* Const Init */
  const nodeMatchLabels = {
    eq: "",
    contains: " Contains",
  }
  const filterReset = {
    // Direct Filters
    dbId: [],
    jobId: "",
    jobIdMatch: "eq",
    arrayJobId: null,
    jobName: "",
    // View Filters
    project: "",
    projectMatch: "contains",
    user: "",
    userMatch: "contains",
    // Filter Modals
    cluster: null,
    partition: null,
    states: allJobStates,
    shared: "",
    schedule: "",
    startTime: { from: null, to: null, range: ""},
    duration: {
      lessThan: null,
      moreThan: null,
      from: null,
      to: null,
    },
    tags: [],
    numNodes: { from: null, to: null },
    numHWThreads: { from: null, to: null },
    numAccelerators: { from: null, to: null },
    node: null,
    nodeMatch: "eq",
    energy: { from: null, to: null },
    stats: [],
  };

  /* State Init */
  let filters = $state({
    dbId: filterPresets.dbId || [],
    jobId: filterPresets.jobId || "",
    jobIdMatch: filterPresets.jobIdMatch || "eq",
    arrayJobId: filterPresets.arrayJobId || null,
    jobName: filterPresets.jobName || "",
    project: filterPresets.project || "",
    projectMatch: filterPresets.projectMatch || "contains",
    user: filterPresets.user || "",
    userMatch: filterPresets.userMatch || "contains",
    cluster: filterPresets.cluster || null,
    partition: filterPresets.partition || null,
    states:
      filterPresets.states || filterPresets.state
        ? [filterPresets.state].flat()
        : allJobStates,
    shared: filterPresets.shared || "",
    schedule: filterPresets.schedule || "",
    startTime: filterPresets.startTime || { from: null, to: null, range: ""},
    duration: filterPresets.duration || {
      lessThan: null,
      moreThan: null,
      from: null,
      to: null,
    },
    tags: filterPresets.tags || [],
    numNodes: filterPresets.numNodes || { from: null, to: null },
    numHWThreads: filterPresets.numHWThreads || { from: null, to: null },
    numAccelerators: filterPresets.numAccelerators || { from: null, to: null },
    node: filterPresets.node || null,
    nodeMatch: filterPresets.nodeMatch || "eq",
    energy: filterPresets.energy || { from: null, to: null },
    stats: filterPresets.stats || [],
  });

  /* Opened States */
  let isClusterOpen = $state(false)
  let isJobStatesOpen = $state(false)
  let isStartTimeOpen = $state(false)
  let isDurationOpen = $state(false)
  let isTagsOpen = $state(false)
  let isResourcesOpen = $state(false)
  let isEnergyOpen = $state(false)
  let isStatsOpen = $state(false)

  /* Functions */
  // Can be called from the outside to trigger a 'update' event from this component.
  // 'force' option empties existing filters and then applies only 'additionalFilters'
  export function updateFilters(additionalFilters = null, force = false) {
    // Empty Current Filter For Force
    if (additionalFilters != null && force) {
      filters = {...filterReset}
    }
    // Add Additional Filters
    if (additionalFilters != null) {
      for (let key in additionalFilters) filters[key] = additionalFilters[key];
    }
    // Construct New Filter
    let items = [];
    if (filters.dbId.length != 0)
      items.push({ dbId: filters.dbId });
    if (filters.cluster) items.push({ cluster: { eq: filters.cluster } });
    if (filters.partition) items.push({ partition: { eq: filters.partition } });
    if (filters.states.length != allJobStates?.length)
      items.push({ state: filters.states });
    if (filters.shared) items.push({ shared: filters.shared });
    if (filters.project)
      items.push({ project: { [filters.projectMatch]: filters.project } });
    if (filters.user)
      items.push({ user: { [filters.userMatch]: filters.user } });
    if (filters.numNodes.from != null || filters.numNodes.to != null) {
      items.push({
        numNodes: { from: filters.numNodes.from, to: filters.numNodes.to },
      });
    }
    if (filters.numAccelerators.from != null || filters.numAccelerators.to != null) {
      items.push({
        numAccelerators: {
          from: filters.numAccelerators.from,
          to: filters.numAccelerators.to,
        },
      });
    }
    if (filters.numHWThreads.from != null || filters.numHWThreads.to != null) {
      items.push({
        numHWThreads: {
          from: filters.numHWThreads.from,
          to: filters.numHWThreads.to,
        },
      });
    }
    if (filters.arrayJobId != null)
      items.push({ arrayJobId: filters.arrayJobId });
    if (filters.tags.length != 0) items.push({ tags: filters.tags });
    if (filters.startTime.from || filters.startTime.to)
      items.push({
        startTime: { from: filters.startTime.from, to: filters.startTime.to },
      });
    if (filters.startTime.range)
      items.push({
        startTime: { range: filters.startTime.range },
      });
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
    if (filters.jobId)
      items.push({ jobId: { [filters.jobIdMatch]: filters.jobId } });
    if (filters.stats.length != 0)
      items.push({ metricStats: filters.stats.map((st) => { return { metricName: st.field, range: { from: st.from, to: st.to }} }) });
    if (filters.node) items.push({ node: { [filters.nodeMatch]: filters.node } });
    if (filters.jobName) items.push({ jobName: { contains: filters.jobName } });
    if (filters.schedule) items.push({ schedule: filters.schedule });
    applyFilters({ filters: items });
    changeURL();
    return items;
  }

  function changeURL() {
    const dateToUnixEpoch = (rfc3339) => Math.floor(Date.parse(rfc3339) / 1000);
    let opts = [];

    // Direct Filters
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
    if (filters.arrayJobId) opts.push(`arrayJobId=${filters.arrayJobId}`);
    if (filters.jobName) opts.push(`jobName=${filters.jobName}`);
    // View Filters
    if (filters.project) opts.push(`project=${filters.project}`);
    if (filters.project && filters.projectMatch != "contains") // "contains" is default-case
     opts.push(`projectMatch=${filters.projectMatch}`);
    if (filters.user.length != 0)
      if (filters.userMatch != "in") {
        opts.push(`user=${filters.user}`);
      } else {
        for (let singleUser of filters.user) opts.push(`user=${singleUser}`);
      }
    if (filters.userMatch != "contains") // "contains" is default-case
      opts.push(`userMatch=${filters.userMatch}`);
    // Filter Modals
    if (filters.cluster) opts.push(`cluster=${filters.cluster}`);
    if (filters.partition) opts.push(`partition=${filters.partition}`);
    if (filters.states.length != allJobStates?.length)
      for (let state of filters.states) opts.push(`state=${state}`);
    if (filters.shared) opts.push(`shared=${filters.shared}`);
    if (filters.schedule) opts.push(`schedule=${filters.schedule}`);
    if (filters.startTime.from && filters.startTime.to)
      opts.push(
        `startTime=${dateToUnixEpoch(filters.startTime.from)}-${dateToUnixEpoch(filters.startTime.to)}`,
      );
    if (filters.startTime.range) {
        opts.push(`startTime=${filters.startTime.range}`)
    }
    if (filters.duration.from && filters.duration.to)
      opts.push(`duration=${filters.duration.from}-${filters.duration.to}`);
    if (filters.duration.lessThan)
      opts.push(`duration=0-${filters.duration.lessThan}`);
    if (filters.duration.moreThan)
      opts.push(`duration=${filters.duration.moreThan}-604800`);
    if (filters.tags.length != 0)
      for (let tag of filters.tags) opts.push(`tag=${tag}`);
    if (filters.numNodes.from && filters.numNodes.to)
      opts.push(`numNodes=${filters.numNodes.from}-${filters.numNodes.to}`);
    if (filters.numHWThreads.from && filters.numHWThreads.to)
      opts.push(`numHWThreads=${filters.numHWThreads.from}-${filters.numHWThreads.to}`);
    if (filters.numAccelerators.from && filters.numAccelerators.to)
      opts.push(`numAccelerators=${filters.numAccelerators.from}-${filters.numAccelerators.to}`);
    if (filters.node) opts.push(`node=${filters.node}`);
    if (filters.node && filters.nodeMatch != "eq") // "eq" is default-case
      opts.push(`nodeMatch=${filters.nodeMatch}`);
    if (filters.energy.from && filters.energy.to)
      opts.push(`energy=${filters.energy.from}-${filters.energy.to}`);
    if (filters.stats.length != 0)
      for (let stat of filters.stats) {
          opts.push(`stat=${stat.field}-${stat.from}-${stat.to}`);
      }
    // Build && Return
    if (opts.length == 0 && window.location.search.length <= 1) return;
    let newurl = `${window.location.pathname}?${opts.join("&")}`;
    window.history.replaceState(null, "", newurl);
  }
</script>

<!-- Dropdown-Button -->
<ButtonGroup>
  {#if showFilter}
    <ButtonDropdown class="cc-dropdown-on-hover mb-1" style={(matchedJobs >= -1) ? '' : 'margin-right: 0.5rem;'}>
      <DropdownToggle outline caret color="success">
        <Icon name="sliders" />
        Filters
      </DropdownToggle>
      <DropdownMenu>
        {#if menuText}
          <DropdownItem header>Note</DropdownItem>
          <DropdownItem disabled>{menuText}</DropdownItem>
          <DropdownItem divider />
        {/if}
        <DropdownItem header>Manage Filters</DropdownItem>
        <DropdownItem onclick={() => (isClusterOpen = true)}>
          <Icon name="cpu" /> Cluster/Partition
        </DropdownItem>
        <DropdownItem onclick={() => (isJobStatesOpen = true)}>
          <Icon name="gear-fill" /> Job States
        </DropdownItem>
        <DropdownItem onclick={() => (isStartTimeOpen = true)}>
          <Icon name="calendar-range" /> Start Time
        </DropdownItem>
        <DropdownItem onclick={() => (isDurationOpen = true)}>
          <Icon name="stopwatch" /> Duration
        </DropdownItem>
        <DropdownItem onclick={() => (isTagsOpen = true)}>
          <Icon name="tags" /> Tags
        </DropdownItem>
        <DropdownItem onclick={() => (isResourcesOpen = true)}>
          <Icon name="hdd-stack" /> Resources
        </DropdownItem>
        <DropdownItem onclick={() => (isEnergyOpen = true)}>
          <Icon name="lightning-charge-fill" /> Energy
        </DropdownItem>
        <DropdownItem onclick={() => (isStatsOpen = true)}>
          <Icon name="bar-chart" onclick={() => (isStatsOpen = true)} /> Statistics
        </DropdownItem>
        {#if startTimeQuickSelect}
          <DropdownItem divider />
          <DropdownItem header>Start Time Quick Selection</DropdownItem>
          {#each startTimeSelectOptions.filter((stso) => stso.range !== "") as { rangeLabel, range }}
            <DropdownItem
              onclick={() => {
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
  {/if}

  {#if matchedJobs >= -1}
    <Button class="mb-1" style="margin-right: 0.25rem;" disabled outline>
      {matchedJobs == -1 ? 'Loading ...' : `${matchedJobs} jobs`}
    </Button>
  {/if}
</ButtonGroup>

{#if showFilter}
  <!-- SELECTED FILTER PILLS -->
  {#if filters.cluster}
    <Info icon="cpu" onclick={() => (isClusterOpen = true)}>
      {filters.cluster}
      {#if filters.partition}
        ({filters.partition})
      {/if}
    </Info>
  {/if}

  {#if filters.states.length != allJobStates?.length}
    <Info icon="gear-fill" onclick={() => (isJobStatesOpen = true)}>
      {filters.states.join(", ")}
      {#if filters.shared && !filters.schedule}
        ({mapSharedStates[filters.shared]})
      {:else if filters.schedule && !filters.shared}
        ({filters.schedule.charAt(0).toUpperCase() + filters.schedule?.slice(1)})
      {:else if (filters.shared && filters.schedule)}
        ({[mapSharedStates[filters.shared], (filters.schedule.charAt(0).toUpperCase() + filters.schedule.slice(1))].join(", ")})
      {/if}
    </Info>
  {:else if (filters.shared || filters.schedule)}
    <Info icon="gear-fill" onclick={() => (isJobStatesOpen = true)}>
      {#if filters.shared && !filters.schedule}
        {mapSharedStates[filters.shared]}
      {:else if filters.schedule && !filters.shared}
        {filters.schedule.charAt(0).toUpperCase() + filters.schedule?.slice(1)}
      {:else if (filters.shared && filters.schedule)}
        {[mapSharedStates[filters.shared], (filters.schedule.charAt(0).toUpperCase() + filters.schedule.slice(1))].join(", ")}
      {/if}
    </Info>
  {/if}

  {#if filters.startTime.from || filters.startTime.to}
    <Info icon="calendar-range" onclick={() => (isStartTimeOpen = true)}>
      {new Date(filters.startTime.from).toLocaleString()} - {new Date(
        filters.startTime.to,
      ).toLocaleString()}
    </Info>
  {/if}

  {#if filters.startTime.range}
    <Info icon="calendar-range" onclick={() => (isStartTimeOpen = true)}>
      {startTimeSelectOptions.find((stso) => stso.range === filters.startTime.range).rangeLabel }
    </Info>
  {/if}

  {#if filters.duration.from || filters.duration.to}
    <Info icon="stopwatch" onclick={() => (isDurationOpen = true)}>
      {Math.floor(filters.duration.from / 3600)}h:{Math.floor(
        (filters.duration.from % 3600) / 60,
      )}m -
      {Math.floor(filters.duration.to / 3600)}h:{Math.floor(
        (filters.duration.to % 3600) / 60,
      )}m
    </Info>
  {/if}

  {#if filters.duration.lessThan}
    <Info icon="stopwatch" onclick={() => (isDurationOpen = true)}>
      Duration less than {Math.floor(
        filters.duration.lessThan / 3600,
      )}h:{Math.floor((filters.duration.lessThan % 3600) / 60)}m
    </Info>
  {/if}

  {#if filters.duration.moreThan}
    <Info icon="stopwatch" onclick={() => (isDurationOpen = true)}>
      Duration more than {Math.floor(
        filters.duration.moreThan / 3600,
      )}h:{Math.floor((filters.duration.moreThan % 3600) / 60)}m
    </Info>
  {/if}

  {#if filters.tags.length != 0}
    <Info icon="tags" onclick={() => (isTagsOpen = true)}>
      {#each filters.tags as tagId}
        <Tag id={tagId} clickable={false} />
      {/each}
    </Info>
  {/if}

  {#if filters.numNodes.from != null || filters.numNodes.to != null}
    <Info icon="hdd-stack" onclick={() => (isResourcesOpen = true)}>
        Nodes: {filters.numNodes.from} - {filters.numNodes.to}
    </Info>
  {/if}

  {#if filters.numHWThreads.from != null || filters.numHWThreads.to != null}
    <Info icon="cpu" onclick={() => (isResourcesOpen = true)}>
        HWThreads: {filters.numHWThreads.from} - {filters.numHWThreads.to}
    </Info>
  {/if}

  {#if filters.numAccelerators.from != null || filters.numAccelerators.to != null}
    <Info icon="gpu-card" onclick={() => (isResourcesOpen = true)}>
        Accelerators: {filters.numAccelerators.from} - {filters.numAccelerators.to}
    </Info>
  {/if}

  {#if filters.node != null}
    <Info icon="hdd-stack" onclick={() => (isResourcesOpen = true)}>
      Node{nodeMatchLabels[filters.nodeMatch]}: {filters.node}
    </Info>
  {/if}

  {#if filters.energy.from || filters.energy.to}
    <Info icon="lightning-charge-fill" onclick={() => (isEnergyOpen = true)}>
      Total Energy: {filters.energy.from} - {filters.energy.to}
    </Info>
  {/if}

  {#if filters.stats.length > 0}
    <Info icon="bar-chart" onclick={() => (isStatsOpen = true)}>
      {filters.stats
        .map((stat) => `${stat.field}: ${stat.from} - ${stat.to}`)
        .join(", ")}
    </Info>
  {/if}
{/if}

<Cluster
  bind:isOpen={isClusterOpen}
  presetCluster={filters.cluster}
  presetPartition={filters.partition}
  {disableClusterSelection}
  setFilter={(filter) => updateFilters(filter)}
/>

<JobStates
  bind:isOpen={isJobStatesOpen}
  presetStates={filters.states}
  presetShared={filters.shared}
  presetSchedule={filters.schedule}
  setFilter={(filter) => updateFilters(filter)}
/>

<StartTime
  bind:isOpen={isStartTimeOpen}
  presetStartTime={filters.startTime}
  setFilter={(filter) => updateFilters(filter)}
/>

<Duration
  bind:isOpen={isDurationOpen}
  presetDuration={filters.duration}
  setFilter={(filter) => updateFilters(filter)}
/>

<Tags
  bind:isOpen={isTagsOpen}
  presetTags={filters.tags}
  setFilter={(filter) => updateFilters(filter)}
/>

<Resources
  bind:isOpen={isResourcesOpen}
  activeCluster={filters.cluster}
  presetNumNodes={filters.numNodes}
  presetNumHWThreads={filters.numHWThreads}
  presetNumAccelerators={filters.numAccelerators}
  presetNamedNode={filters.node}
  presetNodeMatch={filters.nodeMatch}
  setFilter={(filter) => updateFilters(filter)}
/>

<Energy
  bind:isOpen={isEnergyOpen}
  presetEnergy={filters.energy}
  setFilter={(filter) => updateFilters(filter)}
/>

<Statistics
  bind:isOpen={isStatsOpen}
  presetStats={filters.stats}
  setFilter={(filter) => updateFilters(filter)}
/>

<style>
  :global(.cc-dropdown-on-hover:hover .dropdown-menu) {
    display: block;
    margin-top: 0px;
    padding-top: 0px;
    transform: none !important;
  }
</style>
