<!--:
    @component Job-View subcomponent; display table of metric data statistics with selectable scopes

    Properties:
    - `hosts [String]`: The list of hostnames of this job
    - `jobStats Object`: The data object
    - `selectedMetrics [String]`: The selected metrics
 -->

<script>
  import {
    Table,
    Input,
    InputGroup,
    InputGroupText,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import StatsTableEntry from "./StatsTableEntry.svelte";

  /* Svelte 5 Props */
  let {
    hosts = [],
    jobStats = [],
    selectedMetrics = [],
  } = $props();

  /* State Init */
  let sortedHosts = $state(hosts);
  let sorting = $state(setupSorting(selectedMetrics));
  let availableScopes = $state(setupAvailable(jobStats));
  let selectedScopes = $state(setupSelected(availableScopes));

  /* Derived Init */
  const tableData = $derived(setupData(jobStats, hosts, selectedMetrics, availableScopes))

  /* Functions */
  function setupSorting(metrics) {
    let pendingSorting = {};
    if (metrics) {
      for (let metric of metrics) {
        pendingSorting[metric] = {
          min: { dir: "up", active: false },
          avg: { dir: "up", active: false },
          max: { dir: "up", active: false },
        };
      };
    };
    return pendingSorting;
  };

  function setupAvailable(data) {
    let pendingAvailable = {};
    if (data) {
      for (let d of data) {
        if (!pendingAvailable[d.name]) {
          pendingAvailable[d.name] = [d.scope]
        } else {
          pendingAvailable[d.name] = [...pendingAvailable[d.name], d.scope]
        };
      };
    };
    return pendingAvailable;
  };
  
  function setupSelected(available) {
    let pendingSelected = {};
    for (const [metric, scopes] of Object.entries(available)) {
      if (scopes.includes("accelerator")) {
        pendingSelected[metric] = "accelerator"
      } else if (scopes.includes("core")) {
        pendingSelected[metric] = "core"
      } else if (scopes.includes("socket")) {
        pendingSelected[metric] = "socket"
      } else {
        pendingSelected[metric] = "node"
      };
    };
    return pendingSelected;
  };

  function setupData(js, h, sm, as) {
    let pendingTableData = {};
    if (js) {
      for (const host of h) {
        if (!pendingTableData[host]) {
          pendingTableData[host] = {};
        };
        for (const metric of sm) {
          if (!pendingTableData[host][metric]) {
            pendingTableData[host][metric] = {};
          };
          for (const scope of as[metric]) {
            pendingTableData[host][metric][scope] = js.find((d) => d.name == metric && d.scope == scope)
              ?.stats.filter((st) => st.hostname == host && st.data != null)
              ?.sort((a, b) => a.id - b.id) || []
          };
        };
      };
    };
    return pendingTableData;
  }

  function sortBy(metric, stat) {
    let s = sorting[metric][stat];
    if (s.active) {
     s.dir = s.dir == "up" ? "down" : "up";
    } else {
      for (let metric in sorting)
        for (let stat in sorting[metric]) sorting[metric][stat].active = false;
      s.active = true;
    }

    let stats = jobStats.find(
      (js) => js.name == metric && js.scope == "node",
    )?.stats || [];
    sorting = { ...sorting };

    sortedHosts = sortedHosts.sort((h1, h2) => {
      let s1 = stats.find((s) => s.hostname == h1)?.data;
      let s2 = stats.find((s) => s.hostname == h2)?.data;
      if (s1 == null || s2 == null) return -1;

      return s.dir != "up" ? s1[stat] - s2[stat] : s2[stat] - s1[stat];
    });
  }

</script>

<Table class="mb-0">
  <thead>
    <!-- Header Row 1: Selectors -->
    <tr>
      <th></th>
      {#each selectedMetrics as metric}
        <!-- To Match Row-2 Header Field Count-->
        <th colspan={selectedScopes[metric] == "node" ? 3 : 4}>
          <InputGroup>
            <InputGroupText>
              {metric}
            </InputGroupText>
            <Input type="select" bind:value={selectedScopes[metric]} disabled={availableScopes[metric]?.length === 1}>
              {#each (availableScopes[metric] || []) as scope}
                <option value={scope}>{scope}</option>
              {/each}
            </Input>
          </InputGroup>
        </th>
      {/each}
    </tr>
    <!-- Header Row 2: Fields -->
    <tr>
      <th>Node</th>
      {#each selectedMetrics as metric}
        {#if selectedScopes[metric] != "node"}
          <th>Id</th>
        {/if}
        {#each ["min", "avg", "max"] as stat}
          <th onclick={() => sortBy(metric, stat)}>
            {stat}
            {#if selectedScopes[metric] == "node"}
              <Icon
                name="caret-{sorting[metric][stat].dir}{sorting[metric][stat]
                  .active
                  ? '-fill'
                  : ''}"
              />
            {/if}
          </th>
        {/each}
      {/each}
    </tr>
  </thead>
  <tbody>
    {#each sortedHosts as host (host)}
      <tr>
        <th scope="col">{host}</th>
        {#each selectedMetrics as metric (metric)}
          <StatsTableEntry
            data={tableData[host][metric][selectedScopes[metric]]}
            scope={selectedScopes[metric]}
          />
        {/each}
      </tr>
    {/each}
  </tbody>
</Table>