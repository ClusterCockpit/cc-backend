<!--
  @component Filter sub-component for selecting job resources

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `activeCluster String?`: The currently selected cluster name [Default: null]
  - `presetNumNodes Object?`: The currently selected numNodes filter [Default: {from:null, to:null}]
  - `presetNumHWThreads Object?`: The currently selected numHWThreads filter [Default: {from:null, to:null}]
  - `presetNumAccelerators Object?`: The currently selected numAccelerators filter [Default: {from:null, to:null}]
  - `presetNamedNode String?`: The currently selected single named node (= hostname) [Default: null]
  - `presetNodeMatch String?`: The currently selected single named node (= hostname) [Default: "eq"]
  - `setFilter Func`: The callback function to apply current filter selection
-->
 
 <script>
  import { getContext } from "svelte";
  import {
    Button,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Input
  } from "@sveltestrap/sveltestrap";
  import DoubleRangeSlider from "../select/DoubleRangeSlider.svelte";

  /* Svelte 5 Props*/
  let {
    isOpen = $bindable(false),
    activeCluster = null,
    presetNumNodes = { from: null, to: null },
    presetNumHWThreads = { from: null, to: null },
    presetNumAccelerators = { from: null, to: null },
    presetNamedNode = null,
    presetNodeMatch = "eq",
    setFilter
  } = $props()

  /* Const Init */
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

  /* State Init*/
  // Counts
  let minNumNodes = $state(1);
  let maxNumNodes = $state(0);
  let maxNumHWThreads = $state(0);
  let maxNumAccelerators = $state(0);
  // Pending
  let pendingNumNodes = $state(presetNumNodes);
  let pendingNumHWThreads = $state(presetNumHWThreads);
  let pendingNumAccelerators = $state(presetNumAccelerators);
  let pendingNamedNode = $state(presetNamedNode);
  let pendingNodeMatch = $state(presetNodeMatch);
  // Changable States
  let nodesState = $state(presetNumNodes);
  let threadState = $state(presetNumHWThreads);
  let accState = $state(presetNumAccelerators);

  /* Derived States */
  const clusters = $derived(getContext("clusters"));
  const initialized = $derived(getContext("initialized"));
  // Is Selection Active
  const nodesActive = $derived(!(JSON.stringify(nodesState) === JSON.stringify({ from: 1, to: maxNumNodes })));
  const threadActive = $derived(!(JSON.stringify(threadState) === JSON.stringify({ from: 1, to: maxNumHWThreads })));
  const accActive = $derived(!(JSON.stringify(accState) === JSON.stringify({ from: 0, to: maxNumAccelerators })));
  // Block Apply if null
  const disableApply = $derived(
    nodesState.from === null || nodesState.to === null ||
    threadState.from === null || threadState.to === null ||
    accState.from === null || accState.to === null
  );

  /* Reactive Effects | Svelte 5 onMount */
  $effect(() => {
    if ($initialized) {
      // 'hClusters' defined in templates/base.tmpl
      if (activeCluster != null) {
        const { filterRanges } = hClusters.find((c) => c.name == activeCluster);
        minNumNodes = filterRanges.numNodes.from;
        maxNumNodes = filterRanges.numNodes.to;
      } else if (clusters.length > 0) {
        for (let hc of hClusters) {
          const { filterRanges } = hc;
          minNumNodes = Math.min(minNumNodes, filterRanges.numNodes.from);
          maxNumNodes = Math.max(maxNumNodes, filterRanges.numNodes.to);
        };
      };
    };
  });
  
  $effect(() => {
    if ($initialized) {
      // 'hClusters' defined in templates/base.tmpl
      if (activeCluster != null) {
        const { subClusters } = clusters.find((c) => c.name == activeCluster);
        maxNumAccelerators = findMaxNumAccels([{ subClusters }]);
        maxNumHWThreads = findMaxNumHWThreadsPerNode([{ subClusters }]);
      } else if (clusters.length > 0) {
        maxNumAccelerators = findMaxNumAccels(clusters);
        maxNumHWThreads = findMaxNumHWThreadsPerNode(clusters);
      }
    }
  });

  $effect(() => {
    if (
      $initialized &&
      pendingNumNodes.from == null &&
      pendingNumNodes.to == null
    ) {
      nodesState = { from: 1, to: maxNumNodes };
    }
  });

  $effect(() => {
    if (
      $initialized &&
      pendingNumHWThreads.from == null && 
      pendingNumHWThreads.to == null 
    ) {
      threadState = { from: 1, to: maxNumHWThreads };
    }
  });

  $effect(() => {
    if (
      $initialized &&
      pendingNumAccelerators.from == null &&
      pendingNumAccelerators.to == null
    ) {
      accState = { from: 0, to: maxNumAccelerators };
    }
  });

  /* Functions */
  function setResources() {
    if (nodesActive) {
      pendingNumNodes = {...nodesState};
    } else {
      pendingNumNodes = { from: null, to: null };
    };
    if (threadActive) {
      pendingNumHWThreads = {...threadState};
    } else {
      pendingNumHWThreads = { from: null, to: null };
    };
    if (accActive) {
      pendingNumAccelerators = {...accState};
    } else {
      pendingNumAccelerators = { from: null, to: null };
    };
  };

  function resetResources() {
    pendingNumNodes = { from: null, to: null };
    pendingNumHWThreads = { from: null, to: null };
    pendingNumAccelerators = { from: null, to: null };
    pendingNamedNode = null;
    pendingNodeMatch = "eq";
  };

</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select number of utilized Resources</ModalHeader>
  <ModalBody>
    <div><b>Named Node</b></div>
    <div class="d-flex mb-3">
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

    <div class="mb-3">
      <div class="mb-0"><b>Number of Nodes</b></div>
      <DoubleRangeSlider
        changeRange={(detail) => {
          nodesState.from = detail[0];
          nodesState.to = detail[1];
        }}
        sliderMin={minNumNodes}
        sliderMax={maxNumNodes}
        fromPreset={nodesState.from}
        toPreset={nodesState.to}
      />
    </div>

    <div class="mb-3">
      <div class="mb-0"><b>Number of HWThreads</b> (Use for Single-Node Jobs)</div>
      <DoubleRangeSlider
        changeRange={(detail) => {
          threadState.from = detail[0];
          threadState.to = detail[1];
        }}
        sliderMin={1}
        sliderMax={maxNumHWThreads}
        fromPreset={threadState.from}
        toPreset={threadState.to}
      />
    </div>
    {#if maxNumAccelerators != null && maxNumAccelerators > 1}
      <div>
        <div class="mb-0"><b>Number of Accelerators</b></div>
        <DoubleRangeSlider
          changeRange={(detail) => {
            accState.from = detail[0];
            accState.to = detail[1];
          }}
          sliderMin={0}
          sliderMax={maxNumAccelerators}
          fromPreset={accState.from}
          toPreset={accState.to}
        />
      </div>
    {/if}
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      disabled={disableApply}
      onclick={() => {
        isOpen = false;
        setResources();
        setFilter({
          numNodes: pendingNumNodes,
          numHWThreads: pendingNumHWThreads,
          numAccelerators: pendingNumAccelerators,
          node: pendingNamedNode,
          nodeMatch: pendingNodeMatch
        });
      }}
    >
      Close & Apply
    </Button>
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        resetResources();
        setFilter({
          numNodes: pendingNumNodes,
          numHWThreads: pendingNumHWThreads,
          numAccelerators: pendingNumAccelerators,
          node: pendingNamedNode,
          nodeMatch: pendingNodeMatch
        });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
