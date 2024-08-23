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
    createEventDispatcher 
  } from "svelte";
  import { 
    queryStore,
    gql,
    getContextClient 
  } from "@urql/svelte";
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
  let patternMatches = false;
  let nodeOnly = false; // If, after load-all, still only node scope returned
  let statsSeries = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null);

  const dispatch = createEventDispatcher();
  const statsPattern = /(.*)-stat$/;
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

  $: if (selectedScope || pendingResolution) {
    if (!selectedResolution) {
      // Skips reactive data load on init
      selectedResolution = Number(pendingResolution)

    } else {

      if (selectedScope == "load-all") {
        selectedScopes = [...scopes, "socket", "core", "accelerator"]
      }

      if ((selectedResolution !== pendingResolution) && selectedScopes.length >= 2) {
        selectedScope = String("node")
        selectedScopes =  ["node"]
        // Instead of adding acc to load-all: always add by default if native is acc
        // selectedScopes = nativeScope == "accelerator" ? ["node", "accelerator"] : ["node"]
      }

      selectedResolution = Number(pendingResolution)

      metricData = queryStore({
        client: client,
        query: subQuery,
        variables: { dbid, selectedMetrics, selectedScopes, selectedResolution },
        // Never user network-only: causes reactive load-loop!
      });

      if ($metricData && !$metricData.fetching) {

        rawData = $metricData.data.singleUpdate.map((x) => x.metric)
        scopes  = $metricData.data.singleUpdate.map((x) => x.scope)
        statsSeries    = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null)

        // Set selected scope to min of returned scopes
        if (selectedScope == "load-all") {
          selectedScope = minScope(scopes)
          nodeOnly = (selectedScope == "node") // "node" still only scope after load-all
        }

        const statsTableData = $metricData.data.singleUpdate.filter((x) => x.scope !== "node")
        if (statsTableData.length > 0) {
          dispatch("more-loaded", statsTableData);
        }

        patternMatches = statsPattern.exec(selectedScope)

        if (!patternMatches) {
          selectedScopeIndex = scopes.findIndex((s) => s == selectedScope);
        } else {
          selectedScopeIndex = scopes.findIndex((s) => s == patternMatches[1]);
        }
      }
    }
  }

  $: data = rawData[selectedScopeIndex];
  
  $: series = data?.series?.filter(
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
    {#if scopes.length == 1 && nativeScope != "node" && !nodeOnly}
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
      statisticsSeries={statsSeries[selectedScopeIndex]}
      useStatsSeries={!!statsSeries[selectedScopeIndex]}
    />
  {/if}
{/key}
