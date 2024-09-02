<!-- 
    @component Data row for a single job displaying metric plots

    Properties:
    - `job Object`: The job object (GraphQL.Job)
    - `metrics [String]`: Currently selected metrics
    - `plotWidth Number`: Width of the sub-components
    - `plotHeight Number?`: Height of the sub-components [Default: 275]
    - `showFootprint Bool`: Display of footprint component for job
    - `triggerMetricRefresh Bool?`: If changed to true from upstream, will trigger metric query
 -->

<script>
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import { getContext } from "svelte";
  import { Card, Spinner } from "@sveltestrap/sveltestrap";
  import { maxScope, checkMetricDisabled } from "../utils.js";
  import JobInfo from "./JobInfo.svelte";
  import MetricPlot from "../plots/MetricPlot.svelte";
  import JobFootprint from "../helper/JobFootprint.svelte";

  export let job;
  export let metrics;
  export let plotWidth;
  export let plotHeight = 275;
  export let showFootprint;
  export let triggerMetricRefresh = false;

  let { id } = job;
  let scopes = job.numNodes == 1
    ? job.numAcc >= 1
      ? ["core", "accelerator"]
      : ["core"]
    : ["node"];
  let selectedResolution = 600;
  let zoomStates = {};

  const cluster = getContext("clusters").find((c) => c.name == job.cluster);
  const client = getContextClient();
  const query = gql`
    query ($id: ID!, $metrics: [String!]!, $scopes: [MetricScope!]!, $selectedResolution: Int) {
      jobMetrics(id: $id, metrics: $metrics, scopes: $scopes, resolution: $selectedResolution) {
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

  function handleZoom(detail, metric) {
    if (
        (zoomStates[metric]?.x?.min !== detail?.lastZoomState?.x?.min) &&
        (zoomStates[metric]?.y?.max !== detail?.lastZoomState?.y?.max)
    ) {
        zoomStates[metric] = {...detail.lastZoomState}
    }

    if (detail?.newRes) { // Triggers GQL
        selectedResolution = detail.newRes
    }
  }

  $: metricsQuery = queryStore({
    client: client,
    query: query,
    variables: { id, metrics, scopes, selectedResolution },
  });
  
  function refreshMetrics() {
    metricsQuery = queryStore({
      client: client,
      query: query,
      variables: { id, metrics, scopes, selectedResolution },
      // requestPolicy: 'network-only' // use default cache-first for refresh
    });
  }

  $: if (job.state === 'running' && triggerMetricRefresh === true) {
    refreshMetrics();
  }

  // Helper
  const selectScope = (jobMetrics) =>
    jobMetrics.reduce(
      (a, b) =>
        maxScope([a.scope, b.scope]) == a.scope
          ? job.numNodes > 1
            ? a
            : b
          : job.numNodes > 1
            ? b
            : a,
      jobMetrics[0],
    );

  const sortAndSelectScope = (jobMetrics) =>
    metrics
      .map((name) => jobMetrics.filter((jobMetric) => jobMetric.name == name))
      .map((jobMetrics) => ({
        disabled: false,
        data: jobMetrics.length > 0 ? selectScope(jobMetrics) : null,
      }))
      .map((jobMetric) => {
        if (jobMetric.data) {
          return {
            disabled: checkMetricDisabled(
              jobMetric.data.name,
              job.cluster,
              job.subCluster,
            ),
            data: jobMetric.data,
          };
        } else {
          return jobMetric;
        }
      });

</script>

<tr>
  <td>
    <JobInfo {job} />
  </td>
  {#if job.monitoringStatus == 0 || job.monitoringStatus == 2}
    <td colspan={metrics.length}>
      <Card body color="warning">Not monitored or archiving failed</Card>
    </td>
  {:else if $metricsQuery.fetching}
    <td colspan={metrics.length} style="text-align: center;">
      <Spinner secondary />
    </td>
  {:else if $metricsQuery.error}
    <td colspan={metrics.length}>
      <Card body color="danger" class="mb-3">
        {$metricsQuery.error.message.length > 500
          ? $metricsQuery.error.message.substring(0, 499) + "..."
          : $metricsQuery.error.message}
      </Card>
    </td>
  {:else}
    {#if showFootprint}
      <td>
        <JobFootprint
          {job}
          width={plotWidth}
          height="{plotHeight}px"
          displayTitle={false}
        />
      </td>
    {/if}
    {#each sortAndSelectScope($metricsQuery.data.jobMetrics) as metric, i (metric || i)}
      <td>
        <!-- Subluster Metricconfig remove keyword for jobtables (joblist main, user joblist, project joblist) to be used here as toplevel case-->
        {#if metric.disabled == false && metric.data}
          <MetricPlot
            on:zoom={({detail}) => { handleZoom(detail, metric.data.name) }}
            width={plotWidth}
            height={plotHeight}
            timestep={metric.data.metric.timestep}
            scope={metric.data.scope}
            series={metric.data.metric.series}
            statisticsSeries={metric.data.metric.statisticsSeries}
            metric={metric.data.name}
            {cluster}
            subCluster={job.subCluster}
            isShared={job.exclusive != 1}
            numhwthreads={job.numHWThreads}
            numaccs={job.numAcc}
            zoomState={zoomStates[metric.data.name]}
          />
        {:else if metric.disabled == true && metric.data}
          <Card body color="info"
            >Metric disabled for subcluster <code
              >{metric.data.name}:{job.subCluster}</code
            ></Card
          >
        {:else}
          <Card body color="warning">No dataset returned</Card>
        {/if}
      </td>
    {/each}
  {/if}
</tr>
