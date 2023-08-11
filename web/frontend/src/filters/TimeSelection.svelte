<script>
    import { Icon, Input, InputGroup, InputGroupText } from 'sveltestrap'
    import { createEventDispatcher } from "svelte"

    export let from
    export let to
    export let customEnabled = true
    export let anyEnabled = false
    export let options = {
        'Last quarter hour': 15*60,
        'Last half hour': 30*60,
        'Last hour': 60*60,
        'Last 2hrs': 2*60*60,
        'Last 4hrs': 4*60*60,
        'Last 12hrs': 12*60*60,
        'Last 24hrs': 24*60*60
    }

    $: pendingFrom = from
    $: pendingTo = to

    const dispatch = createEventDispatcher()
    let timeRange = to && from
        ? (to.getTime() - from.getTime()) / 1000
        : (anyEnabled ? -2 : -1)

    function updateTimeRange(event) {
        if (timeRange == -1) {
            pendingFrom = null
            pendingTo = null
            return
        }
        if (timeRange == -2) {
            from = pendingFrom = null
            to = pendingTo = null
            dispatch('change', { from, to })
            return
        }

        let now = Date.now(), t = timeRange * 1000
        from = pendingFrom = new Date(now - t)
        to = pendingTo = new Date(now)
        dispatch('change', { from, to })
    }

    function updateExplicitTimeRange(type, event) {
        let d = new Date(Date.parse(event.target.value));
        if (type == 'from') pendingFrom = d
        else                pendingTo = d

        if (pendingFrom != null && pendingTo != null) {
            from = pendingFrom
            to = pendingTo
            dispatch('change', { from, to })
        }
    }
</script>

<InputGroup class="inline-from">
    <InputGroupText><Icon name="clock-history"/></InputGroupText>
    <!-- <InputGroupText>
        Time
    </InputGroupText> -->
    <select class="form-select" bind:value={timeRange} on:change={updateTimeRange}>
        {#if customEnabled}
            <option value={-1}>Custom</option>            
        {/if}
        {#if anyEnabled}
            <option value={-2}>Any</option>
        {/if}
        {#each Object.entries(options) as [name, seconds]}
            <option value={seconds}>{name}</option>
        {/each}
    </select>
    {#if timeRange == -1}
        <InputGroupText>from</InputGroupText>
        <Input type="datetime-local" on:change={(event) => updateExplicitTimeRange('from', event)}></Input>
        <InputGroupText>to</InputGroupText>
        <Input type="datetime-local" on:change={(event) => updateExplicitTimeRange('to', event)}></Input>
    {/if}
</InputGroup>
