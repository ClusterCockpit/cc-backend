<!--
    @component Job-View subcomponent; display table of metric data statistics with selectable scopes

    Properties:
    - `job Object`: The job object
 -->

<script>
  import { 
    queryStore,
    gql,
    getContextClient 
  } from "@urql/svelte";
  import { getContext } from "svelte";
  import {
    Button,
    Table,
    Input,
    InputGroup,
    InputGroupText,
    Icon,
    Row,
    Col
  } from "@sveltestrap/sveltestrap";
  import { maxScope } from "../generic/utils.js";
  import StatsTableEntry from "./StatsTableEntry.svelte";
  import MetricSelection from "../generic/select/MetricSelection.svelte";

  export let job;

  let hosts = job.resources.map((r) => r.hostname).sort(),
    selectedScopes = {},
    sorting = {},
    isMetricSelectionOpen = false,
    availableMetrics = new Set(),
    selectedMetrics = (
      getContext("cc-config")[`job_view_nodestats_selectedMetrics:${job.cluster}:${job.subCluster}`] ||
      getContext("cc-config")[`job_view_nodestats_selectedMetrics:${job.cluster}`]
    ) || getContext("cc-config")["job_view_nodestats_selectedMetrics"];

  const client = getContextClient();
  const query = gql`
    query ($dbid: ID!, $selectedMetrics: [String!]!, $selectedScopes: [MetricScope!]!) {
      scopedJobStats(id: $dbid, metrics: $selectedMetrics, scopes: $selectedScopes) {
        name
        scope
        stats {
          hostname
          id
          data {
            min
            avg
            max
          }
        }
      }
    }
  `;

  $: scopedStats = queryStore({
    client: client,
    query: query,
    variables: { dbid: job.id, selectedMetrics, selectedScopes: ["node"] },
  });

  $: console.log(">>>> RESULT:", $scopedStats?.data?.scopedJobStats)

  $: jobMetrics = $scopedStats?.data?.scopedJobStats || [];

  const scopesForMetric = (metric) =>
      jobMetrics.filter((jm) => jm.name == metric).map((jm) => jm.scope);

  $: if ($scopedStats?.data) {
    for (let metric of selectedMetrics) {
      // Not Exclusive or Multi-Node: get maxScope directly (mostly: node)
      //   -> Else: Load smallest available granularity as default as per availability
      const availableScopes = scopesForMetric(metric);
      if (job.exclusive != 1 || job.numNodes == 1) {
        if (availableScopes.includes("accelerator")) {
          selectedScopes[metric] = "accelerator";
        } else if (availableScopes.includes("core")) {
          selectedScopes[metric] = "core";
        } else if (availableScopes.includes("socket")) {
          selectedScopes[metric] = "socket";
        } else {
          selectedScopes[metric] = "node";
        }
      } else {
        selectedScopes[metric] = maxScope(availableScopes);
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

    let series = jobMetrics.find(
      (jm) => jm.name == metric && jm.scope == "node",
    )?.metric.series;
    sorting = { ...sorting };
    hosts = hosts.sort((h1, h2) => {
      let s1 = series.find((s) => s.hostname == h1)?.statistics;
      let s2 = series.find((s) => s.hostname == h2)?.statistics;
      if (s1 == null || s2 == null) return -1;

      return s.dir != "up" ? s1[stat] - s2[stat] : s2[stat] - s1[stat];
    });
  }

</script>

<Row>
  <Col class="m-2">
    <Button outline on:click={() => (isMetricSelectionOpen = true)} class="w-auto px-2" color="primary">
      Select Metrics (Selected {selectedMetrics.length} of {availableMetrics.size} available)
    </Button>
  </Col>
</Row>
<hr class="mb-1 mt-1"/>
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
            <Input type="select" bind:value={selectedScopes[metric]}>
              {#each scopesForMetric(metric, jobMetrics) as scope}
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
            {host}
            {metric}
            scope={selectedScopes[metric]}
            {jobMetrics}
          />
        {/each}
      </tr>
    {/each}
  </tbody>
</Table>

<MetricSelection
  cluster={job.cluster}
  subCluster={job.subCluster}
  configName="job_view_nodestats_selectedMetrics"
  bind:allMetrics={availableMetrics}
  bind:metrics={selectedMetrics}
  bind:isOpen={isMetricSelectionOpen}
/>
