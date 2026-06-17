<!--
  @component Filter sub-component for selecting cluster, partition and subCluster

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `presetCluster String?`: The latest selected cluster [Default: ""]
  - `presetPartition String?`: The latest selected partition [Default: ""]
  - `presetSubCluster String?`: The latest selected subCluster [Default: ""]
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
    presetSubCluster = "",
    disableClusterSelection = false,
    setFilter
  } = $props();


  /* Derived */
  const initialized = $derived(getContext("initialized") || false);
  const clusterInfos = $derived($initialized ? getContext("clusters") : null);
  let pendingCluster = $derived(presetCluster);
  let pendingPartition = $derived(presetPartition);
  let pendingSubCluster = $derived(presetSubCluster);
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Cluster, SubCluster & Partition</ModalHeader>
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
            onclick={() => ((pendingCluster = null), (pendingPartition = null), (pendingSubCluster = null))}
          >
            Any Cluster
          </ListGroupItem>
          {#each clusterInfos as cluster}
            <ListGroupItem
              disabled={disableClusterSelection}
              active={pendingCluster == cluster.name}
              onclick={() => (
                (pendingCluster = cluster.name), (pendingPartition = null), (pendingSubCluster = null)
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
      <h4>SubCluster</h4>
      <ListGroup>
        <ListGroupItem
          active={pendingSubCluster == null}
          onclick={() => (pendingSubCluster = null)}
        >
          Any SubCluster
        </ListGroupItem>
        {#each clusterInfos?.find((c) => c.name == pendingCluster)?.subClusters as subCluster}
          <ListGroupItem
            active={pendingSubCluster == subCluster.name}
            onclick={() => (pendingSubCluster = subCluster.name)}
          >
            {subCluster.name}
          </ListGroupItem>
        {/each}
      </ListGroup>
    {/if}
    {#if $initialized && pendingCluster != null}
      <br />
      <h4>Partition</h4>
      <ListGroup>
        <ListGroupItem
          active={pendingPartition == null}
          onclick={() => (pendingPartition = null)}
        >
          Any Partition
        </ListGroupItem>
        {#each clusterInfos?.find((c) => c.name == pendingCluster)?.partitions as partition}
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
        setFilter({ cluster: pendingCluster, subCluster: pendingSubCluster, partition: pendingPartition });
      }}>Close & Apply</Button
    >
    {#if !disableClusterSelection}
      <Button
        color="danger"
        onclick={() => {
          isOpen = false;
          pendingCluster = null;
          pendingPartition = null;
          pendingSubCluster = null;
          setFilter({ cluster: pendingCluster, subCluster: pendingSubCluster, partition: pendingPartition })
        }}>Reset</Button
      >
    {/if}
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
