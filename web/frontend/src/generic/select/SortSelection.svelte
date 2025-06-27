<!-- 
    @component Selector for sorting field and direction

    Properties:
    - sorting:  { field: String, order: "DESC" | "ASC" } (changes from inside)
    - isOpen:   Boolean  (can change from inside and outside)
 -->

<script>
  import { getContext, onMount } from "svelte";
  import {
    Icon,
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
    presetSorting = { field: "startTime", type: "col", order: "DESC" },
    applySorting
  } = $props();

  /* Const Init */
  const initialized = getContext("initialized");
  const globalMetrics = getContext("globalMetrics");
  const fixedSortables = $state([ 
    { field: "startTime", type: "col", text: "Start Time (Default)", order: "DESC" },
    { field: "duration", type: "col", text: "Duration", order: "DESC" },
    { field: "numNodes", type: "col", text: "Number of Nodes", order: "DESC" },
    { field: "numHwthreads", type: "col", text: "Number of HWThreads", order: "DESC" },
    { field: "numAcc", type: "col", text: "Number of Accelerators", order: "DESC" },
    { field: "energy", type: "col", text: "Total Energy", order: "DESC" },
  ]);

  /* State Init */
  let sorting = $state({...presetSorting})
  let activeColumnIdx = $state(0);
  let metricSortables = $state([]);

  /* Derived */
  let sortableColumns = $derived([...fixedSortables, ...metricSortables]);

  /* Effect */
  $effect(() => {
    if ($initialized) {
      loadMetricSortables();
    };
  });

  /* Functions */  
  function loadMetricSortables() {
    metricSortables = globalMetrics.map((gm) => {
        if (gm?.footprint) {
            return { 
                field: gm.name + '_' + gm.footprint,
                type: 'foot',
                text: gm.name + ' (' + gm.footprint + ')',
                order: 'DESC'
            }
        }
        return null
    }).filter((r) => r != null)
  };

  function loadActiveIndex() {
    activeColumnIdx = sortableColumns.findIndex(
      (col) => col.field == sorting.field,
    );
    sortableColumns[activeColumnIdx].order = sorting.order;
  }

  function resetSorting(sort) {
    sorting = {...sort};
    loadActiveIndex();
  };
</script>

<Modal
  {isOpen}
  toggle={() => {
    resetSorting(presetSorting);
    isOpen = !isOpen;
  }}
>
  <ModalHeader>Sort rows</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#each sortableColumns as col, i (col)}
        <ListGroupItem>
          <button
            class="sort"
            onclick={() => {
              if (activeColumnIdx == i) {
                col.order = col.order == "DESC" ? "ASC" : "DESC";
              } else {
                sortableColumns[activeColumnIdx] = {
                  ...sortableColumns[activeColumnIdx],
                };
              }

              sortableColumns[i] = { ...sortableColumns[i] };
              activeColumnIdx = i;
              sortableColumns = [...sortableColumns];
              sorting = { field: col.field, type: col.type, order: col.order };
            }}
          >
            <Icon
              name="arrow-{col.order == 'DESC' ? 'down' : 'up'}-circle{i ==
              activeColumnIdx
                ? '-fill'
                : ''}"
            />
          </button>

          {col.text}
        </ListGroupItem>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button
      color="warning"
      onclick={() => {
        isOpen = false;
        resetSorting({ field: "startTime", type: "col", order: "DESC" });
        applySorting(sorting);
      }}>Reset</Button
    >
    <Button
      color="primary"
      onclick={() => {
        applySorting(sorting);
        isOpen = false;
      }}>Close & Apply</Button
    >
    <Button
      color="secondary"
      onclick={() => {
        resetSorting(presetSorting);
        isOpen = false
      }}>Cancel
    </Button>
  </ModalFooter>
</Modal>

<style>
  .sort {
    border: none;
    margin: 0;
    padding: 0;
    background: 0 0;
    transition: all 70ms;
  }
</style>

