<script>
    import { onMount, getContext } from 'svelte'
    import { init, convert2uplot } from './utils.js'
    import { Table, Row, Col, Button, Icon, Card, Spinner } from 'sveltestrap'
    import { queryStore, gql, getContextClient } from '@urql/svelte'
    import Filters from './filters/Filters.svelte'
    import JobList from './joblist/JobList.svelte'
    import Sorting from './joblist/SortSelection.svelte'
    import Refresher from './joblist/Refresher.svelte'
    import Histogram from './plots/Histogram.svelte'
    import MetricSelection from './MetricSelection.svelte'
    import HistogramSelection from './HistogramSelection.svelte'
    import PlotTable from './PlotTable.svelte'
    import { scramble, scrambleNames } from './joblist/JobInfo.svelte'

    const { query: initq } = init()

    const ccconfig = getContext('cc-config')

    export let user
    export let filterPresets

    let filterComponent; // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
    let jobList;
    let jobFilters = [];
    let sorting = { field: 'startTime', order: 'DESC' }, isSortingOpen = false
    let metrics = ccconfig.plot_list_selectedMetrics, isMetricsSelectionOpen = false
    let w1, w2, histogramHeight = 250, isHistogramSelectionOpen = false
    let selectedCluster = filterPresets?.cluster ? filterPresets.cluster : null
    let showFootprint = filterPresets.cluster
        ? !!ccconfig[`plot_list_showFootprint:${filterPresets.cluster}`]
        : !!ccconfig.plot_list_showFootprint

    $: metricsInHistograms = selectedCluster ? (ccconfig[`user_view_histogramMetrics:${selectedCluster}`] || []) : (ccconfig.user_view_histogramMetrics || [])

    const client = getContextClient();
    $: stats = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!, $metricsInHistograms: [String!]) {
            jobsStatistics(filter: $jobFilters, metrics: $metricsInHistograms) {
                totalJobs
                shortJobs
                totalWalltime
                totalCoreHours
                histDuration { count, value }
                histNumNodes { count, value }
                histMetrics  { metric, unit, data { min, max, count, bin } }
            }}`,
        variables: { jobFilters, metricsInHistograms }
    })

    onMount(() => filterComponent.update())
</script>

<Row>
    {#if $initq.fetching}
        <Col>
            <Spinner/>
        </Col>
    {:else if $initq.error}
        <Col xs="auto">
            <Card body color="danger">{$initq.error.message}</Card>
        </Col>
    {/if}

    <Col xs="auto">
        <Button
            outline color="primary"
            on:click={() => (isSortingOpen = true)}>
            <Icon name="sort-up"/> Sorting
        </Button>

        <Button
            outline color="primary"
            on:click={() => (isMetricsSelectionOpen = true)}>
            <Icon name="graph-up"/> Metrics
        </Button>

        <Button
            outline color="secondary"
            on:click={() => (isHistogramSelectionOpen = true)}>
            <Icon name="bar-chart-line"/> Select Histograms
        </Button>
    </Col>
    <Col xs="auto">
        <Filters
            filterPresets={filterPresets}
            startTimeQuickSelect={true}
            bind:this={filterComponent}
            on:update={({ detail }) => {
                jobFilters = [...detail.filters, { user: { eq: user.username } }]
                selectedCluster = jobFilters[0]?.cluster ? jobFilters[0].cluster.eq : null 
                jobList.update(jobFilters)
            }} />
    </Col>
    <Col xs="auto" style="margin-left: auto;">
        <Refresher on:reload={() => jobList.refresh()} />
    </Col>
</Row>
<br/>
<Row>
    {#if $stats.error}
        <Col>
            <Card body color="danger">{$stats.error.message}</Card>
        </Col>
    {:else if !$stats.data}
        <Col>
            <Spinner secondary />
        </Col>
    {:else}
        <Col xs="4">
            <Table>
                <tbody>
                    <tr>
                        <th scope="row">Username</th>
                        <td>{scrambleNames ? scramble(user.username) : user.username}</td>
                    </tr>
                    {#if user.name}
                        <tr>
                            <th scope="row">Name</th>
                            <td>{scrambleNames ? scramble(user.name) : user.name}</td>
                        </tr>
                    {/if}
                    {#if user.email}
                        <tr>
                            <th scope="row">Email</th>
                            <td>{user.email}</td>
                        </tr>
                    {/if}
                    <tr>
                        <th scope="row">Total Jobs</th>
                        <td>{$stats.data.jobsStatistics[0].totalJobs}</td>
                    </tr>
                    <tr>
                        <th scope="row">Short Jobs</th>
                        <td>{$stats.data.jobsStatistics[0].shortJobs}</td>
                    </tr>
                    <tr>
                        <th scope="row">Total Walltime</th>
                        <td>{$stats.data.jobsStatistics[0].totalWalltime}</td>
                    </tr>
                    <tr>
                        <th scope="row">Total Core Hours</th>
                        <td>{$stats.data.jobsStatistics[0].totalCoreHours}</td>
                    </tr>
                </tbody>
            </Table>
        </Col>
        <div class="col-4 text-center" bind:clientWidth={w1}>
            {#key $stats.data.jobsStatistics[0].histDuration}
                <Histogram
                    data={convert2uplot($stats.data.jobsStatistics[0].histDuration)}
                    width={w1 - 25} height={histogramHeight}
                    title="Duration Distribution"
                    xlabel="Current Runtimes"
                    xunit="Hours" 
                    ylabel="Number of Jobs"
                    yunit="Jobs"/>
            {/key}
        </div>
        <div class="col-4 text-center" bind:clientWidth={w2}>
            {#key $stats.data.jobsStatistics[0].histNumNodes}
                <Histogram
                    data={convert2uplot($stats.data.jobsStatistics[0].histNumNodes)}
                    width={w2 - 25} height={histogramHeight}
                    title="Number of Nodes Distribution"
                    xlabel="Allocated Nodes"
                    xunit="Nodes"
                    ylabel="Number of Jobs"
                    yunit="Jobs"/>
            {/key}
        </div>
    {/if}
</Row>
{#if metricsInHistograms}
    <Row>
        {#if $stats.error}
            <Col>
                <Card body color="danger">{$stats.error.message}</Card>
            </Col>
        {:else if !$stats.data}
            <Col>
                <Spinner secondary />
            </Col>
        {:else}
            <Col>
                {#key $stats.data.jobsStatistics[0].histMetrics}
                    <PlotTable
                        let:item
                        let:width
                        renderFor="user"
                        items={$stats.data.jobsStatistics[0].histMetrics}
                        itemsPerRow={3}>

                        <Histogram
                            data={convert2uplot(item.data)}
                            usesBins={true}
                            width={width} height={250}
                            title="Distribution of '{item.metric}' averages"
                            xlabel={`${item.metric} bin maximum ${item?.unit ? `[${item.unit}]` : ``}`}
                            xunit={item.unit}
                            ylabel="Number of Jobs"
                            yunit="Jobs"/>
                    </PlotTable>
                {/key}
            </Col>
        {/if}
    </Row>
{/if}
<br/>
<Row>
    <Col>
        <JobList
            bind:metrics={metrics}
            bind:sorting={sorting}
            bind:this={jobList}
            bind:showFootprint={showFootprint} />
    </Col>
</Row>

<Sorting
    bind:sorting={sorting}
    bind:isOpen={isSortingOpen} />

<MetricSelection 
    bind:cluster={selectedCluster}
    configName="plot_list_selectedMetrics"
    bind:metrics={metrics}
    bind:isOpen={isMetricsSelectionOpen}
    bind:showFootprint={showFootprint}
    view='list'/>
      
<HistogramSelection
    bind:cluster={selectedCluster}
    bind:metricsInHistograms={metricsInHistograms}
    bind:isOpen={isHistogramSelectionOpen} />
