<!--
    @component Filter sub-component for selecting cluster and subCluster

    Properties:
    - `disableClusterSelection Bool?`: Is the selection disabled [Default: false]
    - `isModified Bool?`: Is this filter component modified [Default: false]
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `cluster String?`: The currently selected cluster [Default: null]
    - `partition String?`: The currently selected partition (i.e. subCluster) [Default: null]

    Events:
    - `set-filter, {String?, String?}`: Set 'cluster, subCluster' filter in upstream component
 -->

<script>
  import { createEventDispatcher, getContext } from "svelte";
  import {
    Button,
    ListGroup,
    ListGroupItem,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";

  const clusters = getContext("clusters"),
    initialized = getContext("initialized"),
    dispatch = createEventDispatcher();

  export let disableClusterSelection = false;
  export let isModified = false;
  export let isOpen = false;
  export let cluster = null;
  export let partition = null;
  let pendingCluster = cluster,
    pendingPartition = partition;
  $: isModified = pendingCluster != cluster || pendingPartition != partition;
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Cluster & Slurm Partition</ModalHeader>
  <ModalBody>
    {#if $initialized}
      <h4>Cluster</h4>
      <ListGroup>
        <ListGroupItem
          disabled={disableClusterSelection}
          active={pendingCluster == null}
          on:click={() => ((pendingCluster = null), (pendingPartition = null))}
        >
          Any Cluster
        </ListGroupItem>
        {#each clusters as cluster}
          <ListGroupItem
            disabled={disableClusterSelection}
            active={pendingCluster == cluster.name}
            on:click={() => (
              (pendingCluster = cluster.name), (pendingPartition = null)
            )}
          >
            {cluster.name}
          </ListGroupItem>
        {/each}
      </ListGroup>
    {/if}
    {#if $initialized && pendingCluster != null}
      <br />
      <h4>Partiton</h4>
      <ListGroup>
        <ListGroupItem
          active={pendingPartition == null}
          on:click={() => (pendingPartition = null)}
        >
          Any Partition
        </ListGroupItem>
        {#each clusters.find((c) => c.name == pendingCluster).partitions as partition}
          <ListGroupItem
            active={pendingPartition == partition}
            on:click={() => (pendingPartition = partition)}
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
      on:click={() => {
        isOpen = false;
        cluster = pendingCluster;
        partition = pendingPartition;
        dispatch("set-filter", { cluster, partition });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        cluster = pendingCluster = null;
        partition = pendingPartition = null;
        dispatch("set-filter", { cluster, partition });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
