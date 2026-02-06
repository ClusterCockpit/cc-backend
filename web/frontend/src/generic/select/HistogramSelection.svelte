<!--
  @component Selector component for (footprint) metrics to be displayed as histogram

  Properties:
  - `cluster String`: Currently selected cluster
  - `ìsOpen Bool`: Is selection opened [Bindable]
  - `configName String`: The config id string to be updated in database on selection change
  - `presetSelectedHistograms [String]`: The currently selected metrics to display as histogram
  - `globalMetrics [Obj]`: Includes the backend supplied availabilities for cluster and subCluster
  - `applyChange Func`: The callback function to apply current selection
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

  /* Svelte 5 Props */
  let {
    cluster = "",
    isOpen = $bindable(),
    configName,
    presetSelectedHistograms,
    globalMetrics,
    applyChange
  } = $props();

  /* Const Init */
  const client = getContextClient();

  /* Derived */
  let selectedHistograms = $derived(presetSelectedHistograms); // Non-Const Derived: Is settable
  const availableMetrics = $derived(loadHistoMetrics(cluster));

  /* Functions */
  function loadHistoMetrics(thisCluster) {
    // isInit Check Removed: Parent Component has finished Init-Query: Globalmetrics available here.
    if (!thisCluster) {
      return globalMetrics
      .filter((gm) => gm?.footprint)
      .map((fgm) => { return fgm.name })
    } else {
      return globalMetrics
      .filter((gm) => gm?.availability.find((av) => av.cluster == thisCluster))
      .filter((agm) => agm?.footprint)
      .map((afgm) => { return afgm.name })
    }
  }

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
    isOpen = !isOpen;
    applyChange(selectedHistograms)
    updateConfiguration({
      name: cluster
        ? `${configName}:${cluster}`
        : configName,
      value: selectedHistograms,
    });
  }

  /* Mutation */
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
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select metrics presented in histograms</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#each availableMetrics as metric (metric)}
        <ListGroupItem>
          <input type="checkbox" bind:group={selectedHistograms} value={metric} />
          {metric}
        </ListGroupItem>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button color="primary" onclick={() => closeAndApply()}>Close & Apply</Button>
    <Button color="secondary" onclick={() => (isOpen = !isOpen)}>Close</Button>
  </ModalFooter>
</Modal>
