<script>
    import { createEventDispatcher, getContext } from 'svelte'
    import { Button, Modal, ModalBody, ModalHeader, ModalFooter } from 'sveltestrap'
    import DoubleRangeSlider from './DoubleRangeSlider.svelte'

    const clusters = getContext('clusters'),
          initialized = getContext('initialized'),
          dispatch = createEventDispatcher()

    export let cluster = null
    export let isModified = false
    export let isOpen = false
    export let stats = []

    let statistics = [
        {
            field: 'flopsAnyAvg',
            text: 'FLOPs (Avg.)',
            metric: 'flops_any',
            from: 0, to: 0, peak: 0,
            enabled: false
        },
        {
            field: 'memBwAvg',
            text: 'Mem. Bw. (Avg.)',
            metric: 'mem_bw',
            from: 0, to: 0, peak: 0,
            enabled: false
        },
        {
            field: 'loadAvg',
            text: 'Load (Avg.)',
            metric: 'cpu_load',
            from: 0, to: 0, peak: 0,
            enabled: false
        },
        {
            field: 'memUsedMax',
            text: 'Mem. used (Max.)',
            metric: 'mem_used',
            from: 0, to: 0, peak: 0,
            enabled: false
        }
    ]

    $: isModified = !statistics.every(a => {
        let b = stats.find(s => s.field == a.field)
        if (b == null)
            return !a.enabled

        return a.from == b.from && a.to == b.to
    })

    function getPeak(cluster, metric) {
        const mc = cluster.metricConfig.find(mc => mc.name == metric)
        return mc ? mc.peak : 0
    }

    function resetRange(isInitialized, cluster) {
        if (!isInitialized)
            return

        if (cluster != null) {
            let c = clusters.find(c => c.name == cluster)
            for (let stat of statistics) {
                stat.peak = getPeak(c, stat.metric)
                stat.from = 0
                stat.to = stat.peak
            }
        } else {
            for (let stat of statistics) {
                for (let c of clusters) {
                    stat.peak = Math.max(stat.peak, getPeak(c, stat.metric))
                }
                stat.from = 0
                stat.to = stat.peak
            }
        }

        statistics = [...statistics]
    }

    $: resetRange($initialized, cluster)
</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Filter based on statistics (of non-running jobs)
    </ModalHeader>
    <ModalBody>
        {#each statistics as stat}
            <h4>{stat.text}</h4>
            <DoubleRangeSlider
                on:change={({ detail }) => (stat.from = detail[0], stat.to = detail[1], stat.enabled = true)}
                min={0} max={stat.peak}
                firstSlider={stat.from} secondSlider={stat.to} />
        {/each}
    </ModalBody>
    <ModalFooter>
        <Button color="primary" on:click={() => {
            isOpen = false
            stats = statistics.filter(stat => stat.enabled)
            dispatch('update', { stats })
        }}>Close & Apply</Button>
        <Button color="danger" on:click={() => {
            isOpen = false
            statistics.forEach(stat => stat.enabled = false)
            stats = []
            dispatch('update', { stats })
        }}>Reset</Button>
        <Button on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
</Modal>
