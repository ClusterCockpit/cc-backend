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
    import { gql, getContextClient , mutationStore  } from '@urql/svelte'

    export let metrics
    export let isOpen
    export let configName
    export let allMetrics = null
    export let cluster = null
    export let showFootprint
    export let view = 'job'

    const clusters = getContext('clusters'),
          onInit = getContext('on-init')

    let newMetricsOrder = []
    let unorderedMetrics = [...metrics]
    let pendingShowFootprint = !!showFootprint

    onInit(() => {
        if (allMetrics == null) allMetrics = new Set()
            for (let c of clusters)
                for (let metric of c.metricConfig)
                    allMetrics.add(metric.name)
    })

    $: {
        if (allMetrics != null) {
            if (cluster == null) {
                // console.log('Reset to full metric list')
                for (let c of clusters)
                    for (let metric of c.metricConfig)
                        allMetrics.add(metric.name)
            } else {
                // console.log('Recalculate available metrics for ' + cluster)
                allMetrics.clear()
                for (let c of clusters)
                    if (c.name == cluster)
                        for (let metric of c.metricConfig)
                            allMetrics.add(metric.name)
            }

            newMetricsOrder = [...allMetrics].filter(m => !metrics.includes(m))
            newMetricsOrder.unshift(...metrics.filter(m => allMetrics.has(m)))
            unorderedMetrics = unorderedMetrics.filter(m => allMetrics.has(m))
        }
    }

    const client = getContextClient();
    const updateConfigurationMutation = ({ name, value }) => {
        return mutationStore({
            client: client,
            query: gql`
                mutation($name: String!, $value: String!) {
                    updateConfiguration(name: $name, value: $value)
                }
            `,
            variables: { name, value }
    })}

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

        showFootprint = !!pendingShowFootprint

        updateConfigurationMutation({
            name: cluster == null ? configName : `${configName}:${cluster}`,
            value: JSON.stringify(metrics)
        }).subscribe(res => {
            if (res.fetching === false && res.error) {
                throw res.error
                // console.log('Error on subscription: ' + res.error)
            }
        })

        updateConfigurationMutation({
            name: cluster == null ? 'plot_list_showFootprint' : `plot_list_showFootprint:${cluster}`,
            value: JSON.stringify(showFootprint)
        }).subscribe(res => {
            if (res.fetching === false && res.error) {
                console.log('Error on footprint subscription: ' + res.error)
                throw res.error
            }
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
        Configure columns (Metric availability shown)
    </ModalHeader>
    <ModalBody>
        <ListGroup>
            {#if view === 'list'}
            <li class="list-group-item">
                <input type="checkbox" bind:checked={pendingShowFootprint}> Show Footprint
            </li>
            <hr/>
            {/if}
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
                        {cluster == null ? 
                            clusters // No single cluster specified: List Clusters with Metric
                            .filter(c => c.metricConfig.find(m => m.name == metric) != null)
                            .map(c => c.name).join(', ') : 
                            clusters // Single cluster requested: List Subclusters with do not have metric remove flag
                            .filter(c => c.name == cluster)
                            .filter(c => c.metricConfig.find(m => m.name == metric) != null)
                            .map(function(c) { 
                                let scNames = c.subClusters.map(sc => sc.name)
                                scNames.forEach(function(scName){
                                    let met = c.metricConfig.find(m => m.name == metric)
                                    let msc = met.subClusters.find(msc => msc.name == scName)
                                    if (msc != null) {
                                        if (msc.remove == true) {
                                            scNames = scNames.filter(scn => scn != msc.name)
                                        }
                                    } 
                                })
                                return scNames
                            })
                            .join(', ')}
                    </span>
                </li>
            {/each}
        </ListGroup>
    </ModalBody>
    <ModalFooter>
        <Button color="primary" on:click={closeAndApply}>Close & Apply</Button>
    </ModalFooter>
</Modal>
