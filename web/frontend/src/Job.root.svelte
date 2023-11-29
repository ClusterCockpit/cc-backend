<script>
    import {
        init,
        groupByScope,
        fetchMetricsStore,
        checkMetricDisabled,
        transformDataForRoofline
    } from "./utils.js";
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
    } from "sveltestrap";
    import PlotTable from "./PlotTable.svelte";
    import Metric from "./Metric.svelte";
    import Polar from "./plots/Polar.svelte";
    import Roofline from "./plots/Roofline.svelte";
    import JobInfo from "./joblist/JobInfo.svelte";
    import TagManagement from "./TagManagement.svelte";
    import MetricSelection from "./MetricSelection.svelte";
    import StatsTable from "./StatsTable.svelte";
    import JobFootprint from "./JobFootprint.svelte";
    import { getContext } from "svelte";

    export let dbid;
    export let authlevel;
    export let roles;

    const accMetrics = ['acc_utilization', 'acc_mem_used', 'acc_power', 'nv_mem_util', 'nv_sm_clock', 'nv_temp'];
    let accNodeOnly

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
            flopsAnyAvg, memBwAvg, loadAvg
        }
    `);

    const ccconfig = getContext("cc-config"),
          clusters = getContext("clusters"),
          metrics  = getContext("metrics")

    let isMetricsSelectionOpen = false,
        selectedMetrics = [],
        isFetched = new Set();
    const [jobMetrics, startFetching] = fetchMetricsStore();
    getContext("on-init")(() => {
        let job = $initq.data.job;
        if (!job) return;

        selectedMetrics =
            ccconfig[`job_view_selectedMetrics:${job.cluster}`] ||
            clusters
                .find((c) => c.name == job.cluster)
                .metricConfig.map((mc) => mc.name);

        let toFetch = new Set([
            "flops_any",
            "mem_bw",
            ...selectedMetrics,
            ...(ccconfig[`job_view_polarPlotMetrics:${job.cluster}`] ||
                ccconfig[`job_view_polarPlotMetrics`]),
            ...(ccconfig[`job_view_nodestats_selectedMetrics:${job.cluster}`] ||
                ccconfig[`job_view_nodestats_selectedMetrics`]),
        ]);

        // Select default Scopes to load: Check before if accelerator metrics are not on accelerator scope by default
        accNodeOnly = [...toFetch].some(function(m) {
            if (accMetrics.includes(m)) {
                const mc = metrics(job.cluster, m)
                return mc.scope !== 'accelerator'
            } else {
                return false
            }
        })

        if (job.numAcc === 0 || accNodeOnly === true) {
            // No Accels or Accels on Node Scope
            startFetching(
                job,
                [...toFetch],
                job.numNodes > 2
                    ? ["node"]
                    : ["node", "socket", "core"]
            );
        } else {
            // Accels and not on node scope
            startFetching(
                job,
                [...toFetch],
                job.numNodes > 2
                    ? ["node", "accelerator"]
                    : ["node", "accelerator", "socket", "core"]
            );
        }

        isFetched = toFetch;
    });

    const lazyFetchMoreMetrics = () => {
        let notYetFetched = new Set();
        for (let m of selectedMetrics) {
            if (!isFetched.has(m)) {
                notYetFetched.add(m);
                isFetched.add(m);
            }
        }

        if (notYetFetched.size > 0)
            startFetching(
                $initq.data.job,
                [...notYetFetched],
                $initq.data.job.numNodes > 2 ? ["node"] : ["node", "core"]
            );
    };

    // Fetch more data once required:
    $: if ($initq.data && $jobMetrics.data && selectedMetrics)
        lazyFetchMoreMetrics();

    let plots = {},
        jobTags,
        statsTable,
        jobFootprint;

    $: document.title = $initq.fetching
        ? "Loading..."
        : $initq.error
        ? "Error"
        : `Job ${$initq.data.job.jobId} - ClusterCockpit`;

    // Find out what metrics or hosts are missing:
    let missingMetrics = [],
        missingHosts = [],
        somethingMissing = false;
    $: if ($initq.data && $jobMetrics.data) {
        let job = $initq.data.job,
            metrics = $jobMetrics.data.jobMetrics,
            metricNames = clusters
                .find((c) => c.name == job.cluster)
                .metricConfig.map((mc) => mc.name);

        // Metric not found in JobMetrics && Metric not explicitly disabled: Was expected, but is Missing
        missingMetrics = metricNames.filter(
            (metric) =>
                !metrics.some((jm) => jm.name == metric) &&
                !checkMetricDisabled(
                    metric,
                    $initq.data.job.cluster,
                    $initq.data.job.subCluster
                )
        );
        missingHosts = job.resources
            .map(({ hostname }) => ({
                hostname: hostname,
                metrics: metricNames.filter(
                    (metric) =>
                        !metrics.some(
                            (jm) =>
                                jm.scope == "node" &&
                                jm.metric.series.some(
                                    (series) => series.hostname == hostname
                                )
                        )
                ),
            }))
            .filter(({ metrics }) => metrics.length > 0);
        somethingMissing = missingMetrics.length > 0 || missingHosts.length > 0;
    }

    const orderAndMap = (grouped, selectedMetrics) =>
        selectedMetrics.map((metric) => ({
            metric: metric,
            data: grouped.find((group) => group[0].name == metric),
            disabled: checkMetricDisabled(
                metric,
                $initq.data.job.cluster,
                $initq.data.job.subCluster
            ),
        }));
</script>

<Row>
    <Col>
        {#if $initq.error}
            <Card body color="danger">{$initq.error.message}</Card>
        {:else if $initq.data}
            <JobInfo job={$initq.data.job} {jobTags} />
        {:else}
            <Spinner secondary />
        {/if}
    </Col>
    {#if $jobMetrics.data}
        {#key $jobMetrics.data}
            <Col>
                <JobFootprint
                    bind:this={jobFootprint}
                    job={$initq.data.job}
                    jobMetrics={$jobMetrics.data.jobMetrics}
                />
            </Col>
        {/key}
    {/if}
    {#if $jobMetrics.data && $initq.data}
        {#if $initq.data.job.concurrentJobs != null && $initq.data.job.concurrentJobs.items.length != 0}
            {#if authlevel > roles.manager}
                <Col>
                    <h5>
                        Concurrent Jobs <Icon
                            name="info-circle"
                            style="cursor:help;"
                            title="Shared jobs running on the same node with overlapping runtimes"
                        />
                    </h5>
                    <ul>
                        <li>
                            <a
                                href="/monitoring/jobs/?{$initq.data.job
                                    .concurrentJobs.listQuery}"
                                target="_blank">See All</a
                            >
                        </li>
                        {#each $initq.data.job.concurrentJobs.items as pjob, index}
                            <li>
                                <a
                                    href="/monitoring/job/{pjob.id}"
                                    target="_blank">{pjob.jobId}</a
                                >
                            </li>
                        {/each}
                    </ul>
                </Col>
            {:else}
                <Col>
                    <h5>
                        {$initq.data.job.concurrentJobs.items.length} Concurrent
                        Jobs
                    </h5>
                    <p>
                        Number of shared jobs on the same node with overlapping
                        runtimes.
                    </p>
                </Col>
            {/if}
        {/if}
        <Col>
            <Polar
                metrics={ccconfig[
                    `job_view_polarPlotMetrics:${$initq.data.job.cluster}`
                ] || ccconfig[`job_view_polarPlotMetrics`]}
                cluster={$initq.data.job.cluster}
                jobMetrics={$jobMetrics.data.jobMetrics}
            />
        </Col>
        <Col>
            <Roofline
                renderTime={true}
                cluster={clusters
                    .find((c) => c.name == $initq.data.job.cluster)
                    .subClusters.find(
                        (sc) => sc.name == $initq.data.job.subCluster
                    )}
                data={
                    transformDataForRoofline (
                        $jobMetrics.data.jobMetrics.find((m) => m.name == "flops_any" && m.scope == "node").metric,
                        $jobMetrics.data.jobMetrics.find((m) => m.name == "mem_bw" && m.scope == "node").metric
                    )
                }
            />
        </Col>
    {:else}
        <Col />
        <Col />
    {/if}
</Row>
<Row class="mb-3">
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
<!--     <Col xs="auto">
        <Zoom timeseriesPlots={plots} />
    </Col> -->
</Row>
<Row>
    <Col>
        {#if $jobMetrics.error}
            {#if $initq.data.job.monitoringStatus == 0 || $initq.data.job.monitoringStatus == 2}
                <Card body color="warning"
                    >Not monitored or archiving failed</Card
                >
                <br />
            {/if}
            <Card body color="danger">{$jobMetrics.error.message}</Card>
        {:else if $jobMetrics.fetching}
            <Spinner secondary />
        {:else if $jobMetrics.data && $initq.data}
            <PlotTable
                let:item
                let:width
                renderFor="job"
                items={orderAndMap(
                    groupByScope($jobMetrics.data.jobMetrics),
                    selectedMetrics
                )}
                itemsPerRow={ccconfig.plot_view_plotsPerRow}
            >
                {#if item.data}
                    <Metric
                        bind:this={plots[item.metric]}
                        on:more-loaded={({ detail }) =>
                            statsTable.moreLoaded(detail)}
                        job={$initq.data.job}
                        metricName={item.metric}
                        rawData={item.data.map((x) => x.metric)}
                        scopes={item.data.map((x) => x.scope)}
                        {width}
                        isShared={$initq.data.job.exclusive != 1}
                        resources={$initq.data.job.resources}
                    />
                {:else}
                    <Card body color="warning"
                        >No dataset returned for <code>{item.metric}</code
                        ></Card
                    >
                {/if}
            </PlotTable>
        {/if}
    </Col>
</Row>
<Row class="mt-2">
    <Col>
        {#if $initq.data}
            <TabContent>
                {#if somethingMissing}
                    <TabPane
                        tabId="resources"
                        tab="Resources"
                        active={somethingMissing}
                    >
                        <div style="margin: 10px;">
                            <Card color="warning">
                                <CardHeader>
                                    <CardTitle
                                        >Missing Metrics/Reseources</CardTitle
                                    >
                                </CardHeader>
                                <CardBody>
                                    {#if missingMetrics.length > 0}
                                        <p>
                                            No data at all is available for the
                                            metrics: {missingMetrics.join(", ")}
                                        </p>
                                    {/if}
                                    {#if missingHosts.length > 0}
                                        <p>
                                            Some metrics are missing for the
                                            following hosts:
                                        </p>
                                        <ul>
                                            {#each missingHosts as missing}
                                                <li>
                                                    {missing.hostname}: {missing.metrics.join(
                                                        ", "
                                                    )}
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
                    {#if $jobMetrics.data}
                        {#key $jobMetrics.data}
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
                            <pre><code
                                    >{$initq.data.job.metaData?.jobScript}</code
                                ></pre>
                        {:else}
                            <Card body color="warning"
                                >No job script available</Card
                            >
                        {/if}
                    </div>
                </TabPane>
                <TabPane tabId="slurm-info" tab="Slurm Info">
                    <div class="pre-wrapper">
                        {#if $initq.data.job.metaData?.slurmInfo}
                            <pre><code
                                    >{$initq.data.job.metaData?.slurmInfo}</code
                                ></pre>
                        {:else}
                            <Card body color="warning"
                                >No additional slurm information available</Card
                            >
                        {/if}
                    </div>
                </TabPane>
            </TabContent>
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
