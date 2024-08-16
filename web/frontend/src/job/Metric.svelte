<!--
    @component Metric plot wrapper with user scope selection; used in job detail view

    Properties:
      - `job Object`: The GQL job object
      - `metricName String`: The metrics name
      - `metricUnit Object`: The metrics GQL unit object
      - `nativeScope String`: The metrics native scope
      - `scopes [String]`: The scopes returned for this metric
      - `width Number`: Nested plot width
      - `rawData [Object]`: Metric data for all scopes returned for this metric
      - `isShared Bool?`: If this job used shared resources; will adapt threshold indicators accordingly in downstream plots [Default: false]
 -->

<script>
  import { 
    queryStore,
    gql,
    getContextClient 
  } from "@urql/svelte";
  // import { createEventDispatcher } from "svelte";
  import {
    InputGroup,
    InputGroupText,
    Spinner,
    Card,
  } from "@sveltestrap/sveltestrap";
  import { minScope } from "../generic/utils.js";
  import Timeseries from "../generic/plots/MetricPlot.svelte";

  export let job;
  export let metricName;
  export let metricUnit;
  export let nativeScope;
  export let scopes;
  export let width;
  export let rawData;
  export let isShared = false;

  let selectedHost = null,
    plot,
    error = null;
  let selectedScope = minScope(scopes);
  let selectedResolution = 600
  let statsPattern = /(.*)-stat$/
  let statsSeries = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null)
  let selectedScopeIndex

  // const dispatch = createEventDispatcher();
  const unit = (metricUnit?.prefix ? metricUnit.prefix : "") + (metricUnit?.base ? metricUnit.base : "")
  const resolutions = [600, 240, 60]
  const client = getContextClient();
  const subQuery = gql`
    query ($dbid: ID!, $selectedMetrics: [String!]!, $selectedScopes: [MetricScope!]!, $selectedResolution: Int) {
      singleUpdate: jobMetrics(id: $dbid, metrics: $selectedMetrics, scopes: $selectedScopes, resolution: $selectedResolution) {
        name
        scope
        metric {
          unit {
            prefix
            base
          }
          timestep
          statisticsSeries {
            min
            median
            max
          }
          series {
            hostname
            id
            data
            statistics {
              min
              avg
              max
            }
          }
        }
      }
    }
  `;

  let metricData;
  let selectedScopes = [...scopes]
  const dbid = job.id;
  const selectedMetrics = [metricName]

  function loadUpdate() {
    console.log('S> OLD DATA:', rawData)
    metricData = queryStore({
      client: client,
      query: subQuery,
      variables: { dbid, selectedMetrics, selectedScopes, selectedResolution },
    });

  };

  $: if (selectedScope == "load-all") {
    scopes = [...scopes, "socket", "core"]
    selectedScope = nativeScope
    selectedScopes = [...scopes]
    loadUpdate()
  };

  $: patternMatches = statsPattern.exec(selectedScope)
  $: if (!patternMatches) {
      selectedScopeIndex = scopes.findIndex((s) => s == selectedScope);
    } else {
      selectedScopeIndex = scopes.findIndex((s) => s == patternMatches[1]);
    }
  $: data = rawData[selectedScopeIndex];
  $: series = data?.series.filter(
    (series) => selectedHost == null || series.hostname == selectedHost,
  );

  $: if ($metricData && !$metricData.fetching) {
    rawData = $metricData.data.singleUpdate.map((x) => x.metric)
    console.log('S> NEW DATA:', rawData)
  }
  $: console.log('SelectedScope', selectedScope)
</script>

<InputGroup>
  <InputGroupText style="min-width: 150px;">
    {metricName} ({unit})
  </InputGroupText>
  <select class="form-select" bind:value={selectedScope}>
    {#each scopes as scope, index}
      <option value={scope}>{scope}</option>
      {#if statsSeries[index]}
        <option value={scope + '-stat'}>stats series ({scope})</option>
      {/if}
    {/each}
    {#if scopes.length == 1 && nativeScope != "node"}
      <option value={"load-all"}>Load all...</option>
    {/if}
  </select>
  {#if job.resources.length > 1}
    <select class="form-select" bind:value={selectedHost} disabled={patternMatches}>
      <option value={null}>All Hosts</option>
      {#each job.resources as { hostname }}
        <option value={hostname}>{hostname}</option>
      {/each}
    </select>
  {/if}
  <select class="form-select" bind:value={selectedResolution} on:change={() => {
      scopes = ["node"]
      selectedScope = "node"
      selectedScopes = [...scopes]
      loadUpdate()
    }}>
    {#each resolutions as res}
      <option value={res}>Timestep: {res}</option>
    {/each}
  </select>
</InputGroup>
{#key series}
  {#if $metricData?.fetching == true}
    <Spinner />
  {:else if error != null}
    <Card body color="danger">{error.message}</Card>
  {:else if series != null && !patternMatches}
    <Timeseries
      bind:this={plot}
      {width}
      height={300}
      cluster={job.cluster}
      subCluster={job.subCluster}
      timestep={data.timestep}
      scope={selectedScope}
      metric={metricName}
      {series}
      {isShared}
      resources={job.resources}
    />
  {:else if statsSeries[selectedScopeIndex] != null && patternMatches}
    <Timeseries
      bind:this={plot}
      {width}
      height={300}
      cluster={job.cluster}
      subCluster={job.subCluster}
      timestep={data.timestep}
      scope={selectedScope}
      metric={metricName}
      {series}
      {isShared}
      resources={job.resources}
      statisticsSeries={statsSeries[selectedScopeIndex]}
      useStatsSeries={!!statsSeries[selectedScopeIndex]}
    />
  {/if}
{/key}
