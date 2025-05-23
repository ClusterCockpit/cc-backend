<!--
    @component Filter sub-component for selecting job resources

    Properties:
    - `cluster Object?`: The currently selected cluster config [Default: null]
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `numNodes Object?`: The currently selected numNodes filter [Default: {from:null, to:null}]
    - `numHWThreads Object?`: The currently selected numHWThreads filter [Default: {from:null, to:null}]
    - `numAccelerators Object?`: The currently selected numAccelerators filter [Default: {from:null, to:null}]
    - `isNodesModified Bool?`: Is the node filter modified [Default: false]
    - `isHwthreadsModified Bool?`: Is the Hwthreads filter modified [Default: false]
    - `isAccsModified Bool?`: Is the Accelerator filter modified [Default: false]
    - `namedNode String?`: The currently selected single named node (= hostname) [Default: null]

    Events:
    - `set-filter, {Object, Object, Object, String}`: Set 'numNodes, numHWThreads, numAccelerators, namedNode' filter in upstream component
 -->
 
 <script>
  import { createEventDispatcher, getContext } from "svelte";
  import {
    Button,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Input
  } from "@sveltestrap/sveltestrap";
  import DoubleRangeSlider from "../select/DoubleRangeSlider.svelte";

  const clusters = getContext("clusters"),
    initialized = getContext("initialized"),
    dispatch = createEventDispatcher();

  export let cluster = null;
  export let isOpen = false;
  export let numNodes = { from: null, to: null };
  export let numHWThreads = { from: null, to: null };
  export let numAccelerators = { from: null, to: null };
  export let isNodesModified = false;
  export let isHwthreadsModified = false;
  export let isAccsModified = false;
  export let namedNode = null;
  export let nodeMatch = "eq"

  let pendingNumNodes = numNodes,
    pendingNumHWThreads = numHWThreads,
    pendingNumAccelerators = numAccelerators,
    pendingNamedNode = namedNode,
    pendingNodeMatch = nodeMatch;

  const nodeMatchLabels = {
    eq: "Equal To",
    contains: "Contains",
  }

  const findMaxNumAccels = (clusters) =>
    clusters.reduce(
      (max, cluster) =>
        Math.max(
          max,
          cluster.subClusters.reduce(
            (max, sc) => Math.max(max, sc.topology.accelerators?.length || 0),
            0,
          ),
        ),
      0,
    );

  // Limited to Single-Node Thread Count
  const findMaxNumHWThreadsPerNode = (clusters) =>
    clusters.reduce(
      (max, cluster) =>
        Math.max(
          max,
          cluster.subClusters.reduce(
            (max, sc) =>
              Math.max(
                max,
                sc.threadsPerCore * sc.coresPerSocket * sc.socketsPerNode || 0,
              ),
            0,
          ),
        ),
      0,
    );

  let minNumNodes = 1,
    maxNumNodes = 0,
    minNumHWThreads = 1,
    maxNumHWThreads = 0,
    minNumAccelerators = 0,
    maxNumAccelerators = 0;
  $: {
    if ($initialized) {
      if (cluster != null) {
        const { subClusters } = clusters.find((c) => c.name == cluster);
        const { filterRanges } = header.clusters.find((c) => c.name == cluster);
        minNumNodes = filterRanges.numNodes.from;
        maxNumNodes = filterRanges.numNodes.to;
        maxNumAccelerators = findMaxNumAccels([{ subClusters }]);
        maxNumHWThreads = findMaxNumHWThreadsPerNode([{ subClusters }]);
      } else if (clusters.length > 0) {
        const { filterRanges } = header.clusters[0];
        minNumNodes = filterRanges.numNodes.from;
        maxNumNodes = filterRanges.numNodes.to;
        maxNumAccelerators = findMaxNumAccels(clusters);
        maxNumHWThreads = findMaxNumHWThreadsPerNode(clusters);
        for (let cluster of header.clusters) {
          const { filterRanges } = cluster;
          minNumNodes = Math.min(minNumNodes, filterRanges.numNodes.from);
          maxNumNodes = Math.max(maxNumNodes, filterRanges.numNodes.to);
        }
      }
    }
  }

  $: {
    if (
      isOpen &&
      $initialized &&
      pendingNumNodes.from == null &&
      pendingNumNodes.to == null
    ) {
      pendingNumNodes = { from: 0, to: maxNumNodes };
    }
  }

  $: {
    if (
      isOpen &&
      $initialized &&
      ((pendingNumHWThreads.from == null && pendingNumHWThreads.to == null) ||
        isHwthreadsModified == false)
    ) {
      pendingNumHWThreads = { from: 0, to: maxNumHWThreads };
    }
  }

  $: if (maxNumAccelerators != null && maxNumAccelerators > 1) {
    if (
      isOpen &&
      $initialized &&
      pendingNumAccelerators.from == null &&
      pendingNumAccelerators.to == null
    ) {
      pendingNumAccelerators = { from: 0, to: maxNumAccelerators };
    }
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select number of utilized Resources</ModalHeader>
  <ModalBody>
    <h6>Named Node</h6>
    <div class="d-flex">
      <Input type="text" class="w-75" bind:value={pendingNamedNode} />
      <div class="mx-1"></div>
      <Input type="select" class="w-25" bind:value={pendingNodeMatch}>
        {#each Object.entries(nodeMatchLabels) as [nodeMatchKey, nodeMatchLabel]}
          <option value={nodeMatchKey}>
            {nodeMatchLabel}
          </option>
        {/each}
        </Input>
    </div>
    <h6 style="margin-top: 1rem;">Number of Nodes</h6>
    <DoubleRangeSlider
      on:change={({ detail }) => {
        pendingNumNodes = { from: detail[0], to: detail[1] };
        isNodesModified = true;
      }}
      min={minNumNodes}
      max={maxNumNodes}
      firstSlider={pendingNumNodes.from}
      secondSlider={pendingNumNodes.to}
      inputFieldFrom={pendingNumNodes.from}
      inputFieldTo={pendingNumNodes.to}
    />
    <h6 style="margin-top: 1rem;">
      Number of HWThreads (Use for Single-Node Jobs)
    </h6>
    <DoubleRangeSlider
      on:change={({ detail }) => {
        pendingNumHWThreads = { from: detail[0], to: detail[1] };
        isHwthreadsModified = true;
      }}
      min={minNumHWThreads}
      max={maxNumHWThreads}
      firstSlider={pendingNumHWThreads.from}
      secondSlider={pendingNumHWThreads.to}
      inputFieldFrom={pendingNumHWThreads.from}
      inputFieldTo={pendingNumHWThreads.to}
    />
    {#if maxNumAccelerators != null && maxNumAccelerators > 1}
      <h6 style="margin-top: 1rem;">Number of Accelerators</h6>
      <DoubleRangeSlider
        on:change={({ detail }) => {
          pendingNumAccelerators = { from: detail[0], to: detail[1] };
          isAccsModified = true;
        }}
        min={minNumAccelerators}
        max={maxNumAccelerators}
        firstSlider={pendingNumAccelerators.from}
        secondSlider={pendingNumAccelerators.to}
        inputFieldFrom={pendingNumAccelerators.from}
        inputFieldTo={pendingNumAccelerators.to}
      />
    {/if}
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      disabled={pendingNumNodes.from == null || pendingNumNodes.to == null}
      on:click={() => {
        isOpen = false;
        pendingNumNodes = isNodesModified
          ? pendingNumNodes
          : { from: null, to: null };
        pendingNumHWThreads = isHwthreadsModified
          ? pendingNumHWThreads
          : { from: null, to: null };
        pendingNumAccelerators = isAccsModified
          ? pendingNumAccelerators
          : { from: null, to: null };
        numNodes = { from: pendingNumNodes.from, to: pendingNumNodes.to };
        numHWThreads = {
          from: pendingNumHWThreads.from,
          to: pendingNumHWThreads.to,
        };
        numAccelerators = {
          from: pendingNumAccelerators.from,
          to: pendingNumAccelerators.to,
        };
        namedNode = pendingNamedNode;
        nodeMatch = pendingNodeMatch;
        dispatch("set-filter", {
          numNodes,
          numHWThreads,
          numAccelerators,
          namedNode,
          nodeMatch
        });
      }}
    >
      Close & Apply
    </Button>
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        pendingNumNodes = { from: null, to: null };
        pendingNumHWThreads = { from: null, to: null };
        pendingNumAccelerators = { from: null, to: null };
        pendingNamedNode = null;
        pendingNodeMatch = null;
        numNodes = { from: pendingNumNodes.from, to: pendingNumNodes.to };
        numHWThreads = {
          from: pendingNumHWThreads.from,
          to: pendingNumHWThreads.to,
        };
        numAccelerators = {
          from: pendingNumAccelerators.from,
          to: pendingNumAccelerators.to,
        };
        isNodesModified = false;
        isHwthreadsModified = false;
        isAccsModified = false;
        namedNode = pendingNamedNode;
        nodeMatch = pendingNodeMatch;
        dispatch("set-filter", {
          numNodes,
          numHWThreads,
          numAccelerators,
          namedNode,
          nodeMatch
        });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
