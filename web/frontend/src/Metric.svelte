<script>
  import { getContext, createEventDispatcher } from "svelte";
  import Timeseries from "./plots/MetricPlot.svelte";
  import {
    InputGroup,
    InputGroupText,
    Spinner,
    Card,
  } from "@sveltestrap/sveltestrap";
  import { fetchMetrics, minScope } from "./utils";

  export let job;
  export let metricName;
  export let scopes;
  export let width;
  export let rawData;
  export let isShared = false;

  const dispatch = createEventDispatcher();
  const cluster = getContext("clusters").find(
    (cluster) => cluster.name == job.cluster,
  );
  const subCluster = cluster.subClusters.find(
    (subCluster) => subCluster.name == job.subCluster,
  );
  const metricConfig = cluster.metricConfig.find(
    (metricConfig) => metricConfig.name == metricName,
  );

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

  let from = null,
    to = null;
  export function setTimeRange(f, t) {
    (from = f), (to = t);
  }

  $: if (plot != null) plot.setTimeRange(from, to);

  export async function loadMore() {
    fetching = true;
    let response = await fetchMetrics(job, [metricName], ["core"]);
    fetching = false;

    if (response.error) {
      error = response.error;
      return;
    }

    for (let jm of response.data.jobMetrics) {
      if (jm.scope != "node") {
        scopes = [...scopes, jm.scope];
        rawData.push(jm.metric);
        statsSeries = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null)
        selectedScope = jm.scope;
        selectedScopeIndex = scopes.findIndex((s) => s == jm.scope);
        dispatch("more-loaded", jm);
      }
    }
  }

  $: if (selectedScope == "load-more") loadMore();
</script>

<InputGroup>
  <InputGroupText style="min-width: 150px;">
    {metricName} ({(metricConfig?.unit?.prefix
      ? metricConfig.unit.prefix
      : "") + (metricConfig?.unit?.base ? metricConfig.unit.base : "")})
  </InputGroupText>
  <select class="form-select" bind:value={selectedScope}>
    {#each availableScopes as scope, index}
      <option value={scope}>{scope}</option>
      {#if statsSeries[index]}
        <option value={scope + '-stat'}>stats series ({scope})</option>
      {/if}
    {/each}
    {#if availableScopes.length == 1 && metricConfig?.scope != "node"}
      <option value={"load-more"}>Load more...</option>
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
      {cluster}
      {subCluster}
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
      {cluster}
      {subCluster}
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
