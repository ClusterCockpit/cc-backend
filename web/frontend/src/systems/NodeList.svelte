<!--
    @component Cluster Per Node List component; renders current state of SELECTABLE metrics for ALL nodes

    Properties:
    - `cluster String`: The cluster to show status information for
    - `from Date?`: Custom Time Range selection 'from' [Default: null]
    - `to Date?`: Custom Time Range selection 'to' [Default: null]

    Properties:
    - `sorting Object?`: Currently active sorting [Default: {field: "startTime", type: "col", order: "DESC"}]
    - `matchedJobs Number?`: Number of matched jobs for selected filters [Default: 0]
    - `metrics [String]?`: The currently selected metrics [Default: User-Configured Selection]
    - `showFootprint Bool`: If to display the jobFootprint component

    Functions:
    - `refreshJobs()`: Load jobs data with unchanged parameters and 'network-only' keyword
    - `refreshAllMetrics()`: Trigger downstream refresh of all running jobs' metric data
    - `queryJobs(filters?: [JobFilter])`: Load jobs data with new filters, starts from page 1
 -->

<script>
  import {
    gql,
    mutationStore,
  } from "@urql/svelte";
  import { Row, Table } from "@sveltestrap/sveltestrap";
  import {
    checkMetricsDisabled,
    stickyHeader 
  } from "../generic/utils.js";
  import NodeListRow from "./nodelist/NodeListRow.svelte";

  export let cluster;
  export let nodesData = null;
  export let selectedMetrics = [];
  export let systemUnits = null;
  export let hostnameFilter = "";

  // Always use ONE BIG list, but: Make copyable markers -> Nodeinfo ! (like in markdown)

  $: nodes = nodesData.nodeMetrics
    .filter(
      (h) =>
        h.host.includes(hostnameFilter) &&
        h.metrics.some(
          (m) => selectedMetrics.includes(m.name) && m.scope == "node",
        ),
    )
    .map((h) => ({
      host: h.host,
      subCluster: h.subCluster,
      data: h.metrics.find(
        (m) => selectedMetrics.includes(m.name) && m.scope == "node",
      ),
      disabled: checkMetricsDisabled(
        selectedMetrics,
        cluster,
        h.subCluster,
      ),
    }))
    .sort((a, b) => a.host.localeCompare(b.host))

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

  function updateConfiguration(value) {
    updateConfigurationMutation({
      name: "node_list_selectedMetrics",
      value: value,
    }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        console.log('Selected Metrics for Node List Updated')
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

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
            class="position-sticky top-0"
            scope="col"
            style="padding-top: {headerPaddingTop}px"
          >
            Node Info
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
        {#each nodes as node (node)}
          {node}
          <!-- <NodeListRow {node} {selectedMetrics} /> -->
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
