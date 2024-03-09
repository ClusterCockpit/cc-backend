<!-- 
    @component

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

  export let isOpen = false;
  export let sorting = { field: "startTime", order: "DESC" };

  let sortableColumns = [
    { field: "startTime", text: "Start Time", order: "DESC" },
    { field: "duration", text: "Duration", order: "DESC" },
    { field: "numNodes", text: "Number of Nodes", order: "DESC" },
    { field: "memUsedMax", text: "Max. Memory Used", order: "DESC" },
    { field: "flopsAnyAvg", text: "Avg. FLOPs", order: "DESC" },
    { field: "memBwAvg", text: "Avg. Memory Bandwidth", order: "DESC" },
    { field: "netBwAvg", text: "Avg. Network Bandwidth", order: "DESC" },
  ];

  let activeColumnIdx = sortableColumns.findIndex(
    (col) => col.field == sorting.field,
  );
  sortableColumns[activeColumnIdx].order = sorting.order;
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
              sorting = { field: col.field, order: col.order };
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

