<!--
    @component Main single job display component; displays plots for every metric as well as various information

    Properties:
    - `username String`: Empty string if auth. is disabled, otherwise the username as string
    - `authlevel Number`: The current users authentication level
    - `clusters [String]`: List of cluster names
    - `roles [Number]`: Enum containing available roles
 -->

<script>
  import { 
    queryStore,
    gql,
    getContextClient 
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
    Icon,
  } from "@sveltestrap/sveltestrap";
  import { getContext } from "svelte";
  import {
    init,
    groupByScope,
    checkMetricDisabled,
    transformDataForRoofline,
  } from "./generic/utils.js";
  import Metric from "./job/Metric.svelte";
  import TagManagement from "./job/TagManagement.svelte";
  import StatsTable from "./job/StatsTable.svelte";
  import JobSummary from "./job/JobSummary.svelte";
  import ConcurrentJobs from "./generic/helper/ConcurrentJobs.svelte";
  import PlotTable from "./generic/PlotTable.svelte";
  import Roofline from "./generic/plots/Roofline.svelte";
  import JobInfo from "./generic/joblist/JobInfo.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";

  export let dbid;
  export let authlevel;
  export let roles;

 // Setup General

 const ccconfig = getContext("cc-config")

 let isMetricsSelectionOpen = false,
    showFootprint = !!ccconfig[`job_view_showFootprint`],
    selectedMetrics = [],
    selectedScopes = [];

  let plots = {},
    jobTags,
    statsTable

  let missingMetrics = [],
    missingHosts = [],
    somethingMissing = false;

  // Setup GQL
  // First: Add Job Query to init function -> Only requires DBID as argument, received via URL-ID
  // Second: Trigger jobMetrics query with now received jobInfos (scopes: from job metadata, selectedMetrics: from config or all, job: from url-id)

  const { query: initq } = init(`
        job(id: "${dbid}") {
            id, jobId, user, project, cluster, startTime,
            duration, numNodes, numHWThreads, numAcc,
            SMT, exclusive, partition, subCluster, arrayJobId,
            monitoringStatus, state, walltime,
            tags { id, type, name },
            resources { hostname, hwthreads, accelerators },
            metaData,
            userData { name, email },
            concurrentJobs { items { id, jobId }, count, listQuery },
            footprint { name, stat, value }
        }
    `);

  const client = getContextClient();
  const query = gql`
    query ($dbid: ID!, $selectedMetrics: [String!]!, $selectedScopes: [MetricScope!]!) {
      jobMetrics(id: $dbid, metrics: $selectedMetrics, scopes: $selectedScopes) {
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

  $: jobMetrics = queryStore({
    client: client,
    query: query,
    variables: { dbid, selectedMetrics, selectedScopes },
  });

  function loadAllScopes() {
    selectedScopes = [...selectedScopes, "socket", "core"]
    jobMetrics = queryStore({
      client: client,
      query: query,
      variables: { dbid, selectedMetrics, selectedScopes},
    });
  }

  // Handle Job Query on Init -> is not executed anymore
  getContext("on-init")(() => {
    let job = $initq.data.job;
    if (!job) return;

    const pendingMetrics = [
      "flops_any",
      "mem_bw",
      ...(ccconfig[`job_view_selectedMetrics:${job.cluster}`] ||
        $initq.data.globalMetrics.reduce((names, gm) => {
          if (gm.availability.find((av) => av.cluster === job.cluster)) {
            names.push(gm.name);
          }
          return names;
        }, [])
      ),
      ...(ccconfig[`job_view_polarPlotMetrics:${job.cluster}`] ||
        ccconfig[`job_view_polarPlotMetrics`]
      ),
      ...(ccconfig[`job_view_nodestats_selectedMetrics:${job.cluster}`] ||
        ccconfig[`job_view_nodestats_selectedMetrics`]
      ),
    ];

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

  // Interactive Document Title
  $: document.title = $initq.fetching
    ? "Loading..."
    : $initq.error
      ? "Error"
      : `Job ${$initq.data.job.jobId} - ClusterCockpit`;

  // Find out what metrics or hosts are missing:
  $: if ($initq?.data && $jobMetrics?.data?.jobMetrics) {
    let job = $initq.data.job,
      metrics = $jobMetrics.data.jobMetrics,
      metricNames = $initq.data.globalMetrics.reduce((names, gm) => {
        if (gm.availability.find((av) => av.cluster === job.cluster)) {
            names.push(gm.name);
        }
        return names;
      }, []);

    // Metric not found in JobMetrics && Metric not explicitly disabled in config or deselected: Was expected, but is Missing
    missingMetrics = metricNames.filter(
      (metric) =>
        !metrics.some((jm) => jm.name == metric) &&
        selectedMetrics.includes(metric) && 
        !checkMetricDisabled(
          metric,
          $initq.data.job.cluster,
          $initq.data.job.subCluster,
        ),
    );
    missingHosts = job.resources
      .map(({ hostname }) => ({
        hostname: hostname,
        metrics: metricNames.filter(
          (metric) =>
            !metrics.some(
              (jm) =>
                jm.scope == "node" &&
                jm.metric.series.some((series) => series.hostname == hostname),
            ),
        ),
      }))
      .filter(({ metrics }) => metrics.length > 0);
    somethingMissing = missingMetrics.length > 0 || missingHosts.length > 0;
  }

  // Helper
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

<Row cols={4} class="mb-2">
  <!-- Column 1: Job Info, Concurrent Jobs, Admin Message if found-->
  <Col xs={3}>
    {#if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq.data}
      <Card class="overflow-auto" style="height: 400px;">
        <TabContent> <!-- on:tab={(e) => (status = e.detail)} -->
          <TabPane tabId="meta-info" tab="Job Info" active>
            <CardBody class="pb-2">
              <JobInfo job={$initq.data.job} {jobTags} />
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
          {#if $initq.data?.job?.metaData?.message}
            <TabPane tabId="admin-msg" tab="Admin Note">
              <CardBody>
                <p>This note was added by administrators:</p>
                <hr/>
                <p>{@html $initq.data.job.metaData.message}</p>
              </CardBody>
            </TabPane>
          {/if}
        </TabContent>
      </Card>
    {:else}
      <Spinner secondary />
    {/if}
  </Col>

  <!-- If enabled:  Column 2: Job Footprint, Polar Representation, Heuristic Summary -->
   {#if showFootprint}
    <Col xs={3}>
      {#if $initq.error}
        <Card body color="danger">{$initq.error.message}</Card>
      {:else if $initq?.data && $jobMetrics?.data}
        <JobSummary job={$initq.data.job} jobMetrics={$jobMetrics.data.jobMetrics}/>
      {:else}
        <Spinner secondary />
      {/if}
    </Col>
  {:else}
    <Col xs={3}/>
  {/if}

  <!-- Column 3: Spacer -->
  <Col xs={2}/>

  <!-- Column 4: Job Roofline -->
  <Col xs={4}>
    {#if $initq.error || $jobMetrics.error}
      <Card body color="danger">
        <p>Initq Error: {$initq.error?.message}</p>
        <p>jobMetrics Error: {$jobMetrics.error?.message}</p>
      </Card>
    {:else if $initq?.data && $jobMetrics?.data}
      <Card style="height: 400px;">
        <Roofline
          renderTime={true}
          subCluster={$initq.data.clusters
            .find((c) => c.name == $initq.data.job.cluster)
            .subClusters.find((sc) => sc.name == $initq.data.job.subCluster)}
          data={transformDataForRoofline(
            $jobMetrics.data.jobMetrics.find(
              (m) => m.name == "flops_any" && m.scope == "node",
            )?.metric,
            $jobMetrics.data.jobMetrics.find(
              (m) => m.name == "mem_bw" && m.scope == "node",
            )?.metric,
          )}
        />
      </Card>
    {:else}
        <Spinner secondary />
    {/if}
  </Col>
</Row>

<hr/>

<Row class="mb-2">
  <Col xs="auto">
    {#if $initq.data}
      <TagManagement job={$initq.data.job} bind:jobTags />
    {/if}
  </Col>
  <Col xs="auto">
    {#if $initq.data}
      <Button outline on:click={() => (isMetricsSelectionOpen = true)}>
        <Icon name="graph-up" /> Metrics
      </Button>
    {/if}
  </Col>
</Row>
<Row>
  <Col>
    {#if $jobMetrics.error}
      {#if $initq.data.job.monitoringStatus == 0 || $initq.data.job.monitoringStatus == 2}
        <Card body color="warning">Not monitored or archiving failed</Card>
        <br />
      {/if}
      <Card body color="danger">{$jobMetrics.error.message}</Card>
    {:else if $jobMetrics.fetching}
      <Spinner secondary />
    {:else if $initq?.data && $jobMetrics?.data?.jobMetrics}
      <PlotTable
        let:item
        let:width
        renderFor="job"
        items={orderAndMap(
          groupByScope($jobMetrics.data.jobMetrics),
          selectedMetrics,
        )}
        itemsPerRow={ccconfig.plot_view_plotsPerRow}
      >
        {#if item.data}
          <Metric
            bind:this={plots[item.metric]}
            on:load-all={loadAllScopes}
            job={$initq.data.job}
            metricName={item.metric}
            metricUnit={$initq.data.globalMetrics.find((gm) => gm.name == item.metric)?.unit}
            nativeScope={$initq.data.globalMetrics.find((gm) => gm.name == item.metric)?.scope}
            rawData={item.data.map((x) => x.metric)}
            scopes={item.data.map((x) => x.scope)}
            {width}
            isShared={$initq.data.job.exclusive != 1}
          />
        {:else}
          <Card body color="warning"
            >No dataset returned for <code>{item.metric}</code></Card
          >
        {/if}
      </PlotTable>
    {/if}
  </Col>
</Row>

<hr/>

<Row>
  <Col>
    {#if $initq.data}
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
                        No data at all is available for the metrics: {missingMetrics.join(
                          ", ",
                        )}
                      </p>
                    {/if}
                    {#if missingHosts.length > 0}
                      <p>Some metrics are missing for the following hosts:</p>
                      <ul>
                        {#each missingHosts as missing}
                          <li>
                            {missing.hostname}: {missing.metrics.join(", ")}
                          </li>
                        {/each}
                      </ul>
                    {/if}
                  </CardBody>
                </Card>
              </div>
            </TabPane>
          {/if}
          <TabPane
            tabId="stats"
            tab="Statistics Table"
            active={!somethingMissing}
          >
            {#if $jobMetrics?.data?.jobMetrics}
              {#key $jobMetrics.data.jobMetrics}
                <StatsTable
                  bind:this={statsTable}
                  job={$initq.data.job}
                  jobMetrics={$jobMetrics.data.jobMetrics}
                />
              {/key}
            {/if}
          </TabPane>
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

{#if $initq.data}
  <MetricSelection
    cluster={$initq.data.job.cluster}
    configName="job_view_selectedMetrics"
    bind:metrics={selectedMetrics}
    bind:isOpen={isMetricsSelectionOpen}
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
