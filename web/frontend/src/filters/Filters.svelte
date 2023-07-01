<!--
    @component

    Properties:
    - menuText:      String? (Optional text to show in the dropdown menu)
    - filterPresets: Object? (Optional predefined filter values)
    Events:
    - 'update': The detail's 'filters' prop are new filter items to be applied
    Functions:
    - void update(additionalFilters: Object?): Triggers an update
 -->
<script>
    import { Row, Col, DropdownItem, DropdownMenu,
             DropdownToggle, ButtonDropdown, Icon } from 'sveltestrap'
    import { createEventDispatcher } from 'svelte'
    import Info from './InfoBox.svelte'
    import Cluster from './Cluster.svelte'
    import JobStates, { allJobStates } from './JobStates.svelte'
    import StartTime from './StartTime.svelte'
    import Tags from './Tags.svelte'
    import Tag from '../Tag.svelte'
    import Duration from './Duration.svelte'
    import Resources from './Resources.svelte'
    import Statistics from './Stats.svelte'
    // import TimeSelection from './TimeSelection.svelte'

    const dispatch = createEventDispatcher()

    export let menuText = null
    export let filterPresets = {}
    export let disableClusterSelection = false
    export let startTimeQuickSelect = false

    let filters = {
        projectMatch: filterPresets.projectMatch  || 'contains',
        userMatch:    filterPresets.userMatch     || 'contains',
        jobIdMatch:   filterPresets.jobIdMatch    || 'eq',

        cluster:     filterPresets.cluster    || null,
        partition:   filterPresets.partition  || null,
        states:      filterPresets.states     || filterPresets.state ? [filterPresets.state].flat() : allJobStates,
        startTime:   filterPresets.startTime  || { from: null, to: null },
        tags:        filterPresets.tags       || [],
        duration:    filterPresets.duration   || { from: null, to: null },
        jobId:       filterPresets.jobId      || '',
        arrayJobId:  filterPresets.arrayJobId || null,
        user:        filterPresets.user       || '',
        project:     filterPresets.project    || '',
        jobName:     filterPresets.jobName    || '',

        node:             filterPresets.node             || null,
        numNodes:         filterPresets.numNodes         || { from: null, to: null },
        numHWThreads:     filterPresets.numHWThreads     || { from: null, to: null },
        numAccelerators:  filterPresets.numAccelerators  || { from: null, to: null },

        stats: [],
    }

    let isClusterOpen = false,
        isJobStatesOpen = false,
        isStartTimeOpen = false,
        isTagsOpen = false,
        isDurationOpen = false,
        isResourcesOpen = false,
        isStatsOpen = false,
        isNodesModified = false,
        isHwthreadsModified = false,
        isAccsModified = false

    // Can be called from the outside to trigger a 'update' event from this component.
    export function update(additionalFilters = null) {
        if (additionalFilters != null)
            for (let key in additionalFilters)
            filters[key] = additionalFilters[key]

        let items = []
        if (filters.cluster)
            items.push({ cluster: { eq: filters.cluster } })
        if (filters.node)
            items.push({ node: { contains: filters.node } })
        if (filters.partition)
            items.push({ partition: { eq: filters.partition } })
        if (filters.states.length != allJobStates.length)
            items.push({ state: filters.states })
        if (filters.startTime.from || filters.startTime.to)
            items.push({ startTime: { from: filters.startTime.from, to: filters.startTime.to } })
        if (filters.tags.length != 0)
            items.push({ tags: filters.tags })
        if (filters.duration.from || filters.duration.to)
            items.push({ duration: { from: filters.duration.from, to: filters.duration.to } })
        if (filters.jobId)
            items.push({ jobId: { [filters.jobIdMatch]: filters.jobId } })
        if (filters.arrayJobId != null)
            items.push({ arrayJobId: filters.arrayJobId })
        if (filters.numNodes.from != null || filters.numNodes.to != null)
            items.push({ numNodes: { from: filters.numNodes.from, to: filters.numNodes.to } })
        if (filters.numHWThreads.from != null || filters.numHWThreads.to != null)
            items.push({ numHWThreads: { from: filters.numHWThreads.from, to: filters.numHWThreads.to } })
        if (filters.numAccelerators.from != null || filters.numAccelerators.to != null)
            items.push({ numAccelerators: { from: filters.numAccelerators.from, to: filters.numAccelerators.to } })
        if (filters.user)
            items.push({ user: { [filters.userMatch]: filters.user } })
        if (filters.project)
            items.push({ project: { [filters.projectMatch]: filters.project } })
        if (filters.jobName)
            items.push({ jobName: { contains: filters.jobName } })
        for (let stat of filters.stats)
            items.push({ [stat.field]: { from: stat.from, to: stat.to } })

        dispatch('update', { filters: items })
        changeURL()
        return items
    }

    function changeURL() {
        const dateToUnixEpoch = (rfc3339) => Math.floor(Date.parse(rfc3339) / 1000)

        let opts = []
        if (filters.cluster)
            opts.push(`cluster=${filters.cluster}`)
        if (filters.node)
            opts.push(`node=${filters.node}`)
        if (filters.partition)
            opts.push(`partition=${filters.partition}`)
        if (filters.states.length != allJobStates.length)
            for (let state of filters.states)
                opts.push(`state=${state}`)
        if (filters.startTime.from && filters.startTime.to)
            // if (filters.startTime.url) {
            //     opts.push(`startTime=${filters.startTime.url}`)
            // } else {
                opts.push(`startTime=${dateToUnixEpoch(filters.startTime.from)}-${dateToUnixEpoch(filters.startTime.to)}`)
            // }
        if (filters.jobId.length != 0)
            if (filters.jobIdMatch != 'in') {
                opts.push(`jobId=${filters.jobId}`)
            } else {
                for (let singleJobId of filters.jobId)
                opts.push(`jobId=${singleJobId}`)
            }
        if (filters.jobIdMatch != 'eq')
            opts.push(`jobIdMatch=${filters.jobIdMatch}`)
        for (let tag of filters.tags)
            opts.push(`tag=${tag}`)
        if (filters.duration.from && filters.duration.to)
            opts.push(`duration=${filters.duration.from}-${filters.duration.to}`)
        if (filters.numNodes.from && filters.numNodes.to)
            opts.push(`numNodes=${filters.numNodes.from}-${filters.numNodes.to}`)
        if (filters.numAccelerators.from && filters.numAccelerators.to)
            opts.push(`numAccelerators=${filters.numAccelerators.from}-${filters.numAccelerators.to}`)
        if (filters.user.length != 0)
            if (filters.userMatch != 'in') {
                opts.push(`user=${filters.user}`)
            } else {
                for (let singleUser of filters.user)
                    opts.push(`user=${singleUser}`)
            }
        if (filters.userMatch != 'contains')
            opts.push(`userMatch=${filters.userMatch}`)
        if (filters.project)
            opts.push(`project=${filters.project}`)
        if (filters.jobName)
            opts.push(`jobName=${filters.jobName}`)
        if (filters.projectMatch != 'contains')
            opts.push(`projectMatch=${filters.projectMatch}`)

        if (opts.length == 0 && window.location.search.length <= 1)
            return

        let newurl = `${window.location.pathname}?${opts.join('&')}`
        window.history.replaceState(null, '', newurl)
    }
</script>

<Row>
    <Col xs="auto">
        <ButtonDropdown class="cc-dropdown-on-hover">
            <DropdownToggle outline caret color="success">
                <Icon name="sliders"/>
                Filters
            </DropdownToggle>
            <DropdownMenu>
                <DropdownItem header>
                    Manage Filters
                </DropdownItem>
                {#if menuText}
                    <DropdownItem disabled>{menuText}</DropdownItem>
                    <DropdownItem divider />
                {/if}
                <DropdownItem on:click={() => (isClusterOpen = true)}>
                    <Icon name="cpu"/> Cluster/Partition
                </DropdownItem>
                <DropdownItem on:click={() => (isJobStatesOpen = true)}>
                    <Icon name="gear-fill"/> Job States
                </DropdownItem>
                <DropdownItem on:click={() => (isStartTimeOpen = true)}>
                    <Icon name="calendar-range"/> Start Time
                </DropdownItem>
                <DropdownItem on:click={() => (isDurationOpen = true)}>
                    <Icon name="stopwatch"/> Duration
                </DropdownItem>
                <DropdownItem on:click={() => (isTagsOpen = true)}>
                    <Icon name="tags"/> Tags
                </DropdownItem>
                <DropdownItem on:click={() => (isResourcesOpen = true)}>
                    <Icon name="hdd-stack"/> Resources
                </DropdownItem>
                <DropdownItem on:click={() => (isStatsOpen = true)}>
                    <Icon name="bar-chart" on:click={() => (isStatsOpen = true)}/> Statistics
                </DropdownItem>
                {#if startTimeQuickSelect}
                    <DropdownItem divider/>
                    <DropdownItem disabled>Start Time Qick Selection</DropdownItem>
                    {#each [
                        { text: 'Last 6hrs', url: 'last6h',   seconds: 6*60*60 },
                        // { text: 'Last 12hrs',   seconds: 12*60*60 },
                        { text: 'Last 24hrs', url: 'last24h',  seconds: 24*60*60 },
                        // { text: 'Last 48hrs',   seconds: 48*60*60 },
                        { text: 'Last 7 days', url: 'last7d', seconds: 7*24*60*60 },
                        { text: 'Last 30 days', url: 'last30d', seconds: 30*24*60*60 }
                    ] as {text, url, seconds}}
                        <DropdownItem on:click={() => {
                            filters.startTime.from = (new Date(Date.now() - seconds * 1000)).toISOString()
                            filters.startTime.to = (new Date(Date.now())).toISOString()
                            filters.startTime.text = text,
                            filters.startTime.url = url
                            update()
                        }}>
                            <Icon name="calendar-range"/> {text}
                        </DropdownItem>
                    {/each}
                {/if}
            </DropdownMenu>
        </ButtonDropdown>
    </Col>
    <Col xs="auto">
        {#if filters.cluster}
            <Info icon="cpu" on:click={() => (isClusterOpen = true)}>
                {filters.cluster}
                {#if filters.partition}
                    ({filters.partition})
                {/if}
            </Info>
        {/if}

        {#if filters.states.length != allJobStates.length}
            <Info icon="gear-fill" on:click={() => (isJobStatesOpen = true)}>
                {filters.states.join(', ')}
            </Info>
        {/if}

        {#if filters.startTime.from || filters.startTime.to}
            <Info icon="calendar-range" on:click={() => (isStartTimeOpen = true)}>
                {#if filters.startTime.text}
                    {filters.startTime.text}
                {:else}
                    {new Date(filters.startTime.from).toLocaleString()} - {new Date(filters.startTime.to).toLocaleString()}
                {/if}
             </Info>
        {/if}

        {#if filters.duration.from || filters.duration.to}
            <Info icon="stopwatch" on:click={() => (isDurationOpen = true)}>
                {Math.floor(filters.duration.from / 3600)}h:{Math.floor(filters.duration.from % 3600 / 60)}m
                -
                {Math.floor(filters.duration.to / 3600)}h:{Math.floor(filters.duration.to % 3600 / 60)}m
            </Info>
        {/if}

        {#if filters.tags.length != 0}
            <Info icon="tags" on:click={() => (isTagsOpen = true)}>
                {#each filters.tags as tagId}
                    <Tag id={tagId} clickable={false} />
                {/each}
            </Info>
        {/if}

        {#if filters.numNodes.from != null          || filters.numNodes.to != null          || 
             filters.numHWThreads.from != null      || filters.numHWThreads.to != null      || 
             filters.numAccelerators.from != null   || filters.numAccelerators.to != null   }
            <Info icon="hdd-stack" on:click={() => (isResourcesOpen = true)}>
                {#if isNodesModified } Nodes: {filters.numNodes.from} - {filters.numNodes.to} {/if}
                {#if isNodesModified && isHwthreadsModified }, {/if}
                {#if isHwthreadsModified } HWThreads: {filters.numHWThreads.from} - {filters.numHWThreads.to} {/if}
                {#if (isNodesModified || isHwthreadsModified) && isAccsModified }, {/if}
                {#if isAccsModified } Accelerators: {filters.numAccelerators.from} - {filters.numAccelerators.to} {/if}
            </Info>
        {/if}

        {#if filters.node != null }
           <Info icon="hdd-stack" on:click={() => (isResourcesOpen = true)}>
               Node: {filters.node}
           </Info>
        {/if}

        {#if filters.stats.length > 0}
            <Info icon="bar-chart" on:click={() => (isStatsOpen = true)}>
                {filters.stats.map(stat => `${stat.text}: ${stat.from} - ${stat.to}`).join(', ')}
            </Info>
        {/if}
    </Col>
</Row>

<Cluster
    disableClusterSelection={disableClusterSelection}
    bind:isOpen={isClusterOpen}
    bind:cluster={filters.cluster}
    bind:partition={filters.partition}
    on:update={() => update()} />

<JobStates
    bind:isOpen={isJobStatesOpen}
    bind:states={filters.states}
    on:update={() => update()} />

<StartTime
    bind:isOpen={isStartTimeOpen}
    bind:from={filters.startTime.from}
    bind:to={filters.startTime.to}
    on:update={() => {
        delete filters.startTime['text']
        delete filters.startTime['url']
        update()
    }} />

<Duration
    bind:isOpen={isDurationOpen}
    bind:from={filters.duration.from}
    bind:to={filters.duration.to}
    on:update={() => update()} />

<Tags
    bind:isOpen={isTagsOpen}
    bind:tags={filters.tags}
    on:update={() => update()} />

<Resources cluster={filters.cluster}
    bind:isOpen={isResourcesOpen}
    bind:numNodes={filters.numNodes}
    bind:numHWThreads={filters.numHWThreads}
    bind:numAccelerators={filters.numAccelerators}
    bind:namedNode={filters.node}
    bind:isNodesModified={isNodesModified}
    bind:isHwthreadsModified={isHwthreadsModified}
    bind:isAccsModified={isAccsModified}
    on:update={() => update()} />

<Statistics cluster={filters.cluster}
    bind:isOpen={isStatsOpen}
    bind:stats={filters.stats}
    on:update={() => update()} />

<style>
    :global(.cc-dropdown-on-hover:hover .dropdown-menu) {
        display: block;
        margin-top: 0px;
        padding-top: 0px;
        transform: none !important;
    }
</style>
