<script>
    import { Modal, ModalBody, ModalHeader, ModalFooter, InputGroup,
             Button, ListGroup, ListGroupItem, Icon } from 'sveltestrap'
    import { gql, getContextClient , mutationStore } from '@urql/svelte'

    export let availableMetrics
    export let metricsInHistograms
    export let metricsInScatterplots

    const updateConfigurationMutation = ({ name, value }) => {
    result = mutationStore({
        client: getContextClient(),
        query: gql`mutation($name: String!, $value: String!) {
            updateConfiguration(name: $name, value: $value)
        }`,
        variables: { name, value }
    })
    }

    let isHistogramConfigOpen = false, isScatterPlotConfigOpen = false
    let selectedMetric1 = null, selectedMetric2 = null

    function updateConfiguration(data) {
        updateConfigurationMutation({
                name: data.name,
                value: JSON.stringify(data.value)
            })
            .then(res => {
                if (res.error)
                    console.error(res.error)
            });
    }
</script>

<Button outline
    on:click={() => (isHistogramConfigOpen = true)}>
    <Icon name=""/>
    Select Plots for Histograms
</Button>

<Button outline
    on:click={() => (isScatterPlotConfigOpen = true)}>
    <Icon name=""/>
    Select Plots in Scatter Plots
</Button>

<Modal isOpen={isHistogramConfigOpen}
    toggle={() => (isHistogramConfigOpen = !isHistogramConfigOpen)}>
    <ModalHeader>
        Select metrics presented in histograms
    </ModalHeader>
    <ModalBody>
        <ListGroup>
            {#each availableMetrics as metric (metric)}
                <ListGroupItem>
                    <input type="checkbox" bind:group={metricsInHistograms}
                        value={metric}
                        on:change={() => updateConfiguration({
                            name: 'analysis_view_histogramMetrics',
                            value: metricsInHistograms
                        })} />

                    {metric}
                </ListGroupItem>
            {/each}
        </ListGroup>
    </ModalBody>
    <ModalFooter>
        <Button color="primary"
            on:click={() => (isHistogramConfigOpen = false)}>
            Close
        </Button>
    </ModalFooter>
</Modal>

<Modal isOpen={isScatterPlotConfigOpen}
    toggle={() => (isScatterPlotConfigOpen = !isScatterPlotConfigOpen)}>
    <ModalHeader>
        Select metric pairs presented in scatter plots
    </ModalHeader>
    <ModalBody>
        <ListGroup>
            {#each metricsInScatterplots as pair}
                <ListGroupItem>
                    <b>{pair[0]}</b> / <b>{pair[1]}</b>

                    <Button style="float: right;" outline color="danger"
                        on:click={() => {
                            metricsInScatterplots = metricsInScatterplots.filter(p => pair != p)
                            updateConfiguration({
                                name: 'analysis_view_scatterPlotMetrics',
                                value: metricsInScatterplots
                            });
                        }}>
                        <Icon name="x" />
                    </Button>
                </ListGroupItem>
            {/each}
        </ListGroup>

        <br/>

        <InputGroup>
            <select bind:value={selectedMetric1} class="form-group form-select">
                <option value={null}>Choose Metric for X Axis</option>
                {#each availableMetrics as metric}
                    <option value={metric}>{metric}</option>
                {/each}
            </select>
            <select bind:value={selectedMetric2} class="form-group form-select">
                <option value={null}>Choose Metric for Y Axis</option>
                {#each availableMetrics as metric}
                    <option value={metric}>{metric}</option>
                {/each}
            </select>
            <Button outline disabled={selectedMetric1 == null || selectedMetric2 == null}
                on:click={() => {
                    metricsInScatterplots = [...metricsInScatterplots, [selectedMetric1, selectedMetric2]]
                    selectedMetric1 = null
                    selectedMetric2 = null
                    updateConfiguration({
                        name: 'analysis_view_scatterPlotMetrics',
                        value: metricsInScatterplots
                    })
                }}>
                Add Plot
            </Button>
        </InputGroup>

    </ModalBody>
    <ModalFooter>
        <Button color="primary"
            on:click={() => (isScatterPlotConfigOpen = false)}>
            Close
        </Button>
    </ModalFooter>
</Modal>
