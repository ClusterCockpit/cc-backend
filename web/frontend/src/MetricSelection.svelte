<!-- 
    @component

    Properties:
    - metrics:    [String] (changes from inside, needs to be initialised, list of selected metrics)
    - isOpen:     Boolean  (can change from inside and outside)
    - configName: String   (constant)
 -->

<script>
    import  { Modal, ModalBody, ModalHeader, ModalFooter, Button, ListGroup } from 'sveltestrap'
    import { getContext } from 'svelte'
    import { mutation } from '@urql/svelte'

    export let metrics
    export let isOpen
    export let configName
    export let allMetrics = null
    export let cluster = null

    const clusters = getContext('clusters'),
          onInit = getContext('on-init')

    let newMetricsOrder = []
    let unorderedMetrics = [...metrics]

    onInit(() => {
        if (allMetrics == null) {
            allMetrics = new Set()
            for (let c of clusters)
                if (cluster == null || c.name == cluster)
                    for (let metric of c.metricConfig)
                        allMetrics.add(metric.name)
        }

        newMetricsOrder = [...allMetrics].filter(m => !metrics.includes(m))
        newMetricsOrder.unshift(...metrics)
    })

    const updateConfiguration = mutation({
        query: `mutation($name: String!, $value: String!) {
            updateConfiguration(name: $name, value: $value)
        }`
    })

    let columnHovering = null

    function columnsDragStart(event, i) {
        event.dataTransfer.effectAllowed = 'move'
        event.dataTransfer.dropEffect = 'move'
        event.dataTransfer.setData('text/plain', i)
    }

    function columnsDrag(event, target) {
        event.dataTransfer.dropEffect = 'move'
        const start = Number.parseInt(event.dataTransfer.getData("text/plain"))
        if (start < target) {
            newMetricsOrder.splice(target + 1, 0, newMetricsOrder[start])
            newMetricsOrder.splice(start, 1)
        } else {
            newMetricsOrder.splice(target, 0, newMetricsOrder[start])
            newMetricsOrder.splice(start + 1, 1)
        }
        columnHovering = null
    }

    function closeAndApply() {
        metrics = newMetricsOrder.filter(m => unorderedMetrics.includes(m))
        isOpen = false

        updateConfiguration({
                name: cluster == null ? configName : `${configName}:${cluster}`,
                value: JSON.stringify(metrics)
            })
            .then(res => {
                if (res.error)
                    console.error(res.error)
            })
    }
</script>

<style>
    li.cc-config-column {
        display: block;
        cursor: grab;
    }

    li.cc-config-column.is-active {
        background-color: #3273dc;
        color: #fff;
        cursor: grabbing;
    }
</style>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Configure columns
    </ModalHeader>
    <ModalBody>
        <ListGroup>
            {#each newMetricsOrder as metric, index (metric)}
                <li class="cc-config-column list-group-item"
                    draggable={true} ondragover="return false"
                    on:dragstart={event => columnsDragStart(event, index)}
                    on:drop|preventDefault={event => columnsDrag(event, index)}
                    on:dragenter={() => columnHovering = index}
                    class:is-active={columnHovering === index}>
                    {#if unorderedMetrics.includes(metric)}
                        <input type="checkbox" bind:group={unorderedMetrics} value={metric} checked>
                    {:else}
                        <input type="checkbox" bind:group={unorderedMetrics} value={metric}>
                    {/if}
                    {metric}
                    <span style="float: right;">
                        {cluster == null ? clusters
                            .filter(cluster => cluster.metricConfig.find(m => m.name == metric) != null)
                            .map(cluster => cluster.name).join(', ') : ''}
                    </span>
                </li>
            {/each}
        </ListGroup>
    </ModalBody>
    <ModalFooter>
        <Button color="primary" on:click={closeAndApply}>Close & Apply</Button>
    </ModalFooter>
</Modal>
