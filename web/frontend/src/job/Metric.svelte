<!--
    @component Metric plot wrapper with user scope selection; used in job detail view

    Properties:
      - `job Object`: The GQL job object
      - `metricName String`: The metrics name
      - `metricUnit Object`: The metrics GQL unit object
      - `nativeScope String`: The metrics native scope
      - `scopes [String]`: The scopes returned for this metric
      - `rawData [Object]`: Metric data for all scopes returned for this metric
      - `isShared Bool?`: If this job used shared resources; will adapt threshold indicators accordingly in downstream plots [Default: false]
 -->

<script>
  import { 
    getContext,
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
  import { 
    minScope,
  } from "../generic/utils.js";
  import Timeseries from "../generic/plots/MetricPlot.svelte";

  export let job;
  export let metricName;
  export let metricUnit;
  export let nativeScope;
  export let scopes;
  export let rawData;
  export let isShared = false;

  const resampleConfig = getContext("resampling") || null;
  const resampleDefault = resampleConfig ? Math.max(...resampleConfig.resolutions) : 0;

  let selectedHost = null;
  let error = null;
  let selectedScope = minScope(scopes);
  let selectedResolution = null;
  let pendingResolution = resampleDefault;
  let selectedScopeIndex = scopes.findIndex((s) => s == minScope(scopes));
  let patternMatches = false;
  let nodeOnly = false; // If, after load-all, still only node scope returned
  let statsSeries = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null);
  let zoomState = null;
  let pendingZoomState = null;
  let thresholdState = null;

  const dispatch = createEventDispatcher();
  const statsPattern = /(.*)-stat$/;
  const unit = (metricUnit?.prefix ? metricUnit.prefix : "") + (metricUnit?.base ? metricUnit.base : "");
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
            mean
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

  function handleZoom(detail) {
      if ( // States have to differ, causes deathloop if just set
          (pendingZoomState?.x?.min !== detail?.lastZoomState?.x?.min) &&
          (pendingZoomState?.y?.max !== detail?.lastZoomState?.y?.max)
      ) {
          pendingZoomState = {...detail.lastZoomState};
      }

      if (detail?.lastThreshold) { // Handle to correctly reset on summed metric scope change
        thresholdState = detail.lastThreshold;
      } else {
        thresholdState = null;
      }

      if (detail?.newRes) { // Triggers GQL
          pendingResolution = detail.newRes;
      }
  };

  let metricData;
  let selectedScopes = [...scopes];
  const dbid = job.id;
  const selectedMetrics = [metricName];

  $: if (selectedScope || pendingResolution) {

    if (resampleConfig && !selectedResolution) {
      // Skips reactive data load on init || Only if resampling is enabled
      selectedResolution = Number(pendingResolution)

    } else {
      if (selectedScope == "load-all") {
        selectedScopes = [...scopes, "socket", "core", "accelerator"]
      }

      if (resampleConfig && pendingResolution) {
        selectedResolution = Number(pendingResolution)
      }

      metricData = queryStore({
        client: client,
        query: subQuery,
        variables: { dbid, selectedMetrics, selectedScopes, selectedResolution: (resampleConfig ? selectedResolution : 0) },
        // Never user network-only: causes reactive load-loop!
      });

      if ($metricData && !$metricData.fetching) {
        rawData = $metricData.data.singleUpdate.map((x) => x.metric)
        scopes  = $metricData.data.singleUpdate.map((x) => x.scope)
        statsSeries    = rawData.map((data) => data?.statisticsSeries ? data.statisticsSeries : null)

        // Keep Zoomlevel if ResChange By Zoom
        if (pendingZoomState) {
          zoomState = {...pendingZoomState}
        }

        // On additional scope request
        if (selectedScope == "load-all") {
          // Push scope to statsTable (Needs to be in this case, else newly selected 'Metric.svelte' renders cause statsTable race condition)
          const statsTableData = $metricData.data.singleUpdate.filter((x) => x.scope !== "node")
          if (statsTableData.length > 0) {
            dispatch("more-loaded", statsTableData);
          }
          // Set selected scope to min of returned scopes
          selectedScope = minScope(scopes)
          nodeOnly = (selectedScope == "node") // "node" still only scope after load-all
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

<InputGroup class="mt-2">
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
</InputGroup>
{#key series}
  {#if $metricData?.fetching == true}
    <Spinner />
  {:else if error != null}
    <Card body color="danger">{error.message}</Card>
  {:else if series != null && !patternMatches}
    <Timeseries
      on:zoom={({detail}) => { handleZoom(detail) }}
      cluster={job.cluster}
      subCluster={job.subCluster}
      timestep={data.timestep}
      scope={selectedScope}
      metric={metricName}
      numaccs={job.numAcc}
      numhwthreads={job.numHWThreads}
      {series}
      {isShared}
      {zoomState}
      {thresholdState}
    />
  {:else if statsSeries[selectedScopeIndex] != null && patternMatches}
    <Timeseries
      on:zoom={({detail}) => { handleZoom(detail) }}
      cluster={job.cluster}
      subCluster={job.subCluster}
      timestep={data.timestep}
      scope={selectedScope}
      metric={metricName}
      numaccs={job.numAcc}
      numhwthreads={job.numHWThreads}
      {series}
      {isShared}
      {zoomState}
      {thresholdState}
      statisticsSeries={statsSeries[selectedScopeIndex]}
      useStatsSeries={!!statsSeries[selectedScopeIndex]}
    />
  {/if}
{/key}
