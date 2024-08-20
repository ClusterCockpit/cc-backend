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
  let selectedResolution;
  let pendingResolution = 600;
  let selectedScopeIndex = scopes.findIndex((s) => s == minScope(scopes));
  const statsPattern = /(.*)-stat$/;
  let patternMatches = false;
  let statsSeries = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null);

  // const dispatch = createEventDispatcher();
  const unit = (metricUnit?.prefix ? metricUnit.prefix : "") + (metricUnit?.base ? metricUnit.base : "");
  const resolutions = [600, 240, 60] // DEV: Make configable
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

  $: if (selectedScope == "load-all" || pendingResolution) {
    
    if (selectedScope == "load-all") {
      console.log('Triggered load-all')
      selectedScopes = [...scopes, "socket", "core"]
    } else {
      console.log("Triggered scope switch:", selectedScope, pendingResolution)
    }

    // What if accelerator scope / native core scopes?
    if ((selectedResolution !== pendingResolution) && selectedScopes.length >= 2) {
      selectedScope = String("node")
      selectedScopes = ["node"]
      console.log("New Resolution: Reset to node scope")
    } else {
      console.log("New Resolution: No change in Res or just node scope")
    }

    if (!selectedResolution) {
      selectedResolution = Number(pendingResolution)
    } else {
      selectedResolution = Number(pendingResolution)

      metricData = queryStore({
        client: client,
        query: subQuery,
        variables: { dbid, selectedMetrics, selectedScopes, selectedResolution },
      // requestPolicy: "network-only",
      });

      if ($metricData && !$metricData.fetching) {
        console.log('Trigger Data Handling')

        rawData = $metricData.data.singleUpdate.map((x) => x.metric)
        scopes  = $metricData.data.singleUpdate.map((x) => x.scope)
        statsSeries   = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null)

        // Handle Selected Scope on load-all
        if (selectedScope == "load-all") {
          selectedScope = minScope(scopes)
          console.log('Set New SelectedScope after Load-All', selectedScope, scopes)
        } else {
          console.log('Set New SelectedScope', selectedScope)
        }

        patternMatches = statsPattern.exec(selectedScope)
        if (!patternMatches) {
          selectedScopeIndex = scopes.findIndex((s) => s == selectedScope);
          console.log("Selected Index # from Array", selectedScopeIndex, scopes)
        } else {
          selectedScopeIndex = scopes.findIndex((s) => s == patternMatches[1]);
          console.log("Selected Stats Index # from Array", selectedScopeIndex, scopes)
        }
      }
    }
  }

  $: data = rawData[selectedScopeIndex];
  
  $: series = data?.series.filter(
    (series) => selectedHost == null || series.hostname == selectedHost,
  );
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
  <select class="form-select" bind:value={pendingResolution}>
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
