<!--
    @component Cluster Per Node List component; renders current state of SELECTABLE metrics for ALL nodes

    Properties:
    - `cluster String`: The cluster to show status information for
    - `from Date?`: Custom Time Range selection 'from' [Default: null]
    - `to Date?`: Custom Time Range selection 'to' [Default: null]
 -->

<script>
  import { Row, Table } from "@sveltestrap/sveltestrap";
  import {
    stickyHeader 
  } from "../generic/utils.js";
  import NodeListRow from "./nodelist/NodeListRow.svelte";

  export let cluster;
  export let data = null;
  export let selectedMetrics = [];
  export let systemUnits = null;

  // Always use ONE BIG list, but: Make copyable markers -> Nodeinfo ! (like in markdown)

  let headerPaddingTop = 0;
  stickyHeader(
    ".cc-table-wrapper > table.table >thead > tr > th.position-sticky:nth-child(1)",
    (x) => (headerPaddingTop = x),
  );
</script>

<Row>
  <div class="col cc-table-wrapper">
    <Table cellspacing="0px" cellpadding="0px">
      <thead>
        <tr>
          <th
            class="position-sticky top-0 text-capitalize"
            scope="col"
            style="padding-top: {headerPaddingTop}px"
          >
            {cluster} Node Info
          </th>

          {#each selectedMetrics as metric (metric)}
            <th
              class="position-sticky top-0 text-center"
              scope="col"
              style="padding-top: {headerPaddingTop}px"
            >
              {metric} ({systemUnits[metric]})
            </th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each data as nodeData (nodeData)}
          <NodeListRow {nodeData} {cluster} {selectedMetrics}/>
        {:else}
          <tr>
            <td>No nodes found </td>
          </tr>
        {/each}
      </tbody>
    </Table>
  </div>
</Row>

<style>
  .cc-table-wrapper {
    overflow: initial;
  }

  .cc-table-wrapper > :global(table) {
    border-collapse: separate;
    border-spacing: 0px;
    table-layout: fixed;
  }

  .cc-table-wrapper :global(button) {
    margin-bottom: 0px;
  }

  .cc-table-wrapper > :global(table > tbody > tr > td) {
    margin: 0px;
    padding-left: 5px;
    padding-right: 0px;
  }

  th.position-sticky.top-0 {
    background-color: white;
    z-index: 10;
    border-bottom: 1px solid black;
  }
</style>
