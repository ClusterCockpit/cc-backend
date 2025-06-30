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

  /* Svelte 5 Props */
  let {
    job,
    metricName,
    metricUnit,
    nativeScope,
    presetScopes,
    isShared = false,
  } = $props();

  /* Const Init */
  const client = getContextClient();
  const statsPattern = /(.*)-stat$/;
  const resampleConfig = getContext("resampling") || null;
  const resampleDefault = resampleConfig ? Math.max(...resampleConfig.resolutions) : 0;
  const unit = (metricUnit?.prefix ? metricUnit.prefix : "") + (metricUnit?.base ? metricUnit.base : "");
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

  /* State Init */
  let requestedScopes = $state(presetScopes);
  let selectedResolution = $state(resampleDefault);

  let selectedHost = $state(null);
  let zoomState = $state(null);
  let thresholdState = $state(null);

  /* Derived */
  const metricData = $derived(queryStore({
      client: client,
      query: subQuery,
      variables: { 
        dbid: job.id,
        selectedMetrics: [metricName],
        selectedScopes: [...requestedScopes],
        selectedResolution
      },
      // Never user network-only: causes reactive load-loop!
    })
  );

  const rawData = $derived($metricData?.data ? $metricData.data.singleUpdate.map((x) => x.metric) : []);
  const availableScopes = $derived($metricData?.data ? $metricData.data.singleUpdate.map((x) => x.scope) : presetScopes);
  let selectedScope = $derived(minScope(availableScopes));
  const patternMatches = $derived(statsPattern.exec(selectedScope));
  const selectedScopeIndex = $derived.by(() => {
    if (!patternMatches) {
      return availableScopes.findIndex((s) => s == selectedScope);
    } else {
      return availableScopes.findIndex((s) => s == patternMatches[1]);
    }
  });
  const selectedData = $derived(rawData[selectedScopeIndex]);
  const selectedSeries = $derived(rawData[selectedScopeIndex]?.series?.filter(
      (series) => selectedHost == null || series.hostname == selectedHost
    )
  );
  const statsSeries = $derived(rawData.map((rd) => rd?.statisticsSeries ? rd.statisticsSeries : null));

  /* Effect */
  $effect(() => {
    // Only triggered once
    if (selectedScope == "load-all") {
      requestedScopes = ["node", "socket", "core", "accelerator"];
    }
  });

  /* Functions */
  function handleZoom(detail) {
    // Buffer last zoom state to allow seamless zoom on rerender
    // console.log('Update zoomState with:', {...detail.lastZoomState})
    zoomState = detail?.lastZoomState ? {...detail.lastZoomState} : null;
    // Handle to correctly reset on summed metric scope change
    // console.log('Update thresholdState with:', detail.lastThreshold)
    thresholdState = detail?.lastThreshold ? detail.lastThreshold : null;
    // Triggers GQL
    if (detail?.newRes) { 
      // console.log('Update selectedResolution with:', detail.newRes)
      selectedResolution = detail.newRes;
    }
  };
</script>

<InputGroup class="mt-2">
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
    {#if requestedScopes.length == 1 && nativeScope != "node"}
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
{#key selectedSeries}
  {#if $metricData.fetching}
    <Spinner />
  {:else if $metricData.error}
    <Card body color="danger">{$metricData.error.message}</Card>
  {:else if selectedSeries != null && !patternMatches}
    <Timeseries
      on:zoom={({detail}) => handleZoom(detail)}
      cluster={job.cluster}
      subCluster={job.subCluster}
      timestep={selectedData.timestep}
      scope={selectedScope}
      metric={metricName}
      numaccs={job.numAcc}
      numhwthreads={job.numHWThreads}
      series={selectedSeries}
      {isShared}
      {zoomState}
      {thresholdState}
    />
  {:else if statsSeries[selectedScopeIndex] != null && patternMatches}
    <Timeseries
      on:zoom={({detail}) => handleZoom(detail)}
      cluster={job.cluster}
      subCluster={job.subCluster}
      timestep={selectedData.timestep}
      scope={selectedScope}
      metric={metricName}
      numaccs={job.numAcc}
      numhwthreads={job.numHWThreads}
      series={selectedSeries}
      {isShared}
      {zoomState}
      {thresholdState}
      statisticsSeries={statsSeries[selectedScopeIndex]}
      useStatsSeries={!!statsSeries[selectedScopeIndex]}
    />
  {/if}
{/key}
