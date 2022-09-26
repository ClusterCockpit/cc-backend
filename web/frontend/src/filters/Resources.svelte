<script>
    import { createEventDispatcher, getContext } from 'svelte'
    import { Button, Modal, ModalBody, ModalHeader, ModalFooter } from 'sveltestrap'
import Header from '../Header.svelte';
    import DoubleRangeSlider from './DoubleRangeSlider.svelte'

    const clusters = getContext('clusters'),
          initialized = getContext('initialized'),
          dispatch = createEventDispatcher()

    export let cluster = null
    export let isModified = false
    export let isOpen = false
    export let numNodes = { from: null, to: null }
    export let numHWThreads = { from: null, to: null }
    export let numAccelerators = { from: null, to: null }

    let pendingNumNodes = numNodes, pendingNumHWThreads = numHWThreads, pendingNumAccelerators = numAccelerators
    $: isModified = pendingNumNodes.from != numNodes.from || pendingNumNodes.to != numNodes.to
        || pendingNumHWThreads.from != numHWThreads.from || pendingNumHWThreads.to != numHWThreads.to
        || pendingNumAccelerators.from != numAccelerators.from || pendingNumAccelerators.to != numAccelerators.to

    const findMaxNumAccels = clusters => clusters.reduce((max, cluster) => Math.max(max,
        cluster.subClusters.reduce((max, sc) => Math.max(max, sc.topology.accelerators?.length || 0), 0)), 0)

        console.log(header)
    let minNumNodes = 1, maxNumNodes = 0, minNumHWThreads = 1, maxNumHWThreads = 0, minNumAccelerators = 0, maxNumAccelerators = 0
    $: {
        if ($initialized) {
            if (cluster != null) {
                const { subClusters } = clusters.find(c => c.name == cluster)
                const { filterRanges } = header.clusters.find(c => c.name == cluster)
                minNumNodes = filterRanges.numNodes.from
                maxNumNodes = filterRanges.numNodes.to
                maxNumAccelerators = findMaxNumAccels([{ subClusters }])
            } else if (clusters.length > 0) {
                const { filterRanges } = header.clusters[0]
                minNumNodes = filterRanges.numNodes.from
                maxNumNodes = filterRanges.numNodes.to
                maxNumAccelerators = findMaxNumAccels(clusters)
                for (let cluster of header.clusters) {
                    const { filterRanges } = cluster
                    minNumNodes = Math.min(minNumNodes, filterRanges.numNodes.from)
                    maxNumNodes = Math.max(maxNumNodes, filterRanges.numNodes.to)
                }
            }
        }
    }

    $: {
        if (isOpen && $initialized && pendingNumNodes.from == null && pendingNumNodes.to == null) {
            pendingNumNodes = { from: 0, to: maxNumNodes }
        }
    }
</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Select Number of Nodes, HWThreads and Accelerators
    </ModalHeader>
    <ModalBody>
        <h4>Number of Nodes</h4>
        <DoubleRangeSlider
            on:change={({ detail }) => (pendingNumNodes = { from: detail[0], to: detail[1] })}
            min={minNumNodes} max={maxNumNodes}
            firstSlider={pendingNumNodes.from} secondSlider={pendingNumNodes.to} />
        <!-- <DoubleRangeSlider
            on:change={({ detail }) => (pendingNumHWThreads = { from: detail[0], to: detail[1] })}
            min={minNumHWThreads} max={maxNumHWThreads}
            firstSlider={pendingNumHWThreads.from} secondSlider={pendingNumHWThreads.to} /> -->
        {#if maxNumAccelerators != null && maxNumAccelerators > 1}
            <DoubleRangeSlider
                on:change={({ detail }) => (pendingNumAccelerators = { from: detail[0], to: detail[1] })}
                min={minNumAccelerators} max={maxNumAccelerators}
                firstSlider={pendingNumAccelerators.from} secondSlider={pendingNumAccelerators.to} />
        {/if}
    </ModalBody>
    <ModalFooter>
        <Button color="primary"
            disabled={pendingNumNodes.from == null || pendingNumNodes.to == null}
            on:click={() => {
                isOpen = false
                numNodes = { from: pendingNumNodes.from, to: pendingNumNodes.to }
                numHWThreads = { from: pendingNumHWThreads.from, to: pendingNumHWThreads.to }
                numAccelerators = { from: pendingNumAccelerators.from, to: pendingNumAccelerators.to }
                dispatch('update', { numNodes, numHWThreads, numAccelerators })
            }}>
            Close & Apply
        </Button>
        <Button color="danger" on:click={() => {
            isOpen = false
            pendingNumNodes = { from: null, to: null }
            pendingNumHWThreads = { from: null, to: null }
            pendingNumAccelerators = { from: null, to: null }
            numNodes = { from: pendingNumNodes.from, to: pendingNumNodes.to }
            numHWThreads = { from: pendingNumHWThreads.from, to: pendingNumHWThreads.to }
            numAccelerators = { from: pendingNumAccelerators.from, to: pendingNumAccelerators.to }
            dispatch('update', { numNodes, numHWThreads, numAccelerators })
        }}>Reset</Button>
        <Button on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
</Modal>
