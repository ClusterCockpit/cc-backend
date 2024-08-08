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
  import { createEventDispatcher } from "svelte";
  import {
    InputGroup,
    InputGroupText,
    Spinner,
    Card,
  } from "@sveltestrap/sveltestrap";
  import { minScope } from "../generic/utils";
  import Timeseries from "../generic/plots/MetricPlot.svelte";

  export let job;
  export let metricName;
  export let metricUnit;
  export let nativeScope;
  export let scopes;
  export let width;
  export let rawData;
  export let isShared = false;

  const dispatch = createEventDispatcher();
  const unit = (metricUnit?.prefix ? metricUnit.prefix : "") + (metricUnit?.base ? metricUnit.base : "")
    
  let selectedHost = null,
    plot,
    fetching = false,
    error = null;
  let selectedScope = minScope(scopes);

  let statsPattern = /(.*)-stat$/
  let statsSeries = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null)
  let selectedScopeIndex

  $: availableScopes = scopes;
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

  $: if (selectedScope == "load-all") dispatch("load-all");
</script>

<InputGroup>
  <InputGroupText style="min-width: 150px;">
    {metricName} ({unit})
  </InputGroupText>
  <select class="form-select" bind:value={selectedScope}>
    {#each availableScopes as scope, index}
      <option value={scope}>{scope}</option>
      {#if statsSeries[index]}
        <option value={scope + '-stat'}>stats series ({scope})</option>
      {/if}
    {/each}
    {#if availableScopes.length == 1 && nativeScope != "node"}
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
</InputGroup>
{#key series}
  {#if fetching == true}
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
