<!--
    @component Main single job display component; displays plots for every metric as well as various information

    Properties:
    - `dbid Number`: The jobs DB ID
    - `username String`: Empty string if auth. is disabled, otherwise the username as string
    - `authlevel Number`: The current users authentication level
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
  } from "@sveltestrap/sveltestrap";
  import { getContext } from "svelte";
  import {
    init,
    groupByScope,
    checkMetricDisabled,
    transformDataForRoofline,
  } from "./generic/utils.js";
  import Metric from "./job/Metric.svelte";
  import StatsTable from "./job/StatsTable.svelte";
  import JobSummary from "./job/JobSummary.svelte";
  import EnergySummary from "./job/EnergySummary.svelte";
  import ConcurrentJobs from "./generic/helper/ConcurrentJobs.svelte";
  import PlotGrid from "./generic/PlotGrid.svelte";
  import Roofline from "./generic/plots/Roofline.svelte";
  import JobInfo from "./generic/joblist/JobInfo.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";

  export let dbid;
  export let username;
  export let authlevel;
  export let roles;

 // Setup General

 const ccconfig = getContext("cc-config")

 let isMetricsSelectionOpen = false,
    selectedMetrics = [],
    selectedScopes = [];

  let plots = {},
    roofWidth,
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
            duration, numNodes, numHWThreads, numAcc, energy,
            SMT, exclusive, partition, subCluster, arrayJobId,
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

  $: jobMetrics = queryStore({
    client: client,
    query: query,
    variables: { dbid, selectedMetrics, selectedScopes },
  });

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

<Row class="mb-3">
  <!-- Column 1: Job Info, Job Tags, Concurrent Jobs, Admin Message if found-->
  <Col xs={12} md={6} xl={3} class="mb-3 mb-xxl-0">
    {#if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq.data}
      <Card class="overflow-auto" style="height: 400px;">
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
              <JobInfo job={$initq.data.job} {username} {authlevel} {roles} showTagedit/>
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

  <!-- Column 2: Job Footprint, Polar Representation, Heuristic Summary -->
  <Col xs={12} md={6} xl={4} xxl={3} class="mb-3 mb-xxl-0">
    {#if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq?.data && $jobMetrics?.data}
      <JobSummary job={$initq.data.job} jobMetrics={$jobMetrics.data.jobMetrics}/>
    {:else}
      <Spinner secondary />
    {/if}
  </Col>

  <!-- Column 3: Job Roofline; If footprint Enabled: full width, else half width -->
  <Col xs={12} md={12} xl={5} xxl={6}>
    {#if $initq.error || $jobMetrics.error}
      <Card body color="danger">
        <p>Initq Error: {$initq.error?.message}</p>
        <p>jobMetrics Error: {$jobMetrics.error?.message}</p>
      </Card>
    {:else if $initq?.data && $jobMetrics?.data}
      <Card style="height: 400px;">
        <div bind:clientWidth={roofWidth}>
          <Roofline
            allowSizeChange={true}
            width={roofWidth}
            renderTime={true}
            subCluster={$initq.data.clusters
              .find((c) => c.name == $initq.data.job.cluster)
              .subClusters.find((sc) => sc.name == $initq.data.job.subCluster)}
            data={transformDataForRoofline(
              $jobMetrics.data?.jobMetrics?.find(
                (m) => m.name == "flops_any" && m.scope == "node",
              )?.metric,
              $jobMetrics.data?.jobMetrics?.find(
                (m) => m.name == "mem_bw" && m.scope == "node",
              )?.metric,
            )}
          />
        </div>
      </Card>
    {:else}
        <Spinner secondary />
    {/if}
  </Col>
</Row>

{#if $initq?.data && $initq.data.job.energyFootprint.length != 0}
  <Row class="mb-3">
    <Col>
      <EnergySummary jobId={$initq.data.job.jobId} jobEnergy={$initq.data.job.energy} jobEnergyFootprint={$initq.data.job.energyFootprint}/>
    </Col>
  </Row>
{/if}

<Card class="mb-3">
  <CardBody>
    <Row class="mb-2">
      {#if $initq.data}
        <Col xs="auto">
            <Button outline on:click={() => (isMetricsSelectionOpen = true)} color="primary">
              Select Metrics
            </Button>
        </Col>
      {/if}
    </Row>
    <hr class="mb-2"/>

    {#if $jobMetrics.error}
      <Row class="mt-2">
        <Col>
          {#if $initq.data.job.monitoringStatus == 0 || $initq.data.job.monitoringStatus == 2}
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
    {:else if $initq?.data && $jobMetrics?.data?.jobMetrics}
      <PlotGrid
        let:item
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
            on:more-loaded={({ detail }) => statsTable.moreLoaded(detail)}
            job={$initq.data.job}
            metricName={item.metric}
            metricUnit={$initq.data.globalMetrics.find((gm) => gm.name == item.metric)?.unit}
            nativeScope={$initq.data.globalMetrics.find((gm) => gm.name == item.metric)?.scope}
            rawData={item.data.map((x) => x.metric)}
            scopes={item.data.map((x) => x.scope)}
            isShared={$initq.data.job.exclusive != 1}
          />
        {:else}
          <Card body color="warning" class="mt-2"
            >No dataset returned for <code>{item.metric}</code></Card
          >
        {/if}
      </PlotGrid>
    {/if}
  </CardBody>
</Card>

<Row class="mb-3">
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
            class="overflow-x-auto"
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
