<!-- 
  @component Metric selector component; allows reorder via drag and drop

  Properties:
  - `isOpen Bool`: Is selection modal opened [Bindable, Default: false]
  - `showFootprint Bool?`: Upstream state of whether to render footprint card [Bindable, Default: false]
  - `totalMetrics Number?`: Total available metrics [Bindable, Default: 0]
  - `presetMetrics [String]`: Latest selection of metrics [Default: []]
  - `cluster String?`: The currently selected cluster [Default: null]
  - `subCluster String?`: The currently selected subCluster [Default: null]
  - `footprintSelect Bool?`: Render checkbox for footprint display in upstream component [Default: false]
  - `preInitialized Bool?`: If the parent component has a dedicated call to init() [Default: false]
  - `configName String`: The config key for the last saved selection (constant)
  - `applyMetrics Func`: The callback function to apply current selection
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
  } from "@sveltestrap/sveltestrap";
  import { gql, getContextClient, mutationStore } from "@urql/svelte";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    showFootprint = $bindable(false),
    totalMetrics = $bindable(0),
    presetMetrics = [],
    cluster = null,
    subCluster = null,
    footprintSelect = false,
    preInitialized = false, // Job View is Pre-Init'd: $initialized "alone" store returns false
    configName,
    applyMetrics
  } = $props();

  /* Const Init */
  const globalMetrics = getContext("globalMetrics");
  const initialized = getContext("initialized");
  const client = getContextClient();
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

  /* State Init */
  let pendingShowFootprint = $state(!!showFootprint);
  let listedMetrics = $state([]);
  let columnHovering = $state(null);

  /* Derives States */
  let pendingMetrics = $derived(presetMetrics);
  const allMetrics = $derived(loadAvailable(preInitialized || $initialized));

  /* Reactive Effects */
  $effect(() => {
    totalMetrics = allMetrics?.size || 0;
  });

  $effect(() => {
    listedMetrics = [...presetMetrics, ...allMetrics.difference(new Set(presetMetrics))]; // List (preset) active metrics first, then list inactives
  });

  /* Functions */
  function loadAvailable(init) {
    const availableMetrics = new Set();
    if (init) {
      for (let gm of globalMetrics) {
        if (!cluster) {
          availableMetrics.add(gm.name)
        } else {
          if (!subCluster) {
            if (gm.availability.find((av) => av.cluster === cluster)) availableMetrics.add(gm.name);
          } else {
            if (gm.availability.find((av) => av.cluster === cluster && av.subClusters.includes(subCluster))) availableMetrics.add(gm.name);
          }
        }
      }
    }
    return availableMetrics
  }

  function printAvailability(metric, cluster) {
    const avail = globalMetrics.find((gm) => gm.name === metric)?.availability
    if (!cluster) {
      return avail.map((av) => av.cluster).join(', ')
    } else {
      return avail.find((av) => av.cluster === cluster).subClusters.join(', ')
    }
  }

  function columnsDragStart(event, i) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.dropEffect = "move";
    event.dataTransfer.setData("text/plain", i);
  }

  function columnsDrag(event, target) {
    event.dataTransfer.dropEffect = "move";
    const start = Number.parseInt(event.dataTransfer.getData("text/plain"));

    let pendingMetricsOrder = [...listedMetrics];
    if (start < target) {
      pendingMetricsOrder.splice(target + 1, 0, listedMetrics[start]);
      pendingMetricsOrder.splice(start, 1);
    } else {
      pendingMetricsOrder.splice(target, 0, listedMetrics[start]);
      pendingMetricsOrder.splice(start + 1, 1);
    }
    listedMetrics = [...pendingMetricsOrder];
    columnHovering = null;
  }

  function closeAndApply() {
    pendingMetrics = listedMetrics.filter((m) => pendingMetrics.includes(m));
    isOpen = false;

    let configKey;
    if (cluster && subCluster) {
      configKey = `${configName}:${cluster}:${subCluster}`;
    } else if (cluster && !subCluster) {
      configKey = `${configName}:${cluster}`;
    } else {
      configKey = `${configName}`;
    }

    updateConfigurationMutation({
      name: configKey,
      value: JSON.stringify(pendingMetrics),
    }).subscribe((res) => {
      if (res.fetching === false && res.error) {
        throw res.error;
      }
    });

    if (footprintSelect) {
      showFootprint = !!pendingShowFootprint;
      updateConfigurationMutation({
        name:
          !cluster
            ? "jobList_showFootprint"
            : `jobList_showFootprint:${cluster}`,
        value: JSON.stringify(showFootprint),
      }).subscribe((res) => {
        if (res.fetching === false && res.error) {
          throw res.error;
        }
      });
    };

    applyMetrics(pendingMetrics);
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Configure columns (Metric availability shown)</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#if footprintSelect}
        <li class="list-group-item">
          <input type="checkbox" bind:checked={pendingShowFootprint} /> Show Footprint
        </li>
        <hr />
      {/if}
      {#each listedMetrics as metric, index (metric)}
        <li
          draggable
          class="cc-config-column list-group-item"
          class:is-active={columnHovering === index}
          ondragover={(event) => {
            event.preventDefault()
            return false
          }}
          ondragstart={(event) => {
            columnsDragStart(event, index)
          }}
          ondrop={(event) => {
            event.preventDefault()
            columnsDrag(event, index)
          }}
          ondragenter={() => (columnHovering = index)}
        >
          {#if pendingMetrics.includes(metric)}
            <input
              type="checkbox"
              bind:group={pendingMetrics}
              value={metric}
              checked
            />
          {:else}
            <input
              type="checkbox"
              bind:group={pendingMetrics}
              value={metric}
            />
          {/if}
          {metric}
          <span style="float: right; text-align: justify;">
            {printAvailability(metric, cluster)}
          </span>
        </li>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button color="primary" onclick={() => closeAndApply()}>Close & Apply</Button>
    <Button color="secondary" onclick={() => (isOpen = !isOpen)}>Cancel</Button>
  </ModalFooter>
</Modal>

<style>
  li.cc-config-column {
    display: block;
    cursor: grab;
  }

  li.cc-config-column.is-active {
    background-color: #3273dc;
    color: #fff;
    cursor: grabbing;
  }
</style>
