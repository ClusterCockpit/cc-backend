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
  - `configName String`: The config key for the last saved selection (constant)
  - `globalMetrics [Obj]`: Includes the backend supplied availabilities for cluster and subCluster
  - `applyMetrics Func`: The callback function to apply current selection
-->

<script>
  import {
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Button,
    ListGroup,
    Icon,
    Tooltip
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
    maxClusters = null,
    maxSubClusters = null,
    footprintSelect = false,
    configName,
    globalMetrics,
    applyMetrics
  } = $props();

  /* Const Init */
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
  let columnHovering = $state(null);

  /* Derives States */
  const allMetrics = $derived(loadAvailable(globalMetrics));
  let pendingMetrics = $derived(presetMetrics || []);
  let listedMetrics = $derived([...presetMetrics, ...allMetrics.difference(new Set(presetMetrics))]); // List (preset) active metrics first, then list inactives

  /* Reactive Effects */
  $effect(() => {
    totalMetrics = allMetrics?.size || 0;
  });

  /* Functions */
  function loadAvailable(gms) {
    const availableMetrics = new Set();
    if (gms) {
      for (let gm of gms) {
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
    return availableMetrics;
  }

  function printAvailabilityCount(metric, cluster) {
    const avail = globalMetrics.find((gm) => gm.name === metric)?.availability
    if (avail) {
      if (!cluster) {
        return `${avail.length} / ${maxClusters} Cluster`
      } else {
        const subAvail = avail.find((av) => av.cluster === cluster)?.subClusters
        if (subAvail) {
          return `${subAvail.length} / ${maxSubClusters} SubCluster`
        } else {
          return `0 / ${maxSubClusters} SubCluster`
        }
      }
    }
    return `0 / ${maxClusters} Cluster`
  }

  function printAvailability(metric, cluster) {
    const avail = globalMetrics.find((gm) => gm.name === metric)?.availability
    if (avail) {
      if (!cluster) {
        return avail.map((av) => av.cluster)
      } else {
        const subAvail = avail.find((av) => av.cluster === cluster)?.subClusters
        if (subAvail) {
          return subAvail
        } else {
          return [`Not available for ${cluster}`]
        }
      }
    }
    return [`Not available for ${cluster}`]
  }

  function printTooltip(metric) {
    const toolt = globalMetrics.find((gm) => gm.name === metric)?.tooltip
    if (toolt) {
      return toolt
    }
    return ""
  }

  function columnsDragOver(event) {
		event.preventDefault();
		event.dataTransfer.dropEffect = 'move';
  }

  function columnsDragStart(event, i) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.dropEffect = "move";
    event.dataTransfer.setData("text/plain", i);
  }

  function columnsDrop(event, target) {
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
  <ModalHeader>Configure columns</ModalHeader>
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
          draggable={true}
          class="cc-config-column list-group-item"
          class:is-active={columnHovering === index}
          ondragover={(event) => {
            columnsDragOver(event)
          }}
          ondragstart={(event) => {
            columnsDragStart(event, index)
          }}
          ondrop={(event) => {
            event.preventDefault()
            columnsDrop(event, index)
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
          {#if maxClusters !== null || maxSubClusters !== null}
            <span style="float: right;" class="ms-1">
              <Button id={`${metric}-avail-info`} outline color="secondary" size="sm" class="ml-2">
                <b>{ printAvailabilityCount(metric, cluster) }</b>
              </Button>
              <Tooltip target={`${metric}-avail-info`} placement="right">
                <b>Availability</b>
                <ul style="text-align: left; padding-left: 1.0rem; margin-bottom: 0.25rem;">
                  {#each printAvailability(metric, cluster) as avail}
                    <li>{avail}</li>
                  {/each}
                </ul>
              </Tooltip>
            </span>
          {/if}
          {#if printTooltip(metric) !== ""}
            <span style="float: right;">
              <Button id={`${metric}-kind-info`} outline color="secondary" size="sm" class="ml-2">
                <Icon name="info-square" />
              </Button>
              <Tooltip target={`${metric}-kind-info`} placement="right">
                <b>Information</b>
                <p style="text-align: left; margin-bottom: 0.25rem;">
                  { printTooltip(metric) }
                </p>
              </Tooltip>
            </span>
          {/if}
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
