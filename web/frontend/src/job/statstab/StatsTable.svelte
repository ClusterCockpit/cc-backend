<!--:
    @component Job-View subcomponent; display table of metric data statistics with selectable scopes

    Properties:
    - `data Object`: The data object
    - `selectedMetrics [String]`: The selected metrics
    - `hosts [String]`: The list of hostnames of this job
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

  export let data = [];
  export let selectedMetrics = [];
  export let hosts = [];

  let sorting = {};
  let availableScopes = {};
  let selectedScopes = {};

  const scopesForMetric = (metric) =>
    data?.filter((jm) => jm.name == metric)?.map((jm) => jm.scope) || [];
  const setScopeForMetric = (metric, scope) =>
    selectedScopes[metric] = scope

  $: if (data && selectedMetrics) {
    for (let metric of selectedMetrics) {
      availableScopes[metric] = scopesForMetric(metric);
      // Set Initial Selection, but do not use selectedScopes: Skips reactivity
      if (availableScopes[metric].includes("accelerator")) {
        setScopeForMetric(metric, "accelerator");
      } else if (availableScopes[metric].includes("core")) {
        setScopeForMetric(metric, "core");
      } else if (availableScopes[metric].includes("socket")) {
        setScopeForMetric(metric, "socket");
      } else {
        setScopeForMetric(metric, "node");
      }

      sorting[metric] = {
        min: { dir: "up", active: false },
        avg: { dir: "up", active: false },
        max: { dir: "up", active: false },
      };
    }
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

    let stats = data.find(
      (d) => d.name == metric && d.scope == "node",
    )?.stats || [];
    sorting = { ...sorting };
    hosts = hosts.sort((h1, h2) => {
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
      <th/>
      {#each selectedMetrics as metric}
        <!-- To Match Row-2 Header Field Count-->
        <th colspan={selectedScopes[metric] == "node" ? 3 : 4}>
          <InputGroup>
            <InputGroupText>
              {metric}
            </InputGroupText>
            <Input type="select" bind:value={selectedScopes[metric]} disabled={availableScopes[metric].length === 1}>
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
          <th on:click={() => sortBy(metric, stat)}>
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
    {#each hosts as host (host)}
      <tr>
        <th scope="col">{host}</th>
        {#each selectedMetrics as metric (metric)}
          <StatsTableEntry
            {data}
            {host}
            {metric}
            scope={selectedScopes[metric]}
          />
        {/each}
      </tr>
    {/each}
  </tbody>
</Table>