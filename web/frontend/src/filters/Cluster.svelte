<script>
    import { createEventDispatcher, getContext } from 'svelte'
    import { Button, ListGroup, ListGroupItem,
             Modal, ModalBody, ModalHeader, ModalFooter } from 'sveltestrap'

    const clusters = getContext('clusters'),
          initialized = getContext('initialized'),
          dispatch = createEventDispatcher()

    export let disableClusterSelection = false
    export let isModified = false
    export let isOpen = false
    export let cluster = null
    export let partition = null
    let pendingCluster = cluster, pendingPartition = partition
    $: isModified = pendingCluster != cluster || pendingPartition != partition
</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Select Cluster & Slurm Partition
    </ModalHeader>
    <ModalBody>
        {#if $initialized}
            <h4>Cluster</h4>
            <ListGroup>
                <ListGroupItem
                    disabled={disableClusterSelection}
                    active={pendingCluster == null}
                    on:click={() => (pendingCluster = null, pendingPartition = null)}>
                    Any Cluster
                </ListGroupItem>
                {#each clusters as cluster}
                    <ListGroupItem
                        disabled={disableClusterSelection}
                        active={pendingCluster == cluster.name}
                        on:click={() => (pendingCluster = cluster.name, pendingPartition = null)}>
                        {cluster.name}
                    </ListGroupItem>
                {/each}
            </ListGroup>        
        {/if}
        {#if $initialized && pendingCluster != null}
            <br/>
            <h4>Partiton</h4>
            <ListGroup>
                <ListGroupItem
                    active={pendingPartition == null}
                    on:click={() => (pendingPartition = null)}>
                    Any Partition
                </ListGroupItem>
                {#each clusters.find(c => c.name == pendingCluster).partitions as partition}
                    <ListGroupItem
                        active={pendingPartition == partition}
                        on:click={() => (pendingPartition = partition)}>
                        {partition}
                    </ListGroupItem>
                {/each}
            </ListGroup>
        {/if}
    </ModalBody>
    <ModalFooter>
        <Button color="primary" on:click={() => {
            isOpen = false
            cluster = pendingCluster
            partition = pendingPartition
            dispatch('update', { cluster, partition })
        }}>Close & Apply</Button>
        <Button color="danger" on:click={() => {
            isOpen = false
            cluster = pendingCluster = null
            partition = pendingPartition = null
            dispatch('update', { cluster, partition })
        }}>Reset</Button>
        <Button on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
</Modal>
