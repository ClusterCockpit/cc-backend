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
    import {
        queryStore,
        gql,
        getContextClient,
        mutationStore,
    } from "@urql/svelte";
    import { getContext } from "svelte";
    import { Row, Table, Card, Spinner } from "sveltestrap";
    import Pagination from "./Pagination.svelte";
    import JobListRow from "./Row.svelte";
    import { stickyHeader } from "../utils.js";

    const ccconfig = getContext("cc-config"),
        clusters = getContext("clusters"),
        initialized = getContext("initialized");

    export let sorting = { field: "startTime", order: "DESC" };
    export let matchedJobs = 0;
    export let metrics = ccconfig.plot_list_selectedMetrics;
    export let showFootprint;

    let itemsPerPage = ccconfig.plot_list_jobsPerPage;
    let page = 1;
    let paging = { itemsPerPage, page };
    let filter = [];

    const client = getContextClient();
    const query = gql`
        query (
            $filter: [JobFilter!]!
            $sorting: OrderByInput!
            $paging: PageRequest!
        ) {
            jobs(filter: $filter, order: $sorting, page: $paging) {
                items {
                    id
                    jobId
                    user
                    project
                    cluster
                    subCluster
                    startTime
                    duration
                    numNodes
                    numHWThreads
                    numAcc
                    walltime
                    resources {
                        hostname
                    }
                    SMT
                    exclusive
                    partition
                    arrayJobId
                    monitoringStatus
                    state
                    tags {
                        id
                        type
                        name
                    }
                    userData {
                        name
                    }
                    metaData
                    flopsAnyAvg
                    memBwAvg
                    loadAvg
                }
                count
            }
        }
    `;

    $: jobs = queryStore({
        client: client,
        query: query,
        variables: { paging, sorting, filter }
    });

    $: matchedJobs = $jobs.data != null ? $jobs.data.jobs.count : 0;

    // Force refresh list with existing unchanged variables (== usually would not trigger reactivity)
    export function refresh() {
        jobs = queryStore({
            client: client,
            query: query,
            variables: { paging, sorting, filter },
            requestPolicy: 'network-only'
        });
    }

    // (Re-)query and optionally set new filters.
    export function update(filters) {
        if (filters != null) {
            let minRunningFor = ccconfig.plot_list_hideShortRunningJobs;
            if (minRunningFor && minRunningFor > 0) {
                filters.push({ minRunningFor });
            }
            filter = filters;
        }
        page = 1;
        paging = paging = { page, itemsPerPage };
    }

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

    function updateConfiguration(value, page) {
        updateConfigurationMutation({ name: 'plot_list_jobsPerPage', value: value })
        .subscribe(res => {
            if (res.fetching === false && !res.error) {
                paging = { itemsPerPage: value, page: page }; // Trigger reload of jobList
            } else if (res.fetching === false && res.error) {
                throw res.error
                // console.log('Error on subscription: ' + res.error)
            }
        })
    };

    let plotWidth = null;
    let tableWidth = null;
    let jobInfoColumnWidth = 250;

    $: if (showFootprint) {
        plotWidth = Math.floor(
            (tableWidth - jobInfoColumnWidth) / (metrics.length + 1) - 10
        )
    } else { 
        plotWidth = Math.floor(
            (tableWidth - jobInfoColumnWidth) / metrics.length - 10
        )
    }

    let headerPaddingTop = 0;
    stickyHeader(
        ".cc-table-wrapper > table.table >thead > tr > th.position-sticky:nth-child(1)",
        (x) => (headerPaddingTop = x)
    );
</script>

<Row>
    <div class="col cc-table-wrapper" bind:clientWidth={tableWidth}>
        <Table cellspacing="0px" cellpadding="0px">
            <thead>
                <tr>
                    <th
                        class="position-sticky top-0"
                        scope="col"
                        style="width: {jobInfoColumnWidth}px; padding-top: {headerPaddingTop}px"
                    >
                        Job Info
                    </th>
                    {#if showFootprint}
                        <th
                            class="position-sticky top-0"
                            scope="col"
                            style="width: {plotWidth}px; padding-top: {headerPaddingTop}px"
                        >
                            Job Footprint
                        </th>
                    {/if}
                    {#each metrics as metric (metric)}
                        <th
                            class="position-sticky top-0 text-center"
                            scope="col"
                            style="width: {plotWidth}px; padding-top: {headerPaddingTop}px"
                        >
                            {metric}
                            {#if $initialized}
                                ({clusters
                                    .map((cluster) =>
                                        cluster.metricConfig.find(
                                            (m) => m.name == metric
                                        )
                                    )
                                    .filter((m) => m != null)
                                    .map(
                                        (m) =>
                                            (m.unit?.prefix
                                                ? m.unit?.prefix
                                                : "") +
                                            (m.unit?.base ? m.unit?.base : "")
                                    ) // Build unitStr
                                    .reduce(
                                        (arr, unitStr) =>
                                            arr.includes(unitStr)
                                                ? arr
                                                : [...arr, unitStr],
                                        []
                                    ) // w/o this, output would be [unitStr, unitStr]
                                    .join(", ")})
                            {/if}
                        </th>
                    {/each}
                </tr>
            </thead>
            <tbody>
                {#if $jobs.error}
                    <tr>
                        <td colspan={metrics.length + 1}>
                            <Card body color="danger" class="mb-3"
                                ><h2>{$jobs.error.message}</h2></Card
                            >
                        </td>
                    </tr>
                {:else if $jobs.fetching || !$jobs.data}
                    <tr>
                        <td colspan={metrics.length + 1}>
                            <Spinner secondary />
                        </td>
                    </tr>
                {:else if $jobs.data && $initialized}
                    {#each $jobs.data.jobs.items as job (job)}
                        <JobListRow {job} {metrics} {plotWidth} {showFootprint}/>
                    {:else}
                        <tr>
                            <td colspan={metrics.length + 1}>
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
    bind:page
    {itemsPerPage}
    itemText="Jobs"
    totalItems={matchedJobs}
    on:update={({ detail }) => {
        if (detail.itemsPerPage != itemsPerPage) {
            updateConfiguration(
                detail.itemsPerPage.toString(),
                detail.page
            )
        } else {
            paging = { itemsPerPage: detail.itemsPerPage, page: detail.page }
        }
    }}
/>

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
