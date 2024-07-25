<!-- 
    @component Selector for sorting field and direction

    Properties:
    - sorting:  { field: String, order: "DESC" | "ASC" } (changes from inside)
    - isOpen:   Boolean  (can change from inside and outside)
 -->

<script>
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
  import { getContext } from "svelte";
  import { getSortItems } from "../utils.js";

  export let isOpen = false;
  export let sorting = { field: "startTime", type: "col", order: "DESC" };

  let sortableColumns = [];
  let activeColumnIdx;

  const initialized = getContext("initialized");
  
  function loadSortables(isInitialized) {
    if (!isInitialized) return;
    sortableColumns = [ 
      { field: "startTime", type: "col", text: "Start Time", order: "DESC" },
      { field: "duration", type: "col", text: "Duration", order: "DESC" },
      { field: "numNodes", type: "col", text: "Number of Nodes", order: "DESC" },
      { field: "numHwthreads", type: "col", text: "Number of HWThreads", order: "DESC" },
      { field: "numAcc", type: "col", text: "Number of Accelerators", order: "DESC" },
      ...getSortItems()
    ]
  }

  function loadActiveIndex(isInitialized) {
    if (!isInitialized) return;
    activeColumnIdx = sortableColumns.findIndex(
      (col) => col.field == sorting.field,
    );
    sortableColumns[activeColumnIdx].order = sorting.order;
  }

  $: loadSortables($initialized);
  $: loadActiveIndex($initialized)
</script>

<Modal
  {isOpen}
  toggle={() => {
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
            on:click={() => {
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
      color="primary"
      on:click={() => {
        isOpen = false;
      }}>Close</Button
    >
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

