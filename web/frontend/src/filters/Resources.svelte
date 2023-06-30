<script>
    import { createEventDispatcher, getContext } from 'svelte'
    import { Button, Modal, ModalBody, ModalHeader, ModalFooter } from 'sveltestrap'
    import Header from '../Header.svelte';
    import DoubleRangeSlider from './DoubleRangeSlider.svelte'

    const clusters = getContext('clusters'),
          initialized = getContext('initialized'),
          dispatch = createEventDispatcher()

    export let cluster = null
    export let isOpen = false
    export let numNodes = { from: null, to: null }
    export let numHWThreads = { from: null, to: null }
    export let numAccelerators = { from: null, to: null }
    export let isNodesModified = false
    export let isHwthreadsModified = false
    export let isAccsModified = false
    export let namedNode = null

    let pendingNumNodes = numNodes, pendingNumHWThreads = numHWThreads, pendingNumAccelerators = numAccelerators, pendingNamedNode = namedNode

    const findMaxNumAccels = clusters => clusters.reduce((max, cluster) => Math.max(max,
        cluster.subClusters.reduce((max, sc) => Math.max(max, sc.topology.accelerators?.length || 0), 0)), 0)

    // Limited to Single-Node Thread Count
    const findMaxNumHWTreadsPerNode = clusters => clusters.reduce((max, cluster) => Math.max(max,
        cluster.subClusters.reduce((max, sc) => Math.max(max, (sc.threadsPerCore * sc.coresPerSocket * sc.socketsPerNode) || 0), 0)), 0)

    // console.log(header)
    let minNumNodes = 1, maxNumNodes = 0, minNumHWThreads = 1, maxNumHWThreads = 0, minNumAccelerators = 0, maxNumAccelerators = 0
    $: {
        if ($initialized) {
            if (cluster != null) {
                const { subClusters } = clusters.find(c => c.name == cluster)
                const { filterRanges } = header.clusters.find(c => c.name == cluster)
                minNumNodes = filterRanges.numNodes.from
                maxNumNodes = filterRanges.numNodes.to
                maxNumAccelerators = findMaxNumAccels([{ subClusters }])
                maxNumHWThreads = findMaxNumHWTreadsPerNode([{ subClusters }])
            } else if (clusters.length > 0) {
                const { filterRanges } = header.clusters[0]
                minNumNodes = filterRanges.numNodes.from
                maxNumNodes = filterRanges.numNodes.to
                maxNumAccelerators = findMaxNumAccels(clusters)
                maxNumHWThreads = findMaxNumHWTreadsPerNode(clusters)
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

    $: {
        if (isOpen && $initialized && ((pendingNumHWThreads.from == null && pendingNumHWThreads.to == null) || (isHwthreadsModified == false))) {
            pendingNumHWThreads = { from: 0, to: maxNumHWThreads }
        }
    }

    $: if ( maxNumAccelerators != null && maxNumAccelerators > 1 ) {
        if (isOpen && $initialized && pendingNumAccelerators.from == null && pendingNumAccelerators.to == null) {
            pendingNumAccelerators = { from: 0, to: maxNumAccelerators }
        }
    }
</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Select number of utilized Resources
    </ModalHeader>
    <ModalBody>
        <h6>Named Node</h6>
            <input type="text" class="form-control"  bind:value={pendingNamedNode}>
        <h6 style="margin-top: 1rem;">Number of Nodes</h6>
        <DoubleRangeSlider
            on:change={({ detail }) => {
                pendingNumNodes = { from: detail[0], to: detail[1] }
                isNodesModified = true
            }}
            min={minNumNodes} max={maxNumNodes}
            firstSlider={pendingNumNodes.from} secondSlider={pendingNumNodes.to}
            inputFieldFrom={pendingNumNodes.from} inputFieldTo={pendingNumNodes.to}/>
        <h6 style="margin-top: 1rem;">Number of HWThreads (Use for Single-Node Jobs)</h6>
        <DoubleRangeSlider
            on:change={({ detail }) => {
                pendingNumHWThreads = { from: detail[0], to: detail[1] }
                isHwthreadsModified = true
            }}
            min={minNumHWThreads} max={maxNumHWThreads}
            firstSlider={pendingNumHWThreads.from} secondSlider={pendingNumHWThreads.to}
            inputFieldFrom={pendingNumHWThreads.from} inputFieldTo={pendingNumHWThreads.to}/>
        {#if maxNumAccelerators != null && maxNumAccelerators > 1}
            <h6 style="margin-top: 1rem;">Number of Accelerators</h6>
            <DoubleRangeSlider
                on:change={({ detail }) => {
                    pendingNumAccelerators = { from: detail[0], to: detail[1] }
                    isAccsModified = true 
                }}
                min={minNumAccelerators} max={maxNumAccelerators}
                firstSlider={pendingNumAccelerators.from} secondSlider={pendingNumAccelerators.to} 
                inputFieldFrom={pendingNumAccelerators.from} inputFieldTo={pendingNumAccelerators.to}/>
        {/if}
    </ModalBody>
    <ModalFooter>
        <Button color="primary"
            disabled={pendingNumNodes.from == null || pendingNumNodes.to == null}
            on:click={() => {
                isOpen = false
                pendingNumNodes = isNodesModified ? pendingNumNodes : { from: null, to: null }
                pendingNumHWThreads = isHwthreadsModified ? pendingNumHWThreads : { from: null, to: null }
                pendingNumAccelerators = isAccsModified ? pendingNumAccelerators : { from: null, to: null }
                numNodes ={ from: pendingNumNodes.from, to: pendingNumNodes.to }
                numHWThreads = { from: pendingNumHWThreads.from, to: pendingNumHWThreads.to }
                numAccelerators = { from: pendingNumAccelerators.from, to: pendingNumAccelerators.to }
                namedNode = pendingNamedNode
                dispatch('update', { numNodes, numHWThreads, numAccelerators, namedNode })
            }}>
            Close & Apply
        </Button>
        <Button color="danger" on:click={() => {
            isOpen = false
            pendingNumNodes = { from: null, to: null }
            pendingNumHWThreads = { from: null, to: null }
            pendingNumAccelerators = { from: null, to: null }
            pendingNamedNode = null
            numNodes = { from: pendingNumNodes.from, to: pendingNumNodes.to }
            numHWThreads = { from: pendingNumHWThreads.from, to: pendingNumHWThreads.to }
            numAccelerators = { from: pendingNumAccelerators.from, to: pendingNumAccelerators.to }
            isNodesModified = false
            isHwthreadsModified = false
            isAccsModified = false
            namedNode = pendingNamedNode
            dispatch('update', { numNodes, numHWThreads, numAccelerators, namedNode})
        }}>Reset</Button>
        <Button on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
</Modal>
