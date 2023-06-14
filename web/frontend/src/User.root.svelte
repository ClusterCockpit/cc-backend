<script>
    import { onMount, getContext } from 'svelte'
    import { init } from './utils.js'
    import { Table, Row, Col, Button, Icon, Card, Spinner, Input } from 'sveltestrap'
    import { queryStore, gql, getContextClient } from '@urql/svelte'
    import Filters from './filters/Filters.svelte'
    import JobList from './joblist/JobList.svelte'
    import Sorting from './joblist/SortSelection.svelte'
    import Refresher from './joblist/Refresher.svelte'
    import Histogram from './plots/Histogram.svelte'
    import MetricSelection from './MetricSelection.svelte'
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
    let w1, w2, histogramHeight = 250
    let selectedCluster = filterPresets?.cluster ? filterPresets.cluster : null
    let resize = false 
    /* Resize Context 
    *  A) Each viewport change triggers histogram rerender due to variable dimensions clearing canvas if not rerendered
    *  B) Opening filters (and some other things) triggers small change in viewport dimensions (Fix here?)
    *  A+B) Histogram rerenders if filters opened, high performance impact if dataload heavy
    *  Solution: Default to fixed histogram dimensions, allow user to enable automatic resizing
    */

    const client = getContextClient();
    $: stats = queryStore({
        client: client,
        query: gql`
            query($jobFilters: [JobFilter!]!) {
            jobsStatistics(filter: $jobFilters) {
                totalJobs
                shortJobs
                totalWalltime
                totalCoreHours
                histDuration { count, value }
                histNumNodes { count, value }
            }}`,
        variables: { jobFilters }
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
<!--                     <tr>
                        <th scope="row">Toggle Histogram Resizing</th>
                        <td><Input id="c3" value={resize} type="switch" on:change={() => (resize = !resize)}/></td>
                    </tr> -->
                </tbody>
            </Table>
        </Col>
        <div class="col-4" style="text-align: center;" bind:clientWidth={w1}>
            <b>Duration Distribution</b>
            {#key $stats.data.jobsStatistics[0].histDuration}
                {#if resize == true}
                <Histogram
                    data={$stats.data.jobsStatistics[0].histDuration}
                    width={w1 - 25} height={histogramHeight}
                    xlabel="Current Runtimes [h]" 
                    ylabel="Number of Jobs"/>
                {:else}
                <Histogram
                    data={$stats.data.jobsStatistics[0].histDuration}
                    width={400} height={250}
                    xlabel="Current Runtimes [h]" 
                    ylabel="Number of Jobs"/>
                {/if}
            {/key}
        </div>
        <div class="col-4" style="text-align: center;" bind:clientWidth={w2}>
            <b>Number of Nodes Distribution</b>
            {#key $stats.data.jobsStatistics[0].histNumNodes}
                {#if resize == true}
                <Histogram
                    data={$stats.data.jobsStatistics[0].histNumNodes}
                    width={w2 - 25} height={histogramHeight}
                    xlabel="Allocated Nodes [#]"
                    ylabel="Number of Jobs" />
                {:else}
                <Histogram
                    data={$stats.data.jobsStatistics[0].histNumNodes}
                    width={400} height={250}
                    xlabel="Allocated Nodes [#]"
                    ylabel="Number of Jobs" />
                {/if}
            {/key}
        </div>
    {/if}
</Row>
<br/>
<Row>
    <Col>
        <JobList
            bind:metrics={metrics}
            bind:sorting={sorting}
            bind:this={jobList} />
    </Col>
</Row>

<Sorting
    bind:sorting={sorting}
    bind:isOpen={isSortingOpen} />

<MetricSelection 
    bind:cluster={selectedCluster}
    configName="plot_list_selectedMetrics"
    bind:metrics={metrics}
    bind:isOpen={isMetricsSelectionOpen} />