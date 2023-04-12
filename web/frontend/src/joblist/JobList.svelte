<!--
    @component

    Properties:
    - metrics:     [String] (can change from outside)
    - sorting:     { field: String, order: "DESC" | "ASC" } (can change from outside)
    - matchedJobs: Number (changes from inside)
    Functions:
    - update(filters?: [JobFilter])
 -->
<script>
    import { operationStore, query, mutation } from '@urql/svelte'
    import { getContext } from 'svelte';
    import { Row, Table, Card, Spinner } from 'sveltestrap'
    import Pagination from './Pagination.svelte'
    import JobListRow from './Row.svelte'
    import { stickyHeader } from '../utils.js'

    const ccconfig = getContext('cc-config'),
          clusters = getContext('clusters'),
          initialized = getContext('initialized')

    export let sorting = { field: "startTime", order: "DESC" }
    export let matchedJobs = 0
    export let metrics = ccconfig.plot_list_selectedMetrics

    let itemsPerPage = ccconfig.plot_list_jobsPerPage
    let page = 1
    let paging = { itemsPerPage, page }
    let filter = []

    const jobs = operationStore(`
    query($filter: [JobFilter!]!, $sorting: OrderByInput!, $paging: PageRequest! ){
        jobs(filter: $filter, order: $sorting, page: $paging) {
            items {
                id, jobId, user, project, jobName, cluster, subCluster, startTime,
                duration, numNodes, numHWThreads, numAcc, walltime, resources { hostname },
                SMT, exclusive, partition, arrayJobId,
                monitoringStatus, state,
                tags { id, type, name }
                userData { name }
                metaData
            }
            count
        }
    }`, {
        paging,
        sorting,
        filter,
    }, {
        pause: true
    })

    const updateConfiguration = mutation({
        query: `mutation($name: String!, $value: String!) {
            updateConfiguration(name: $name, value: $value)
        }`
    })

    $: $jobs.variables = { ...$jobs.variables, sorting, paging }
    $: matchedJobs = $jobs.data != null ? $jobs.data.jobs.count : 0

    // (Re-)query and optionally set new filters.
    export function update(filters) {
        if (filters != null) {
            let minRunningFor = ccconfig.plot_list_hideShortRunningJobs
            if (minRunningFor && minRunningFor > 0) {
                filters.push({ minRunningFor })
            }

            $jobs.variables.filter = filters
            // console.log('filters:', ...filters.map(f => Object.entries(f)).flat(2))
        }

        page = 1
        $jobs.variables.paging = paging = { page, itemsPerPage };
        $jobs.context.pause = false
        $jobs.reexecute({ requestPolicy: 'network-only' })
    }

    query(jobs)

    let tableWidth = null
    let jobInfoColumnWidth = 250
    $: plotWidth = Math.floor((tableWidth - jobInfoColumnWidth) / metrics.length - 10)

    let headerPaddingTop = 0
    stickyHeader('.cc-table-wrapper > table.table >thead > tr > th.position-sticky:nth-child(1)', (x) => (headerPaddingTop = x))
</script>

<Row>
    <div class="col cc-table-wrapper" bind:clientWidth={tableWidth}>
        <Table cellspacing="0px" cellpadding="0px">
            <thead>
                <tr>
                    <th class="position-sticky top-0" scope="col" style="width: {jobInfoColumnWidth}px; padding-top: {headerPaddingTop}px">
                        Job Info
                    </th>
                    {#each metrics as metric (metric)}
                        <th class="position-sticky top-0 text-center" scope="col" style="width: {plotWidth}px; padding-top: {headerPaddingTop}px">
                            {metric}
                            {#if $initialized}
                                ({clusters
                                    .map(cluster => cluster.metricConfig.find(m => m.name == metric))
                                    .filter(m => m != null)
                                    .map(m => (m.unit?.prefix?m.unit?.prefix:'') + (m.unit?.base?m.unit?.base:'')) // Build unitStr
                                    .reduce((arr, unitStr) => arr.includes(unitStr) ? arr : [...arr, unitStr], []) // w/o this, output would be [unitStr, unitStr]
                                    .join(', ')
                                })
                            {/if}
                        </th>
                    {/each}
                </tr>
            </thead>
            <tbody>
                {#if $jobs.error}
                    <tr>
                        <td colspan="{metrics.length + 1}">
                            <Card body color="danger" class="mb-3"><h2>{$jobs.error.message}</h2></Card>
                        </td>
                    </tr>
                {:else if $jobs.fetching || !$jobs.data}
                    <tr>
                        <td colspan="{metrics.length + 1}">
                            <Spinner secondary />
                        </td>
                    </tr>
                {:else if $jobs.data && $initialized}
                    {#each $jobs.data.jobs.items as job (job)}
                        <JobListRow
                            job={job}
                            metrics={metrics}
                            plotWidth={plotWidth} />
                    {:else}
                    <tr>
                        <td colspan="{metrics.length + 1}">
                            No jobs found
                        </td>
                    </tr>
                    {/each}
                {/if}
            </tbody>
        </Table>
    </div>
</Row>

<Pagination
    bind:page={page}
    {itemsPerPage}
    itemText="Jobs"
    totalItems={matchedJobs}
    on:update={({ detail }) => {
        if (detail.itemsPerPage != itemsPerPage) {
            itemsPerPage = detail.itemsPerPage
            updateConfiguration({
                name: "plot_list_jobsPerPage",
                value: itemsPerPage.toString()
            }).then(res => {
                if (res.error)
                    console.error(res.error);
            })
        }

        paging = { itemsPerPage: detail.itemsPerPage, page: detail.page }
    }} />

<style>
    .cc-table-wrapper {
        overflow: initial;
    }

    .cc-table-wrapper > :global(table) {
        border-collapse: separate;
        border-spacing: 0px;
        table-layout: fixed;
    }

    .cc-table-wrapper :global(button) {
        margin-bottom: 0px;
    }

    .cc-table-wrapper > :global(table > tbody > tr > td) {
        margin: 0px;
        padding-left: 5px;
        padding-right: 0px;
    }

    th.position-sticky.top-0 {
        background-color: white;
        z-index: 10;
        border-bottom: 1px solid black;
    }
</style>
