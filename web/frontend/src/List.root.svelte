<!--
    @component List of users or projects
 -->
<script>
    import { onMount } from 'svelte'
    import { init } from './utils.js'
    import { Row, Col, Button, Icon, Table, Card, Spinner,
             InputGroup, Input } from 'sveltestrap'
    import Filters from './filters/Filters.svelte'
    import { operationStore, query } from '@urql/svelte';
    import { scramble, scrambleNames } from './joblist/JobInfo.svelte'

    const { } = init()

    export let type
    export let filterPresets

    console.assert(type == 'USER' || type == 'PROJECT', 'Invalid list type provided!')

    const stats = operationStore(`query($filter: [JobFilter!]!) {
        rows: jobsStatistics(filter: $filter, groupBy: ${type}) {
            id
            name
            totalJobs
            totalWalltime
            totalCoreHours
        }
    }`, {
        filter: []
    }, {
        pause: true
    })

    query(stats)

    let filters
    let nameFilter = ''
    let sorting = { field: 'totalJobs', direction: 'down' }

    function changeSorting(event, field) {
        let target = event.target
        while (target.tagName != 'BUTTON')
            target = target.parentElement

        let direction = target.children[0].className.includes('up') ? 'down' : 'up'
        target.children[0].className = `bi-sort-numeric-${direction}`
        sorting = { field, direction }
    }

    function sort(stats, sorting, nameFilter) {
        const cmp = sorting.field == 'id'
            ? (sorting.direction == 'up'
                ? (a, b) => a.id < b.id
                : (a, b) => a.id > b.id)
            : (sorting.direction == 'up'
                ? (a, b) => a[sorting.field] - b[sorting.field]
                : (a, b) => b[sorting.field] - a[sorting.field])

        return stats.filter(u => u.id.includes(nameFilter)).sort(cmp)
    }

    onMount(() => filters.update())
</script>

<Row>
    <Col xs="auto">
        <InputGroup>
            <Button disabled outline>
                Search {type.toLowerCase()}s
            </Button>
            <Input bind:value={nameFilter} placeholder="Filter by {({ USER: 'username', PROJECT: 'project' })[type]}" />
        </InputGroup>
    </Col>
    <Col xs="auto">
        <Filters
            bind:this={filters}
            filterPresets={filterPresets}
            startTimeQuickSelect={true}
            menuText="Only {type.toLowerCase()}s with jobs that match the filters will show up"
            on:update={({ detail }) => {
                $stats.variables = { filter: detail.filters }
                $stats.context.pause = false
                $stats.reexecute()
            }} />
    </Col>
</Row>
<Table>
    <thead>
        <tr>
            <th scope="col">
                {({ USER: 'Username', PROJECT: 'Project Name' })[type]}
                <Button color="{sorting.field == 'id' ? 'primary' : 'light'}"
                    size="sm" on:click={e => changeSorting(e, 'id')}>
                    <Icon name="sort-numeric-down" />
                </Button>
            </th>
            {#if type == 'USER'}
                <th scope="col">
                    Name
                    <Button color="{sorting.field == 'name' ? 'primary' : 'light'}"
                        size="sm" on:click={e => changeSorting(e, 'name')}>
                        <Icon name="sort-numeric-down" />
                    </Button>
                </th>
            {/if}
            <th scope="col">
                Total Jobs
                <Button color="{sorting.field == 'totalJobs' ? 'primary' : 'light'}"
                    size="sm" on:click={e => changeSorting(e, 'totalJobs')}>
                    <Icon name="sort-numeric-down" />
                </Button>
            </th>
            <th scope="col">
                Total Walltime
                <Button color="{sorting.field == 'totalWalltime' ? 'primary' : 'light'}"
                    size="sm" on:click={e => changeSorting(e, 'totalWalltime')}>
                    <Icon name="sort-numeric-down" />
                </Button>
            </th>
            <th scope="col">
                Total Core Hours
                <Button color="{sorting.field == 'totalCoreHours' ? 'primary' : 'light'}"
                    size="sm" on:click={e => changeSorting(e, 'totalCoreHours')}>
                    <Icon name="sort-numeric-down" />
                </Button>
            </th>
        </tr>
    </thead>
    <tbody>
        {#if $stats.fetching}
            <tr>
                <td colspan="4" style="text-align: center;"><Spinner secondary/></td>
            </tr>
        {:else if $stats.error}
            <tr>
                <td colspan="4"><Card body color="danger" class="mb-3">{$stats.error.message}</Card></td>
            </tr>
        {:else if $stats.data}
            {#each sort($stats.data.rows, sorting, nameFilter) as row (row.id)}
                <tr>
                    <td>
                        {#if type == 'USER'}
                            <a href="/monitoring/user/{row.id}">{scrambleNames ? scramble(row.id) : row.id}</a>
                        {:else if type == 'PROJECT'}
                            <a href="/monitoring/jobs/?project={row.id}">{row.id}</a>
                        {:else}
                            {row.id}
                        {/if}
                    </td>
                    {#if type == 'USER'}
                        <td>{row?.name ? row.name : ''}</td>
                    {/if}
                    <td>{row.totalJobs}</td>
                    <td>{row.totalWalltime}</td>
                    <td>{row.totalCoreHours}</td>
                </tr>
            {:else}
                <tr>
                    <td colspan="4"><i>No {type.toLowerCase()}s/jobs found</i></td>
                </tr>
            {/each}
        {/if}
    </tbody>
</Table>
