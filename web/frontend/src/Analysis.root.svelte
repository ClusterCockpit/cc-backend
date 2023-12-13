<script>
    import { init, convert2uplot } from './utils.js'
    import { getContext, onMount } from 'svelte'
    import { queryStore, gql, getContextClient, mutationStore } from '@urql/svelte'
    import { Row, Col, Spinner, Card, Table, Icon } from 'sveltestrap'
    import Filters from './filters/Filters.svelte'
    import PlotSelection from './PlotSelection.svelte'
    import Histogram from './plots/Histogram.svelte'
    import Pie, { colors } from './plots/Pie.svelte'
    import { binsFromFootprint } from './utils.js'
    import ScatterPlot from './plots/Scatter.svelte'
    import PlotTable from './PlotTable.svelte'
    import RooflineHeatmap from './plots/RooflineHeatmap.svelte'

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
    let colWidth1, colWidth2, colWidth3, colWidth4;
    let numBins = 50;
    let maxY = -1;
    const ccconfig = getContext('cc-config')
    const metricConfig = getContext('metrics')

    let metricsInHistograms = ccconfig.analysis_view_histogramMetrics,
        metricsInScatterplots = ccconfig.analysis_view_scatterPlotMetrics

    $: metrics = [...new Set([...metricsInHistograms, ...metricsInScatterplots.flat()])]

    const sortOptions = [
        {key: 'totalWalltime',  label: 'Walltime'},
        {key: 'totalNodeHours', label: 'Node Hours'},
        {key: 'totalCoreHours', label: 'Core Hours'},
        {key: 'totalAccHours',  label: 'Accelerator Hours'}
    ]
    const groupOptions = [
        {key: 'user',  label: 'User Name'},
        {key: 'project', label: 'Project ID'}
    ]

    let sortSelection = sortOptions.find((option) => option.key == ccconfig[`analysis_view_selectedTopCategory:${filterPresets.cluster}`]) || sortOptions.find((option) => option.key == ccconfig.analysis_view_selectedTopCategory)
    let groupSelection = groupOptions.find((option) => option.key == ccconfig[`analysis_view_selectedTopEntity:${filterPresets.cluster}`]) || groupOptions.find((option) => option.key == ccconfig.analysis_view_selectedTopEntity)

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
                    totalNodeHours
                    totalCoreHours
                    totalAccHours
                    histDuration { count, value }
                    histNumCores { count, value }
                }
            }
        `, 
        variables: { jobFilters }
    })

    $: topQuery = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!, $paging: PageRequest!, $sortBy: SortByAggregate!, $groupBy: Aggregate!) {
                topList: jobsStatistics(filter: $jobFilters, page: $paging, sortBy: $sortBy, groupBy: $groupBy) {
                    id
                    totalWalltime
                    totalNodeHours
                    totalCoreHours
                    totalAccHours
                }
            }
        `, 
        variables: { jobFilters, paging: { itemsPerPage: 10, page: 1 }, sortBy: sortSelection.key.toUpperCase(), groupBy: groupSelection.key.toUpperCase() }
    })

    $: footprintsQuery = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!, $metrics: [String!]!) {
                footprints: jobsFootprints(filter: $jobFilters, metrics: $metrics) {
                    timeWeights { nodeHours, accHours, coreHours },
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

    const updateConfigurationMutation = ({ name, value }) => {
        return mutationStore({
            client: client,
            query: gql`
                mutation ($name: String!, $value: String!) {
                    updateConfiguration(name: $name, value: $value)
                }
            `,
            variables: { name, value }
        });
    }

    function updateEntityConfiguration(select) {
        if (ccconfig[`analysis_view_selectedTopEntity:${filterPresets.cluster}`] != select) {
            updateConfigurationMutation({ name: `analysis_view_selectedTopEntity:${filterPresets.cluster}`, value: JSON.stringify(select) })
            .subscribe(res => {
                if (res.fetching === false && !res.error) {
                    // console.log(`analysis_view_selectedTopEntity:${filterPresets.cluster}` + ' -> Updated!')
                } else if (res.fetching === false && res.error) {
                    throw res.error
                }
            })
        } else {
            // console.log('No Mutation Required: Entity')
        }
    };


    function updateCategoryConfiguration(select) {
        if (ccconfig[`analysis_view_selectedTopCategory:${filterPresets.cluster}`] != select) {
            updateConfigurationMutation({ name: `analysis_view_selectedTopCategory:${filterPresets.cluster}`, value: JSON.stringify(select) })
            .subscribe(res => {
                if (res.fetching === false && !res.error) {
                    // console.log(`analysis_view_selectedTopCategory:${filterPresets.cluster}` + ' -> Updated!')
                } else if (res.fetching === false && res.error) {
                    throw res.error
                }
            })
        } else {
            // console.log('No Mutation Required: Category')
        }

    };

    $: updateEntityConfiguration(groupSelection.key)
    $: updateCategoryConfiguration(sortSelection.key)

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
    <Row cols={3} class="mb-4">
        <Col>
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
                    <th scope="col">Total Node Hours</th>
                    <td>{$statsQuery.data.stats[0].totalNodeHours}</td>
                </tr>
                <tr>
                    <th scope="col">Total Core Hours</th>
                    <td>{$statsQuery.data.stats[0].totalCoreHours}</td>
                </tr>
                <tr>
                    <th scope="col">Total Accelerator Hours</th>
                    <td>{$statsQuery.data.stats[0].totalAccHours}</td>
                </tr>
            </Table>
        </Col>
        <Col>
            <div bind:clientWidth={colWidth1}>
            <h5>Top 
                <select class="p-0" bind:value={groupSelection}>
                    {#each groupOptions as option}
                        <option value={option}>
                            {option.key.charAt(0).toUpperCase() + option.key.slice(1)}s
                        </option>
                    {/each}
                </select>
            </h5>
            {#key $topQuery.data}
                {#if $topQuery.fetching}
                    <Spinner/>
                {:else if $topQuery.error}
                    <Card body color="danger">{$topQuery.error.message}</Card>
                {:else}                    
                    <Pie
                        size={colWidth1}
                        sliceLabel={sortSelection.label}
                        quantities={$topQuery.data.topList.map((t) => t[sortSelection.key])}
                        entities={$topQuery.data.topList.map((t) => t.id)}
                    />
                {/if}
            {/key}
            </div>
        </Col>
        <Col>
            {#key $topQuery.data}
                {#if $topQuery.fetching}
                    <Spinner/>
                {:else if $topQuery.error}
                    <Card body color="danger">{$topQuery.error.message}</Card>
                {:else}
                    <Table>
                        <tr class="mb-2">
                            <th>Legend</th>
                            <th>{groupSelection.label}</th>
                            <th>
                                <select class="p-0" bind:value={sortSelection}>
                                    {#each sortOptions as option}
                                        <option value={option}>
                                            {option.label}
                                        </option>
                                    {/each}
                                </select>
                            </th>
                        </tr>
                        {#each $topQuery.data.topList as te, i}
                            <tr>
                                <td><Icon name="circle-fill" style="color: {colors[i]};"/></td>
                                {#if groupSelection.key == 'user'} 
                                    <th scope="col"><a href="/monitoring/user/{te.id}?cluster={cluster.name}">{te.id}</a></th>
                                {:else}
                                    <th scope="col"><a href="/monitoring/jobs/?cluster={cluster.name}&project={te.id}&projectMatch=eq">{te.id}</a></th>
                                {/if}
                                <td>{te[sortSelection.key]}</td>
                            </tr>
                        {/each}
                    </Table>
                {/if}
            {/key}
        </Col>
    </Row>
    <Row cols={3} class="mb-2">
        <Col>
            {#if $rooflineQuery.fetching}
                <Spinner />
            {:else if $rooflineQuery.error}
                <Card body color="danger">{$rooflineQuery.error.message}</Card>
            {:else if $rooflineQuery.data && cluster}
                <div bind:clientWidth={colWidth2}>
                {#key $rooflineQuery.data}
                    <RooflineHeatmap
                        width={colWidth2} height={300}
                        tiles={$rooflineQuery.data.rooflineHeatmap}
                        cluster={cluster.subClusters.length == 1 ? cluster.subClusters[0] : null}
                        maxY={rooflineMaxY} />
                {/key}
                </div>
            {/if}
        </Col>
        <Col>
            <div bind:clientWidth={colWidth3}>
            {#key $statsQuery.data.stats[0].histDuration}
                <Histogram
                    width={colWidth3} height={300}
                    data={convert2uplot($statsQuery.data.stats[0].histDuration)}
                    title="Duration Distribution"
                    xlabel="Current Runtimes"
                    xunit="Hours" 
                    ylabel="Number of Jobs"
                    yunit="Jobs"/>
            {/key}
            </div>
        </Col>
        <Col>
            <div bind:clientWidth={colWidth4}>
            {#key $statsQuery.data.stats[0].histNumCores}
                <Histogram
                    width={colWidth4} height={300}
                    data={convert2uplot($statsQuery.data.stats[0].histNumCores)}
                    title="Number of Cores Distribution"
                    xlabel="Allocated Cores"
                    xunit="Cores"
                    ylabel="Number of Jobs"
                    yunit="Jobs"/>
            {/key}
            </div>
        </Col>
    </Row>
{/if}

<hr class="my-6"/>

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
                These histograms show the distribution of the averages of all jobs matching the filters. Each job/average is weighted by its node hours by default 
                (Accelerator hours for native accelerator scope metrics, coreHours for native core scope metrics).
                Note that some metrics could be disabled for specific subclusters as per metricConfig and thus could affect shown average values.
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
                    $footprintsQuery.data.footprints.timeWeights,
                    metricConfig(cluster.name, metric)?.scope,
                    $footprintsQuery.data.footprints.metrics.find(f => f.metric == metric).data, numBins) }))}
                itemsPerRow={ccconfig.plot_view_plotsPerRow}>

                <Histogram
                    data={convert2uplot(item.bins)}
                    width={width} height={250}
                    usesBins={true}
                    title="Average Distribution of '{item.metric}'"
                    xlabel={`${item.metric} bin maximum ${(metricConfig(cluster.name, item.metric)?.unit?.prefix ? '[' + metricConfig(cluster.name, item.metric)?.unit?.prefix : '') +
                                                          (metricConfig(cluster.name, item.metric)?.unit?.base   ? metricConfig(cluster.name, item.metric)?.unit?.base + ']'   : '')}`}
                    xunit={`${(metricConfig(cluster.name, item.metric)?.unit?.prefix ? metricConfig(cluster.name, item.metric)?.unit?.prefix : '') +
                              (metricConfig(cluster.name, item.metric)?.unit?.base   ? metricConfig(cluster.name, item.metric)?.unit?.base   : '')}`}
                    ylabel="Normalized Hours"
                    yunit="Hours"/>
            </PlotTable>
        </Col>
    </Row>
    <br/>
    <Row>
        <Col>
            <Card body>
                Each circle represents one job. The size of a circle is proportional to its node hours. Darker circles mean multiple jobs have the same averages for the respective metrics.
                Note that some metrics could be disabled for specific subclusters as per metricConfig and thus could affect shown average values.
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
                    X={item.f1} Y={item.f2} S={$footprintsQuery.data.footprints.timeWeights.nodeHours} />
            </PlotTable>
        </Col>
    </Row>
{/if}

<style>
    h5 {
        text-align: center;
    }
</style>
