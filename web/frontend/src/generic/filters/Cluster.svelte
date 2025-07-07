<!--
  @component Filter sub-component for selecting cluster and subCluster

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `presetCluster String?`: The latest selected cluster [Default: ""]
  - `presetPartition String?`: The latest selected partition [Default: ""]
  - `disableClusterSelection Bool?`: Is the selection disabled [Default: false]
  - `setFilter Func`: The callback function to apply current filter selection
-->

<script>
  import { getContext } from "svelte";
  import {
    Button,
    ListGroup,
    ListGroupItem,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetCluster = "",
    presetPartition = "",
    disableClusterSelection = false,
    setFilter
  } = $props();

  /* Const Init */
  const clusters = getContext("clusters");
  const initialized = getContext("initialized");

  /* State Init */
  let pendingCluster = $state(presetCluster);
  let pendingPartition = $state(presetPartition);
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Cluster & Slurm Partition</ModalHeader>
  <ModalBody>
    {#if $initialized}
      <h4>Cluster</h4>
      {#if disableClusterSelection}
        <Button color="info" class="w-100 mb-2" disabled><b>Info: Cluster Selection Disabled in This View</b></Button>
        <Button outline color="primary" class="w-100 mb-2" disabled><b>Selected Cluster: {presetCluster}</b></Button>
      {:else}
        <ListGroup>
          <ListGroupItem
            disabled={disableClusterSelection}
            active={pendingCluster == null}
            onclick={() => ((pendingCluster = null), (pendingPartition = null))}
          >
            Any Cluster
          </ListGroupItem>
          {#each clusters as cluster}
            <ListGroupItem
              disabled={disableClusterSelection}
              active={pendingCluster == cluster.name}
              onclick={() => (
                (pendingCluster = cluster.name), (pendingPartition = null)
              )}
            >
              {cluster.name}
            </ListGroupItem>
          {/each}
        </ListGroup>
      {/if}
    {/if}
    {#if $initialized && pendingCluster != null}
      <br />
      <h4>Partiton</h4>
      <ListGroup>
        <ListGroupItem
          active={pendingPartition == null}
          onclick={() => (pendingPartition = null)}
        >
          Any Partition
        </ListGroupItem>
        {#each clusters?.find((c) => c.name == pendingCluster)?.partitions as partition}
          <ListGroupItem
            active={pendingPartition == partition}
            onclick={() => (pendingPartition = partition)}
          >
            {partition}
          </ListGroupItem>
        {/each}
      </ListGroup>
    {/if}
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      onclick={() => {
        isOpen = false;
        setFilter({ cluster: pendingCluster, partition: pendingPartition });
      }}>Close & Apply</Button
    >
    {#if !disableClusterSelection}
      <Button
        color="danger"
        onclick={() => {
          isOpen = false;
          pendingCluster = null;
          pendingPartition = null;
          setFilter({ cluster: pendingCluster, partition: pendingPartition})
        }}>Reset</Button
      >
    {/if}
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
