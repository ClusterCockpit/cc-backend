<!--
    @component Selector component for (footprint) metrics to be displayed as histogram

    Properties:
    - `cluster String`: Currently selected cluster
    - `metricsInHistograms [String]`: The currently selected metrics to display as histogram
    - ìsOpen Bool`: Is selection opened
 -->

<script>
  import { getContext } from "svelte";
  import {
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Button,
    ListGroup,
    ListGroupItem,
  } from "@sveltestrap/sveltestrap";
  import { gql, getContextClient, mutationStore } from "@urql/svelte";

  export let cluster;
  export let metricsInHistograms;
  export let isOpen;

  const client = getContextClient();
  const initialized = getContext("initialized");

  let availableMetrics = []

  function loadHistoMetrics(isInitialized) {
    if (!isInitialized) return;
    const rawAvailableMetrics = getContext("globalMetrics").filter((gm) => gm?.footprint).map((fgm) => { return fgm.name })
    availableMetrics = [...rawAvailableMetrics]
  }

  let pendingMetrics = [...metricsInHistograms]; // Copy

  const updateConfigurationMutation = ({ name, value }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($name: String!, $value: String!) {
          updateConfiguration(name: $name, value: $value)
        }
      `,
      variables: { name, value },
    });
  };

  function updateConfiguration(data) {
    updateConfigurationMutation({
      name: data.name,
      value: JSON.stringify(data.value),
    }).subscribe((res) => {
      if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

  function closeAndApply() {
    metricsInHistograms = [...pendingMetrics]; // Set for parent
    isOpen = !isOpen;
    updateConfiguration({
      name: cluster
        ? `user_view_histogramMetrics:${cluster}`
        : "user_view_histogramMetrics",
      value: metricsInHistograms,
    });
  }

  $: loadHistoMetrics($initialized);

</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select metrics presented in histograms</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#each availableMetrics as metric (metric)}
        <ListGroupItem>
          <input type="checkbox" bind:group={pendingMetrics} value={metric} />
          {metric}
        </ListGroupItem>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button color="primary" on:click={closeAndApply}>Close & Apply</Button>
    <Button color="secondary" on:click={() => (isOpen = !isOpen)}>Close</Button>
  </ModalFooter>
</Modal>
