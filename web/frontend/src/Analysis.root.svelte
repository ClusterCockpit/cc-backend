<script>
    import { init } from './utils.js'
    import { getContext, onMount } from 'svelte'
    import { queryStore, gql, getContextClient  } from '@urql/svelte'
    import { Row, Col, Spinner, Card, Table } from 'sveltestrap'
    import Filters from './filters/Filters.svelte'
    import PlotSelection from './PlotSelection.svelte'
    import Histogram, { binsFromFootprint } from './plots/Histogram.svelte'
    import ScatterPlot from './plots/Scatter.svelte'
    import PlotTable from './PlotTable.svelte'
    import Roofline from './plots/Roofline.svelte'

    const { query: initq } = init()

    export let filterPresets

    // By default, look at the jobs of the last 6 hours:
    if (filterPresets?.startTime == null) {
        if (filterPresets == null)
            filterPresets = {}

        let now = new Date(Date.now())
        let hourAgo = new Date(now)
        hourAgo.setHours(hourAgo.getHours() - 6)
        filterPresets.startTime = { from: hourAgo.toISOString(), to: now.toISOString() }
    }

    let cluster;
    let filterComponent; // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
    let jobFilters = [];
    let rooflineMaxY;
    let colWidth;
    let numBins = 50;
    let maxY = -1;
    const ccconfig = getContext('cc-config')
    const metricConfig = getContext('metrics')

    let metricsInHistograms = ccconfig.analysis_view_histogramMetrics,
        metricsInScatterplots = ccconfig.analysis_view_scatterPlotMetrics

    $: metrics = [...new Set([...metricsInHistograms, ...metricsInScatterplots.flat()])]

    getContext('on-init')(({ data }) => {
        if (data != null) {
            cluster = data.clusters.find(c => c.name == filterPresets.cluster)
            console.assert(cluster != null, `This cluster could not be found: ${filterPresets.cluster}`)

            rooflineMaxY = cluster.subClusters.reduce((max, part) => Math.max(max, part.flopRateSimd.value), 0)
            maxY = rooflineMaxY
        }
    })

    const client = getContextClient();

    $: statsQuery = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!) {
                stats: jobsStatistics(filter: $jobFilters) {
                    totalJobs
                    shortJobs
                    totalWalltime
                    totalCoreHours
                    histDuration { count, value }
                    histNumNodes { count, value }
                }

                topUsers: jobsCount(filter: $jobFilters, groupBy: USER, weight: NODE_HOURS, limit: 5) { name, count }
            }
        `, 
        variables: { jobFilters }
    })

    $: footprintsQuery = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!, $metrics: [String!]!) {
                footprints: jobsFootprints(filter: $jobFilters, metrics: $metrics) {
                    nodehours,
                    metrics { metric, data }
                }
            }`,
        variables:  { jobFilters, metrics }
    })

    $: rooflineQuery = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!, $rows: Int!, $cols: Int!,
                    $minX: Float!, $minY: Float!, $maxX: Float!, $maxY: Float!) {
                rooflineHeatmap(filter: $jobFilters, rows: $rows, cols: $cols,
                        minX: $minX, minY: $minY, maxX: $maxX, maxY: $maxY)
            }
        `,
        variables: { jobFilters, rows: 50, cols: 50, minX: 0.01, minY: 1., maxX: 1000., maxY }
    })

    onMount(() => filterComponent.update())
</script>

<Row>
    {#if $initq.fetching || $statsQuery.fetching || $footprintsQuery.fetching}
        <Col xs="auto">
            <Spinner />
        </Col>
    {/if}
    <Col xs="auto">
        {#if $initq.error}
            <Card body color="danger">{$initq.error.message}</Card>
        {:else if cluster}
            <PlotSelection
                availableMetrics={cluster.metricConfig.map(mc => mc.name)}
                bind:metricsInHistograms={metricsInHistograms}
                bind:metricsInScatterplots={metricsInScatterplots} />
        {/if}
    </Col>
    <Col xs="auto">
        <Filters
            bind:this={filterComponent}
            filterPresets={filterPresets}
            disableClusterSelection={true}
            startTimeQuickSelect={true}
            on:update={({ detail }) => {
                jobFilters = detail.filters;
            }} />
    </Col>
</Row>

<br/>
{#if $statsQuery.error}
    <Row>
        <Col>
            <Card body color="danger">{$statsQuery.error.message}</Card>
        </Col>
    </Row>
{:else if $statsQuery.data}
    <Row>
        <div class="col-3" bind:clientWidth={colWidth}>
            <div style="height: 40%">
                <Table>
                    <tr>
                        <th scope="col">Total Jobs</th>
                        <td>{$statsQuery.data.stats[0].totalJobs}</td>
                    </tr>
                    <tr>
                        <th scope="col">Short Jobs</th>
                        <td>{$statsQuery.data.stats[0].shortJobs}</td>
                    </tr>
                    <tr>
                        <th scope="col">Total Walltime</th>
                        <td>{$statsQuery.data.stats[0].totalWalltime}</td>
                    </tr>
                    <tr>
                        <th scope="col">Total Core Hours</th>
                        <td>{$statsQuery.data.stats[0].totalCoreHours}</td>
                    </tr>
                </Table>
            </div>
            <div style="height: 60%;">
                {#key $statsQuery.data.topUsers}
                    <h4>Top Users (by node hours)</h4>
                    <Histogram
                        width={colWidth - 25} height={300 * 0.5} small={true}
                        data={$statsQuery.data.topUsers.sort((a, b) => b.count - a.count).map(({ count }, idx) => ({ count, value: idx }))}
                        label={(x) => x < $statsQuery.data.topUsers.length ? $statsQuery.data.topUsers[Math.floor(x)].name : 'No Users'} 
                        ylabel="Node Hours [h]"/>
                {/key}
            </div>
        </div>
        <div class="col-3">
            {#key $statsQuery.data.stats[0].histDuration}
                <h4>Duration Distribution</h4>
                <Histogram
                    width={colWidth - 25}
                    data={$statsQuery.data.stats[0].histDuration} 
                    xlabel="Current Runtimes [h]" 
                    ylabel="Number of Jobs"/>
            {/key}
        </div>
        <div class="col-3">
            {#key $statsQuery.data.stats[0].histNumNodes}
                <h4>Number of Nodes Distribution</h4>
                <Histogram
                    width={colWidth - 25}
                    data={$statsQuery.data.stats[0].histNumNodes} 
                    xlabel="Allocated Nodes [#]"
                    ylabel="Number of Jobs" />
            {/key}
        </div>
        <div class="col-3">
            {#if $rooflineQuery.fetching}
                <Spinner />
            {:else if $rooflineQuery.error}
                <Card body color="danger">{$rooflineQuery.error.message}</Card>
            {:else if $rooflineQuery.data && cluster}
                {#key $rooflineQuery.data}
                    <Roofline
                        width={colWidth - 25}
                        tiles={$rooflineQuery.data.rooflineHeatmap}
                        cluster={cluster.subClusters.length == 1 ? cluster.subClusters[0] : null}
                        maxY={rooflineMaxY} />
                {/key}
            {/if}
        </div>
    </Row>
{/if}

<br/>
{#if $footprintsQuery.error}
    <Row>
        <Col>
            <Card body color="danger">{$footprintsQuery.error.message}</Card>
        </Col>
    </Row>
{:else if $footprintsQuery.data && $initq.data}
    <Row>
        <Col>
            <Card body>
                These histograms show the distribution of the averages of all jobs matching the filters. Each job/average is weighted by its node hours.
                Note that some metrics could be disabled for specific subclusters as per metriConfig and thus could affect shown average values.
            </Card>
            <br/>
        </Col>
    </Row>
    <Row>
        <Col>
            <PlotTable
                let:item
                let:width
                renderFor="analysis"
                items={metricsInHistograms.map(metric => ({ metric, ...binsFromFootprint(
                    $footprintsQuery.data.footprints.nodehours,
                    $footprintsQuery.data.footprints.metrics.find(f => f.metric == metric).data, numBins) }))}
                itemsPerRow={ccconfig.plot_view_plotsPerRow}>

                <h4>Average Distribution of '{item.metric}'</h4>
                <Histogram
                    width={width} height={250}
                    min={item.min} max={item.max}
                    data={item.bins} 
                    label={item.label}
                    xlabel={`${item.metric} Average [${(metricConfig(cluster.name, item.metric)?.unit?.prefix ? metricConfig(cluster.name, item.metric)?.unit?.prefix : '') +
                                                       (metricConfig(cluster.name, item.metric)?.unit?.base   ? metricConfig(cluster.name, item.metric)?.unit?.base   : '')}]`}
                    ylabel="Node Hours [h]" />
            </PlotTable>
        </Col>
    </Row>
    <br/>
    <Row>
        <Col>
            <Card body>
                Each circle represents one job. The size of a circle is proportional to its node hours. Darker circles mean multiple jobs have the same averages for the respective metrics.
                Note that some metrics could be disabled for specific subclusters as per metriConfig and thus could affect shown average values.
            </Card>
            <br/>
        </Col>
    </Row>
    <Row>
        <Col>
            <PlotTable
                let:item
                let:width
                items={metricsInScatterplots.map(([m1, m2]) => ({
                    m1, f1: $footprintsQuery.data.footprints.metrics.find(f => f.metric == m1).data,
                    m2, f2: $footprintsQuery.data.footprints.metrics.find(f => f.metric == m2).data }))}
                itemsPerRow={ccconfig.plot_view_plotsPerRow}>

                <ScatterPlot
                    width={width} height={250} color={"rgba(0, 102, 204, 0.33)"}
                    xLabel={`${item.m1} [${(metricConfig(cluster.name, item.m1)?.unit?.prefix ? metricConfig(cluster.name, item.m1)?.unit?.prefix : '') + 
                                           (metricConfig(cluster.name, item.m1)?.unit?.base   ? metricConfig(cluster.name, item.m1)?.unit?.base   : '')}]`}
                    yLabel={`${item.m2} [${(metricConfig(cluster.name, item.m2)?.unit?.prefix ? metricConfig(cluster.name, item.m2)?.unit?.prefix : '') + 
                                           (metricConfig(cluster.name, item.m2)?.unit?.base   ? metricConfig(cluster.name, item.m2)?.unit?.base   : '')}]`}
                    X={item.f1} Y={item.f2} S={$footprintsQuery.data.footprints.nodehours} />
            </PlotTable>
        </Col>
    </Row>
{/if}

<style>
    h4 {
        text-align: center;
    }
</style>
