<script>
    import { Modal, ModalBody, ModalHeader, ModalFooter,
             Button, ListGroup, ListGroupItem, Icon } from 'sveltestrap'
    import { gql, getContextClient , mutationStore } from '@urql/svelte'

    export let cluster
    export let availableMetrics
    export let metricsInHistograms

    const client = getContextClient();
    const updateConfigurationMutation = ({ name, value }) => {
        return mutationStore({
            client: client,
            query: gql`mutation($name: String!, $value: String!) {
                updateConfiguration(name: $name, value: $value)
            }`,
            variables: { name, value }
        })
    }

    let isHistogramConfigOpen = false

    function updateConfiguration(data) {
        updateConfigurationMutation({
                name: data.name,
                value: JSON.stringify(data.value)
        }).subscribe(res => {
            if (res.fetching === false && res.error) {
                throw res.error
                // console.log('Error on subscription: ' + res.error)
            }
        })
    }
</script>

<Button outline
    on:click={() => (isHistogramConfigOpen = true)}>
    <Icon name=""/>
    Select Histograms
</Button>

<Modal isOpen={isHistogramConfigOpen}
    toggle={() => (isHistogramConfigOpen = !isHistogramConfigOpen)}>
    <ModalHeader>
        Select metrics presented in histograms
    </ModalHeader>
    <ModalBody>
        <ListGroup>
            <!-- <li class="list-group-item">
                <input type="checkbox" bind:checked={pendingShowFootprint}> Show Footprint
            </li>
            <hr/> -->
            {#each availableMetrics as metric (metric)}
                <ListGroupItem>
                    <input type="checkbox" bind:group={metricsInHistograms}
                        value={metric}
                        on:change={() => updateConfiguration({
                            name: cluster ? `user_view_histogramMetrics:${cluster}` : 'user_view_histogramMetrics',
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
