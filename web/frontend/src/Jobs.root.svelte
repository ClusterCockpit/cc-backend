<script>
    import { onMount, getContext } from 'svelte'
    import { init } from './utils.js'
    import { Row, Col, Button, Icon, Card, Spinner } from 'sveltestrap'
    import Filters from './filters/Filters.svelte'
    import JobList from './joblist/JobList.svelte'
    import Refresher from './joblist/Refresher.svelte'
    import Sorting from './joblist/SortSelection.svelte'
    import MetricSelection from './MetricSelection.svelte'
    import UserOrProject from './filters/UserOrProject.svelte'

    const { query: initq } = init()

    const ccconfig = getContext('cc-config')

    export let filterPresets = {}
    export let authlevel
    export let roles

    let filterComponent; // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
    let jobList, matchedJobs = null
    let sorting = { field: 'startTime', order: 'DESC' }, isSortingOpen = false, isMetricsSelectionOpen = false
    let metrics = filterPresets.cluster
        ? ccconfig[`plot_list_selectedMetrics:${filterPresets.cluster}`] || ccconfig.plot_list_selectedMetrics
        : ccconfig.plot_list_selectedMetrics
    let showFootprint = filterPresets.cluster
        ? !!ccconfig[`plot_list_showFootprint:${filterPresets.cluster}`]
        : !!ccconfig.plot_list_showFootprint
    let selectedCluster = filterPresets?.cluster ? filterPresets.cluster : null
    
    // The filterPresets are handled by the Filters component,
    // so we need to wait for it to be ready before we can start a query.
    // This is also why JobList component starts out with a paused query.
    onMount(() => filterComponent.update())
</script>

<Row>
    {#if $initq.fetching}
        <Col xs="auto">
            <Spinner/>
        </Col>
    {:else if $initq.error}
        <Col xs="auto">
            <Card body color="danger">{$initq.error.message}</Card>
        </Col>
    {/if}
</Row>
<Row>
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
        <Button disabled outline>{matchedJobs == null ? 'Loading...' : `${matchedJobs} jobs`}</Button>
    </Col>
    <Col xs="auto">
        <Filters
            filterPresets={filterPresets}
            bind:this={filterComponent}
            on:update={({ detail }) => {
                selectedCluster = detail.filters[0]?.cluster ? detail.filters[0].cluster.eq : null 
                jobList.update(detail.filters)
            }
            } />
    </Col>

    <Col xs="3" style="margin-left: auto;">
        <UserOrProject bind:authlevel={authlevel} bind:roles={roles} on:update={({ detail }) => filterComponent.update(detail)}/>
    </Col>
    <Col xs="2">
        <Refresher on:reload={() => jobList.refresh()} />
    </Col>
</Row>
<br/>
<Row>
    <Col>
        <JobList
            bind:metrics={metrics}
            bind:sorting={sorting}
            bind:matchedJobs={matchedJobs}
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
