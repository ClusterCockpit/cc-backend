<!-- 
    @component

    Properties:
    - job:        GraphQL.Job (constant/key)
    - metrics:    [String]    (can change)
    - plotWidth:  Number
    - plotHeight: Number
 -->

<script>
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import { getContext } from "svelte";
  import { Card, Spinner } from "@sveltestrap/sveltestrap";
  import MetricPlot from "../plots/MetricPlot.svelte";
  import JobInfo from "./JobInfo.svelte";
  import JobFootprint from "../JobFootprint.svelte";
  import { maxScope, checkMetricDisabled } from "../utils.js";

  export let job;
  export let metrics;
  export let plotWidth;
  export let plotHeight = 275;
  export let showFootprint;

  let { id } = job;
  let scopes = job.numNodes == 1
    ? job.numAcc >= 1
      ? ["core", "accelerator"]
      : ["core"]
    : ["node"];

  const cluster = getContext("clusters").find((c) => c.name == job.cluster);
  const client = getContextClient();
  const query = gql`
    query ($id: ID!, $metrics: [String!]!, $scopes: [MetricScope!]!) {
      jobMetrics(id: $id, metrics: $metrics, scopes: $scopes) {
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

  $: metricsQuery = queryStore({
    client: client,
    query: query,
    variables: { id, metrics, scopes },
  });
  
  export function refresh() {
    metricsQuery = queryStore({
      client: client,
      query: query,
      variables: { id, metrics, scopes },
      // requestPolicy: 'network-only' // use default cache-first for refresh
    });
  }

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

  if (job.monitoringStatus) refresh();
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
          view="list"
        />
      </td>
    {/if}
    {#each sortAndSelectScope($metricsQuery.data.jobMetrics) as metric, i (metric || i)}
      <td>
        <!-- Subluster Metricconfig remove keyword for jobtables (joblist main, user joblist, project joblist) to be used here as toplevel case-->
        {#if metric.disabled == false && metric.data}
          <MetricPlot
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
            resources={job.resources}
            numhwthreads={job.numHWThreads}
            numaccs={job.numAcc}
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
