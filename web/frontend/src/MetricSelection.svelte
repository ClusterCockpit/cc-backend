<!-- 
    @component

    Properties:
    - metrics:    [String] (changes from inside, needs to be initialised, list of selected metrics)
    - isOpen:     Boolean  (can change from inside and outside)
    - configName: String   (constant)
 -->

<script>
  import {
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Button,
    ListGroup,
  } from "@sveltestrap/sveltestrap";
  import { getContext } from "svelte";
  import { gql, getContextClient, mutationStore } from "@urql/svelte";

  export let metrics;
  export let isOpen;
  export let configName;
  export let allMetrics = null;
  export let cluster = null;
  export let showFootprint = false;
  export let view = "job";

  const onInit = getContext("on-init")
  const globalMetrics = getContext("globalMetrics")

  let newMetricsOrder = [];
  let unorderedMetrics = [...metrics];
  let pendingShowFootprint = !!showFootprint;

  onInit(() => {
    if (allMetrics == null) allMetrics = new Set();
    for (let metric of globalMetrics) allMetrics.add(metric.name);
  });

  $: {
    if (allMetrics != null) {
      if (cluster == null) {
        for (let metric of globalMetrics) allMetrics.add(metric.name);
      } else {
        allMetrics.clear();
        for (let gm of globalMetrics) {
          if (gm.availability.find((av) => av.cluster === cluster)) allMetrics.add(gm.name);
        }
      }
      newMetricsOrder = [...allMetrics].filter((m) => !metrics.includes(m));
      newMetricsOrder.unshift(...metrics.filter((m) => allMetrics.has(m)));
      unorderedMetrics = unorderedMetrics.filter((m) => allMetrics.has(m));
    }
  }

  function printAvailability(metric, cluster) {
    const avail = globalMetrics.find((gm) => gm.name === metric)?.availability
    if (cluster == null) {
      return avail.map((av) => av.cluster).join(',')
    } else {
      return avail.find((av) => av.cluster === cluster).subClusters.join(',')
    }
  }

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

  let columnHovering = null;

  function columnsDragStart(event, i) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.dropEffect = "move";
    event.dataTransfer.setData("text/plain", i);
  }

  function columnsDrag(event, target) {
    event.dataTransfer.dropEffect = "move";
    const start = Number.parseInt(event.dataTransfer.getData("text/plain"));
    if (start < target) {
      newMetricsOrder.splice(target + 1, 0, newMetricsOrder[start]);
      newMetricsOrder.splice(start, 1);
    } else {
      newMetricsOrder.splice(target, 0, newMetricsOrder[start]);
      newMetricsOrder.splice(start + 1, 1);
    }
    columnHovering = null;
  }

  function closeAndApply() {
    metrics = newMetricsOrder.filter((m) => unorderedMetrics.includes(m));
    isOpen = false;

    showFootprint = !!pendingShowFootprint;

    updateConfigurationMutation({
      name: cluster == null ? configName : `${configName}:${cluster}`,
      value: JSON.stringify(metrics),
    }).subscribe((res) => {
      if (res.fetching === false && res.error) {
        throw res.error;
      }
    });

    updateConfigurationMutation({
      name:
        cluster == null
          ? "plot_list_showFootprint"
          : `plot_list_showFootprint:${cluster}`,
      value: JSON.stringify(showFootprint),
    }).subscribe((res) => {
      if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Configure columns (Metric availability shown)</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#if view === "list"}
        <li class="list-group-item">
          <input type="checkbox" bind:checked={pendingShowFootprint} /> Show Footprint
        </li>
        <hr />
      {/if}
      {#each newMetricsOrder as metric, index (metric)}
        <li
          class="cc-config-column list-group-item"
          draggable={true}
          ondragover="return false"
          on:dragstart={(event) => columnsDragStart(event, index)}
          on:drop|preventDefault={(event) => columnsDrag(event, index)}
          on:dragenter={() => (columnHovering = index)}
          class:is-active={columnHovering === index}
        >
          {#if unorderedMetrics.includes(metric)}
            <input
              type="checkbox"
              bind:group={unorderedMetrics}
              value={metric}
              checked
            />
          {:else}
            <input
              type="checkbox"
              bind:group={unorderedMetrics}
              value={metric}
            />
          {/if}
          {metric}
          <span style="float: right;">
            {printAvailability(metric, cluster)}
          </span>
        </li>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button color="primary" on:click={closeAndApply}>Close & Apply</Button>
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
