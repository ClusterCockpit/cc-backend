<!--
  @component Main single job display component; displays plots for every metric as well as various information

  Properties:
  - `dbid Number`: The jobs DB ID
  - `username String`: Empty string if auth. is disabled, otherwise the username as string
  - `authlevel Number`: The current users authentication level
  - `roles [Number]`: Enum containing available roles
-->

<script>
  import { getContext } from "svelte";
  import { 
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    Row,
    Col,
    Card,
    Spinner,
    TabContent,
    TabPane,
    CardBody,
    CardHeader,
    CardTitle,
    Button,
  } from "@sveltestrap/sveltestrap";
  import {
    init,
    groupByScope,
    checkMetricDisabled,
  } from "./generic/utils.js";
  import Metric from "./job/Metric.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";
  import JobInfo from "./generic/joblist/JobInfo.svelte";
  import ConcurrentJobs from "./generic/helper/ConcurrentJobs.svelte";
  import JobSummary from "./job/JobSummary.svelte";
  import JobRoofline from "./job/JobRoofline.svelte";
  import EnergySummary from "./job/EnergySummary.svelte";
  import PlotGrid from "./generic/PlotGrid.svelte";
  import StatsTab from "./job/StatsTab.svelte";

  /* Svelte 5 Props */
  let {
    dbid,
    username,
    authlevel,
    roles
  } = $props();

  /* Const Init */
  // Important: init() needs to be first const declaration or contextclient will not be initialized before "const client = ..."
  // svelte-ignore state_referenced_locally
  const { query: initq } = init(`
    job(id: "${dbid}") {
      id, jobId, user, project, cluster, startTime,
      duration, numNodes, numHWThreads, numAcc, energy,
      SMT, shared, partition, subCluster, arrayJobId,
      monitoringStatus, state, walltime,
      tags { id, type, scope, name },
      resources { hostname, hwthreads, accelerators },
      metaData,
      userData { name, email },
      concurrentJobs { items { id, jobId }, count, listQuery },
      footprint { name, stat, value },
      energyFootprint { hardware, metric, value }
    }
  `);
  const client = getContextClient();
  const ccconfig = getContext("cc-config");
  const showRoofline = !!ccconfig[`jobView_showRoofline`];
  const showStatsTable = !!ccconfig[`jobView_showStatTable`];

  /* Note: Actual metric data queried in <Metric> Component, only require base infos here -> reduce backend load by requesting just stats */
  const query = gql`
    query ($dbid: ID!, $selectedMetrics: [String!]!, $selectedScopes: [MetricScope!]!) {
      scopedJobStats(id: $dbid, metrics: $selectedMetrics, scopes: $selectedScopes) {
        name
        scope
        stats {
          hostname
        }
      }
    }
  `;

  /* State Init */
  let plots = $state({});
  let isMetricsSelectionOpen = $state(false);
  let selectedMetrics = $state([]);
  let selectedScopes = $state([]);
  let totalMetrics = $state(0);

  /* Derived */
  const showSummary = $derived((!!ccconfig[`jobView_showFootprint`] || !!ccconfig[`jobView_showPolarPlot`]))
  const jobMetrics = $derived(queryStore({
      client: client,
      query: query,
      variables: { dbid, selectedMetrics, selectedScopes },
    })
  );
  
  const missingMetrics = $derived.by(() => {
    if ($initq?.data && $jobMetrics?.data) {
      let job = $initq.data.job;
      let metrics = $jobMetrics.data.scopedJobStats;
      let metricNames = $initq.data.globalMetrics.reduce((names, gm) => {
          if (gm.availability.find((av) => av.cluster === job.cluster)) {
              names.push(gm.name);
          }
          return names;
        }, []);

      return metricNames.filter(
        (metric) =>
          !metrics.some((jm) => jm.name == metric) &&
          selectedMetrics.includes(metric) && 
          !checkMetricDisabled(
            metric,
            $initq.data.job.cluster,
            $initq.data.job.subCluster,
          ),
      );
    } else {
      return []
    }
  });

  const missingHosts = $derived.by(() => {
    if ($initq?.data && $jobMetrics?.data) {
      let job = $initq.data.job;
      let metrics = $jobMetrics.data.scopedJobStats;
      let metricNames = $initq.data.globalMetrics.reduce((names, gm) => {
          if (gm.availability.find((av) => av.cluster === job.cluster)) {
              names.push(gm.name);
          }
          return names;
        }, []);

      return job.resources
        .map(({ hostname }) => ({
          hostname: hostname,
          metrics: metricNames.filter(
            (metric) =>
              !metrics.some(
                (jm) =>
                  jm.scope == "node" &&
                  jm.stats.some((s) => s.hostname == hostname),
              ),
          ),
        }))
        .filter(({ metrics }) => metrics.length > 0);
    } else {
      return [];
    }
  });

  const somethingMissing = $derived(missingMetrics?.length > 0 || missingHosts?.length > 0);

  /* Effects */
  $effect(() => {
    document.title = $initq?.fetching
      ? "Loading..."
      : $initq?.error
        ? "Error"
        : `Job ${$initq.data.job.jobId} - ClusterCockpit`;
  });

  /* On Init */
  getContext("on-init")(() => {    
    let job = $initq.data.job;
    if (!job) return;
    const pendingMetrics = (
      ccconfig[`metricConfig_jobViewPlotMetrics:${job.cluster}:${job.subCluster}`] ||
      ccconfig[`metricConfig_jobViewPlotMetrics:${job.cluster}`]
    ) || 
    $initq.data.globalMetrics.reduce((names, gm) => {
      if (gm.availability.find((av) => av.cluster === job.cluster && av.subClusters.includes(job.subCluster))) {
        names.push(gm.name);
      }
      return names;
    }, [])

    // Select default Scopes to load: Check before if any metric has accelerator scope by default
    const accScopeDefault = [...pendingMetrics].some(function (m) {
      const cluster = $initq.data.clusters.find((c) => c.name == job.cluster);
      const subCluster = cluster.subClusters.find((sc) => sc.name == job.subCluster);
      return subCluster.metricConfig.find((smc) => smc.name == m)?.scope === "accelerator";
    });

    const pendingScopes = ["node"]
    if (accScopeDefault) pendingScopes.push("accelerator")
    if (job.numNodes === 1) {
      pendingScopes.push("socket")
      pendingScopes.push("core")
    }

    selectedMetrics = [...new Set(pendingMetrics)];
    selectedScopes = [...new Set(pendingScopes)];
  });

  /* Functions */
  const orderAndMap = (grouped, selectedMetrics) =>
    selectedMetrics.map((metric) => ({
      metric: metric,
      data: grouped.find((group) => group[0].name == metric),
      disabled: checkMetricDisabled(
        metric,
        $initq.data.job.cluster,
        $initq.data.job.subCluster,
      ),
    }));
</script>

<Row class="mb-3">
  <!-- Row 1, Column 1: Job Info, Job Tags, Concurrent Jobs, Admin Message if found-->
  <Col xs={12} md={6} xl={3} class="mb-3 mb-xxl-0">
    {#if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq?.data}
      <Card class="overflow-auto" style="height: auto;">
        <TabContent> <!-- on:tab={(e) => (status = e.detail)} -->
          {#if $initq.data?.job?.metaData?.message}
            <TabPane tabId="admin-msg" tab="Admin Note" active>
              <CardBody>
                <Card body class="mb-2" color="warning">
                  <h5>Job {$initq.data?.job?.jobId} ({$initq.data?.job?.cluster})</h5>
                  The following note was added by administrators:
                </Card>
                <Card body>
                  {@html $initq.data.job.metaData.message}
                </Card>
              </CardBody>
            </TabPane>
          {/if}
          <TabPane tabId="meta-info" tab="Job Info" active={$initq.data?.job?.metaData?.message?false:true}>
            <CardBody class="pb-2">
              <JobInfo job={$initq.data.job} {username} {authlevel} {roles} showTagEdit/>
            </CardBody>
          </TabPane>
          {#if $initq.data.job.concurrentJobs != null && $initq.data.job.concurrentJobs.items.length != 0}
            <TabPane  tabId="shared-jobs">
              <span slot="tab">
                {$initq.data.job.concurrentJobs.items.length} Concurrent Jobs
              </span>
              <CardBody>
                <ConcurrentJobs cJobs={$initq.data.job.concurrentJobs} showLinks={(authlevel > roles.manager)}/>
              </CardBody>
            </TabPane>
          {/if}
        </TabContent>
      </Card>
    {:else}
      <Spinner secondary />
    {/if}
  </Col>

  <!-- Row 1, Column 2: Job Footprint, Polar Representation -->
  <Col xs={12} md={6} xl={4} xxl={3} class="mb-3 mb-xxl-0">
    {#if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq?.data}
      {#if showSummary}
        <JobSummary job={$initq.data.job}/>
      {/if}
    {:else}
      <Spinner secondary />
    {/if}
  </Col>

  <!-- Row 1, Column 3: Job Roofline; If footprint Enabled: full width, else half width -->
  <Col xs={12} md={12} xl={5} xxl={6}>
    {#if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq?.data}
      {#if showRoofline}
        <JobRoofline job={$initq.data.job} clusters={$initq.data.clusters}/>
      {/if}    
    {:else}
      <Spinner secondary />
    {/if}
  </Col>
</Row>

<!-- Row 2: Energy Information if available -->
{#if $initq?.data && $initq.data.job.energyFootprint.length != 0}
  <Row class="mb-3">
    <Col>
      <EnergySummary jobId={$initq.data.job.jobId} jobEnergy={$initq.data.job.energy} jobEnergyFootprint={$initq.data.job.energyFootprint}/>
    </Col>
  </Row>
{/if}

<!-- Metric Plot Grid -->
<Card class="mb-3">
  <CardBody>
    <Row class="mb-2">
      {#if $initq?.data}
        <Col xs="auto">
            <Button outline onclick={() => (isMetricsSelectionOpen = true)} color="primary">
              Select Metrics (Selected {selectedMetrics.length} of {totalMetrics} available)
            </Button>
        </Col>
      {/if}
    </Row>
    <hr class="mb-2"/>

    {#if $jobMetrics.error}
      <Row class="mt-2">
        <Col>
          {#if $initq?.data && ($initq.data.job?.monitoringStatus == 0 || $initq.data.job?.monitoringStatus == 2)}
            <Card body color="warning">Not monitored or archiving failed</Card>
            <br />
          {/if}
          <Card body color="danger">{$jobMetrics.error.message}</Card>
        </Col>
      </Row>
    {:else if $jobMetrics.fetching}
      <Row class="mt-2">
        <Col>
          <Spinner secondary />
        </Col>
      </Row>
    {:else if $initq?.data && $jobMetrics?.data?.scopedJobStats}
      <!-- Note: Ignore '#snippet' Error in IDE -->
      {#snippet gridContent(item)}
        {#if item.data}
          <Metric
            bind:this={plots[item.metric]}
            job={$initq.data.job}
            metricName={item.metric}
            metricUnit={$initq.data.globalMetrics.find((gm) => gm.name == item.metric)?.unit}
            nativeScope={$initq.data.globalMetrics.find((gm) => gm.name == item.metric)?.scope}
            presetScopes={item.data.map((x) => x.scope)}
            isShared={$initq.data.job.shared != "none"}
          />
        {:else if item.disabled == true}
          <Card color="info">
            <CardHeader class="mb-0">
              <b>Disabled Metric</b>
            </CardHeader>
            <CardBody>
              <p>Metric <b>{item.metric}</b> is disabled for cluster <b>{$initq.data.job.cluster}:{$initq.data.job.subCluster}</b>.</p>
              <p class="mb-1">To remove this card, open metric selection and press "Close and Apply".</p>
            </CardBody>
          </Card>
        {:else}
          <Card color="warning" class="mt-2">
            <CardHeader class="mb-0">
              <b>Missing Metric</b>
            </CardHeader>
            <CardBody>
              <p>No dataset(s) returned for <b>{item.metric}</b>.</p>
              <p class="mb-1">Metric was not found in metric store for cluster <b>{$initq.data.job.cluster}</b>.</p>
            </CardBody>
          </Card>
        {/if}
      {/snippet}

      <PlotGrid
        items={orderAndMap(
          groupByScope($jobMetrics.data.scopedJobStats),
          selectedMetrics,
        )}
        itemsPerRow={ccconfig.plotConfiguration_plotsPerRow}
        {gridContent}
      />
    {/if}
  </CardBody>
</Card>

<!-- Metadata && Statistcics Table -->
<Row class="mb-3">
  <Col>
    {#if $initq?.data}
      <Card>
        <TabContent>
          {#if somethingMissing}
            <TabPane tabId="resources" tab="Resources" active={somethingMissing}>
              <div style="margin: 10px;">
                <Card color="warning">
                  <CardHeader>
                    <CardTitle>Missing Metrics/Resources</CardTitle>
                  </CardHeader>
                  <CardBody>
                    {#if missingMetrics.length > 0}
                      <p>
                        No datasets were returned for the metrics: <b>{missingMetrics.join(
                          ", ",
                        )}</b>
                      </p>
                    {/if}
                    {#if missingHosts.length > 0}
                      <p>Metrics are missing for the following hosts:</p>
                      <ul>
                        {#each missingHosts as missing}
                          <li>
                            <b>{missing.hostname}</b>: {missing.metrics.join(", ")}
                          </li>
                        {/each}
                      </ul>
                    {/if}
                  </CardBody>
                </Card>
              </div>
            </TabPane>
          {/if}
          {#if showStatsTable}
            <!-- Includes <TabPane> Statistics Table with Independent GQL Query -->
            <StatsTab job={$initq.data.job} clusters={$initq.data.clusters} tabActive={!somethingMissing}/>
          {/if}
          <TabPane tabId="job-script" tab="Job Script">
            <div class="pre-wrapper">
              {#if $initq.data.job.metaData?.jobScript}
                <pre><code>{$initq.data.job.metaData?.jobScript}</code></pre>
              {:else}
                <Card body color="warning">No job script available</Card>
              {/if}
            </div>
          </TabPane>
          <TabPane tabId="slurm-info" tab="Slurm Info">
            <div class="pre-wrapper">
              {#if $initq.data.job.metaData?.slurmInfo}
                <pre><code>{$initq.data.job.metaData?.slurmInfo}</code></pre>
              {:else}
                <Card body color="warning"
                  >No additional slurm information available</Card
                >
              {/if}
            </div>
          </TabPane>
        </TabContent>
      </Card>
    {/if}
  </Col>
</Row>

{#if $initq?.data}
  <MetricSelection
    bind:isOpen={isMetricsSelectionOpen}
    bind:totalMetrics
    presetMetrics={selectedMetrics}
    cluster={$initq.data.job.cluster}
    subCluster={$initq.data.job.subCluster}
    configName="metricConfig_jobViewPlotMetrics"
    preInitialized
    applyMetrics={(newMetrics) => 
      selectedMetrics = [...newMetrics]
    }
  />
{/if}

<style>
  .pre-wrapper {
    font-size: 1.1rem;
    margin: 10px;
    border: 1px solid #bbb;
    border-radius: 5px;
    padding: 5px;
  }

  ul {
    columns: 2;
    -webkit-columns: 2;
    -moz-columns: 2;
  }
</style>
