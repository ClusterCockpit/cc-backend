<!-- 
  @component Data row for a single job displaying metric plots

  Properties:
  - `job Object`: The job object (GraphQL.Job)
  - `metrics [String]`: Currently selected metrics
  - `plotWidth Number`: Width of the sub-components
  - `plotHeight Number?`: Height of the sub-components [Default: 275]
  - `showFootprint Bool`: Display of footprint component for job [Default: false]
  - `previousSelect Bool`: The latest job select state for job comparison [Default: false]
  - `triggerMetricRefresh Bool?`: If changed to true from upstream, will trigger metric query [Default: false]
  - `selectJob Func`: The callback function to select a job for comparison
  - `unselectJob Func`: The callback function to unselect a job from comparison
  - `globalMetrics [Obj]`: Includes the backend supplied availabilities for cluster and subCluster
  - `clusterInfos [Obj]`: Includes the backend supplied cluster topology
-->

<script>
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import { 
    Card,
    Spinner,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    TabContent,
    TabPane,
    Button
  } from "@sveltestrap/sveltestrap";
  import { maxScope, checkMetricAvailability } from "../utils.js";
  import uPlot from "uplot";
  import JobInfo from "./JobInfo.svelte";
  import MetricPlot from "../plots/MetricPlot.svelte";
  import JobFootprint from "../helper/JobFootprint.svelte";

  /* Svelte 5 Props */
  let {
    job,
    metrics,
    plotWidth,
    plotHeight = 275,
    showFootprint = false,
    previousSelect = false,
    triggerMetricRefresh = false,
    selectJob,
    unselectJob,
    globalMetrics,
    clusterInfos
  } = $props();

  /* Const Init */
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

  /* Var Init*/
  // svelte-ignore state_referenced_locally
  let plotSync = uPlot.sync(`jobMetricStack-${job.cluster}-${job.id}`);

  /* State Init */
  let zoomStates = $state({});
  let thresholdStates = $state({});
  let rowModalOpen = $state(false);
  let rowModalLast = $state(null)
  let modalZoomStates = $state({});
  let modalThresholdStates = $state({});

  /* Derived */
  const jobId = $derived(job.id);
  const scopes = $derived.by(() => {
    if (job.numNodes == 1) {
      if (job.numAcc >= 1) return ["core", "accelerator"];
      else return ["core"];
    } else {
      return ["node"];
    };
  });

  // null lets the backend resolve the resolution from the configured resample
  // policy; zoom interactions override it with an explicit value.
  let selectedResolution = $state(null);
  let isSelected = $derived(previousSelect);
  let metricsQuery = $derived(queryStore({
      client: client,
      query: query,
      variables: { id: jobId, metrics, scopes, selectedResolution },
    })
  );

  const refinedData = $derived($metricsQuery?.data?.jobMetrics ? sortAndSelectScope(metrics, $metricsQuery.data.jobMetrics) : []);

  /* Effects */
  $effect(() => {
    if (job.state === 'running' && triggerMetricRefresh === true) {
      refreshMetrics();
    }
  });

  $effect(() => {
    if (isSelected == true && previousSelect == false) {
      selectJob(jobId)
    } else if (isSelected == false && previousSelect == true) {
      unselectJob(jobId)
    }
  });

  /* Functions */
  function handleZoom(detail, metric, isModal = false) {
    // Buffer last zoom state to allow seamless zoom on rerender
    // console.log('Update zoomState for/with:', metric, {...detail.lastZoomState})
    if (isModal) modalZoomStates[metric] = detail?.lastZoomState ? {...detail.lastZoomState} : null;
    else zoomStates[metric] = detail?.lastZoomState ? {...detail.lastZoomState} : null;
    // Handle to correctly reset on summed metric scope change
    // console.log('Update thresholdState for/with:', metric, detail.lastThreshold)
    if (isModal) modalThresholdStates[metric] = detail?.lastThreshold ? detail.lastThreshold : null;
    else thresholdStates[metric] = detail?.lastThreshold ? detail.lastThreshold : null;
    // Triggers GQL
    if (detail?.newRes) { 
      // console.log('Update selectedResolution for/with:', metric, detail.newRes)
      selectedResolution = detail.newRes;
    }
  }

  // Sveltestrap renders every TabPane header as <a href="#"> and does not forward
  // restProps to it, so draggable={false} cannot reach that anchor. NavLink only
  // preventDefaults on click, so a native link drag escapes it and leaves the portalled
  // modal unresponsive. Cancelling dragstart aborts the drag in every engine, unlike the
  // CSS-only -webkit-user-drag. Match the anchor itself, never the .nav-tabs/.nav-pills
  // wrapper: TabContent swaps that class and the guard would silently go dead.
  function cancelTabHeaderDrag(event) {
    if (event.target.closest?.(".nav-link")) event.preventDefault();
  }

  function closeRowModal() {
    rowModalOpen = false;
    rowModalLast = null;
  }

  function refreshMetrics() {
    metricsQuery = queryStore({
      client: client,
      query: query,
      variables: { id: jobId, metrics, scopes, selectedResolution },
      // requestPolicy: 'network-only' // use default cache-first for refresh
    });
  }

  function sortAndSelectScope(metricList = [], jobMetrics = []) {
    const pendingData = [];
    metricList.forEach((metricName) => {
      const pendingMetric = {
        name: metricName,
        availability: checkMetricAvailability(
          globalMetrics,
          metricName,
          job.cluster,
          job.subCluster,
        ),
        data: null,
        unit: getUnit(metricName)
      };
      const scopesData = jobMetrics.filter((jobMetric) => jobMetric.name == metricName)
      if (scopesData.length > 0) pendingMetric.data = selectScope(scopesData)
      pendingData.push(pendingMetric)
    });
    return pendingData;
  };

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

  const getUnit = (metricName) => {
    const rawUnit = globalMetrics.find((gm) => gm.name === metricName)?.unit
    return (rawUnit?.prefix ? rawUnit.prefix : "") + (rawUnit?.base ? rawUnit.base : "")
  };

</script>

<tr>
  <td>
    <JobInfo {job} bind:isSelected bind:rowModalOpen showJobSelect/>
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
    {#each refinedData as metric, i (metric?.name || i)}
      <td>
        {#if metric?.availability == "none"}
          <Card body class="mx-2" color="light">
            <p>No dataset(s) returned for <b>{metrics[i]}</b></p>
            <p class="mb-1">Metric is not configured for cluster <b>{job.cluster}</b>.</p>
          </Card>
        {:else if metric?.availability == "disabled"}
          <Card body class="mx-2" color="info">
            <p>No dataset(s) returned for <b>{metrics[i]}</b></p>
            <p class="mb-1">Metric has been disabled for subcluster <b>{job.subCluster}</b>.</p>
          </Card>
        {:else if metric?.data}
          <MetricPlot
            onZoom={(detail) => handleZoom(detail, metric.data.name)}
            height={plotHeight}
            timestep={metric.data.metric.timestep}
            scope={metric.data.scope}
            series={metric.data.metric.series}
            statisticsSeries={metric.data.metric?.statisticsSeries}
            metric={metric.data.name}
            cluster={clusterInfos.find((c) => c.name == job.cluster)}
            subCluster={job.subCluster}
            isShared={job.shared != "none"}
            zoomState={zoomStates[metric.data.name] || null}
            thresholdState={thresholdStates[metric.data.name] || null}
            {plotSync}
          />
        {:else}
          <Card body class="mx-2" color="warning">
            <p>No dataset(s) returned for <b>{metrics[i]}</b></p>
            <p class="mb-1">Metric or host was not found in metric store for cluster <b>{job.cluster}</b>:</p>
            <p class="mb-1">Identical messages in <i>{metrics[i]} column</i>: Metric not found.</p>
            <p class="mb-1">Identical messages in <i>job {job.jobId} row</i>: Host not found.</p>
          </Card>
        {/if}
      </td>
    {:else}
      <td>
        <Card body class="mx-2">No metrics selected for display.</Card>
      </td>
    {/each}
  {/if}
</tr>

<Modal isOpen={rowModalOpen} toggle={closeRowModal} size="xl">
  <ModalHeader>
    Job <a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a> ({job.cluster}, {job.subCluster}): {job?.metaData?.jobName}
  </ModalHeader>
  <ModalBody>
    <TabContent pills ondragstart={cancelTabHeaderDrag} on:tab={(e) => (rowModalLast = e.detail)}>
      {#each refinedData as metric, i (metric?.name || i)}
        <TabPane tabId="{job.jobId}-{i}" tab="{metric?.name || i} ({metric.unit})" active={rowModalLast ? (rowModalLast == `${job.jobId}-${i}`) : (i == 0)}>
          {#if metric?.availability == "none"}
            <Card body class="mx-2" color="light">
              <p>No dataset(s) returned for <b>{metrics[i]}</b></p>
              <p class="mb-1">Metric is not configured for cluster <b>{job.cluster}</b>.</p>
            </Card>
          {:else if metric?.availability == "disabled"}
            <Card body class="mx-2" color="info">
              <p>No dataset(s) returned for <b>{metrics[i]}</b></p>
              <p class="mb-1">Metric has been disabled for subcluster <b>{job.subCluster}</b>.</p>
            </Card>
          {:else if metric?.data}
            <MetricPlot
              height="600"
              onZoom={(modalDetail) => handleZoom(modalDetail, metric.data.name, true)}
              timestep={metric.data.metric.timestep}
              scope={metric.data.scope}
              series={metric.data.metric.series}
              statisticsSeries={metric.data.metric?.statisticsSeries}
              metric={metric.data.name}
              cluster={clusterInfos.find((c) => c.name == job.cluster)}
              subCluster={job.subCluster}
              isShared={job.shared != "none"}
              zoomState={modalZoomStates[metric.data.name] || null}
              thresholdState={modalThresholdStates[metric.data.name] || null}
            />
          {:else}
            <Card body class="mx-2" color="warning">
              <p>No dataset(s) returned for <b>{metrics[i]}</b></p>
              <p class="mb-1">Metric or host was not found in metric store for cluster <b>{job.cluster}</b>:</p>
              <p class="mb-1">Identical messages in <i>{metrics[i]} column</i>: Metric not found.</p>
              <p class="mb-1">Identical messages in <i>job {job.jobId} row</i>: Host not found.</p>
            </Card>
          {/if}
        </TabPane>
      {/each}
    </TabContent>
  </ModalBody>
  <ModalFooter>
    <Button onclick={closeRowModal}>Close</Button>
  </ModalFooter>
</Modal>
