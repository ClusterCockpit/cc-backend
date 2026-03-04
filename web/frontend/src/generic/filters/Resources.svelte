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
    Input,
    Tooltip,
    Icon
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

  const findMaxNumNodes = (infos) =>
    infos.reduce(
      (max, cluster) =>
        Math.max(
          max,
          cluster.subClusters.reduce(
            (max, sc) => Math.max(max, sc.numberOfNodes || 0),
            0,
          ),
        ),
      0,
    );

  const findMaxNumAccels = (infos) =>
    infos.reduce(
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
  const findMaxNumHWThreadsPerNode = (infos) =>
    infos.reduce(
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
  let maxNumNodes = $state(1);
  let maxNumHWThreads = $state(1);
  let maxNumAccelerators = $state(1);

  /* Derived States */
  // Pending
  let pendingNumNodes = $derived({
    from: presetNumNodes.from,
    to: (presetNumNodes.to == 0) ? maxNumNodes : presetNumNodes.to
  });
  let pendingNumHWThreads = $derived({
    from: presetNumHWThreads.from,
    to: (presetNumHWThreads.to == 0) ? maxNumHWThreads : presetNumHWThreads.to
  });
  let pendingNumAccelerators = $derived({
    from: presetNumAccelerators.from,
    to: (presetNumAccelerators.to == 0) ? maxNumAccelerators : presetNumAccelerators.to
  });
  let pendingNamedNode = $derived(presetNamedNode);
  let pendingNodeMatch = $derived(presetNodeMatch);
  // Changable States
  let nodesState = $derived({
    from: presetNumNodes.from,
    to: (presetNumNodes.to == 0) ? maxNumNodes : presetNumNodes.to
  });
  let threadState = $derived({
    from: presetNumHWThreads.from,
    to: (presetNumHWThreads.to == 0) ? maxNumHWThreads : presetNumHWThreads.to
  });
  let accState = $derived({
    from: presetNumAccelerators.from,
    to: (presetNumAccelerators.to == 0) ? maxNumAccelerators : presetNumAccelerators.to
  });

  const initialized = $derived(getContext("initialized") || false);
  const clusterInfos = $derived($initialized ? getContext("clusters") : null);
  // Is Selection Active
  const nodesActive = $derived(!(JSON.stringify(nodesState) === JSON.stringify({ from: 1, to: maxNumNodes })));
  const threadActive = $derived(!(JSON.stringify(threadState) === JSON.stringify({ from: 1, to: maxNumHWThreads })));
  const accActive = $derived(!(JSON.stringify(accState) === JSON.stringify({ from: 1, to: maxNumAccelerators })));
  // Block Apply if null
  const disableApply = $derived(
    nodesState.from === null || nodesState.to === null ||
    threadState.from === null || threadState.to === null ||
    accState.from === null || accState.to === null
  );

  /* Reactive Effects | Svelte 5 onMount */
  $effect(() => {
    if ($initialized) {
      if (activeCluster != null) {
        const { subClusters } = clusterInfos.find((c) => c.name == activeCluster);
        maxNumNodes = findMaxNumNodes([{ subClusters }]);
        maxNumHWThreads = findMaxNumHWThreadsPerNode([{ subClusters }]);
        maxNumAccelerators = findMaxNumAccels([{ subClusters }]);
      } else if (clusterInfos.length > 0) {
        maxNumNodes = findMaxNumNodes(clusterInfos);
        maxNumHWThreads = findMaxNumHWThreadsPerNode(clusterInfos);
        maxNumAccelerators = findMaxNumAccels(clusterInfos);
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
      accState = { from: 1, to: maxNumAccelerators };
    }
  });

  /* Functions */
  function setResources() {
    if (nodesActive) {
      pendingNumNodes = {
        from: nodesState.from,
        to: (nodesState.to == maxNumNodes) ? 0 : nodesState.to
      };
    } else {
      pendingNumNodes = { from: null, to: null};
    };
    if (threadActive) {
      pendingNumHWThreads = {
        from: threadState.from,
        to: (threadState.to == maxNumHWThreads) ? 0 : threadState.to
      };
    } else {
      pendingNumHWThreads = { from: null, to: null};
    };
    if (accActive) {
      pendingNumAccelerators = {
        from: accState.from,
        to: (accState.to == maxNumAccelerators) ? 0 : accState.to
      };
    } else {
      pendingNumAccelerators = { from: null, to: null};
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
      <div class="mb-0"><b>Number of Nodes</b> 
        <Icon id="numnodes-info" style="cursor:help; padding-right: 10px;" size="sm" name="info-circle"/>
      </div>
      <Tooltip target={`numnodes-info`} placement="right">
        Preset maximum is for whole cluster.
      </Tooltip>
      <DoubleRangeSlider
        changeRange={(detail) => {
          nodesState.from = detail[0];
          nodesState.to = detail[1];
        }}
        sliderMin={1}
        sliderMax={maxNumNodes}
        fromPreset={nodesState.from}
        toPreset={nodesState.to}
      />
    </div>

    <div class="mb-3">
      <div class="mb-0">
        <b>Number of HWThreads</b> 
        <Icon id="numthreads-info" style="cursor:help; padding-right: 10px;" size="sm" name="info-circle"/>
      </div>
      <Tooltip target={`numthreads-info`} placement="right">
        Presets for a single node. Can be changed to higher values.
      </Tooltip>
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
        <div class="mb-0">
          <b>Number of Accelerators</b> 
          <Icon id="numaccs-info" style="cursor:help; padding-right: 10px;" size="sm" name="info-circle"/>
        </div>
        <Tooltip target={`numaccs-info`} placement="right">
          Presets for a single node. Can be changed to higher values.
        </Tooltip>
        <DoubleRangeSlider
          changeRange={(detail) => {
            accState.from = detail[0];
            accState.to = detail[1];
          }}
          sliderMin={1}
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
