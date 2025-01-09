<!--
    @component Cluster Per Node List component; renders current state of SELECTABLE metrics for ALL nodes

    Properties:
    - `cluster String`: The nodes' cluster
    - `subCluster String`: The nodes' subCluster
    - `ccconfig Object?`: The ClusterCockpit Config Context [Default: null]
    - `selectedMetrics [String]`: The array of selected metrics
    - `systemUnits Object`: The object of metric units
 -->

<script>
  import { queryStore, gql, getContextClient } from "@urql/svelte";
  import { Row, Col, Card, Table, Spinner } from "@sveltestrap/sveltestrap";
  import { init, stickyHeader } from "../generic/utils.js";
  import NodeListRow from "./nodelist/NodeListRow.svelte";
  import Pagination from "../generic/joblist/Pagination.svelte";

  export let cluster;
  export let subCluster = "";
  export const ccconfig = null;
  export let selectedMetrics = [];
  export let hostnameFilter = "";
  export let systemUnits = null;
  export let from = null;
  export let to = null;

  // let usePaging = ccconfig.node_list_usePaging
  let itemsPerPage = 10 // usePaging ? ccconfig.node_list_jobsPerPage : 10;
  let page = 1;
  let paging = { itemsPerPage, page };

  let headerPaddingTop = 0;
  stickyHeader(
    ".cc-table-wrapper > table.table >thead > tr > th.position-sticky:nth-child(1)",
    (x) => (headerPaddingTop = x),
  );

  const { query: initq } = init();
  const client = getContextClient();
  const nodeListQuery = gql`
    query ($cluster: String!, $subCluster: String!, $nodeFilter: String!, $metrics: [String!], $scopes: [MetricScope!]!, $from: Time!, $to: Time!, $paging: PageRequest!) {
      nodeMetricsList(
        cluster: $cluster
        subCluster: $subCluster
        nodeFilter: $nodeFilter
        scopes: $scopes
        metrics: $metrics
        from: $from
        to: $to
        page: $paging
      ) {
        items {
          host
          subCluster
          metrics {
            name
            scope
            metric {
              timestep
              unit {
                base
                prefix
              }
              series {
                statistics {
                  min
                  avg
                  max
                }
                data
              }
            }
          }
        }
        totalNodes
        hasNextPage
      }
    }
  `

  $: nodesQuery = queryStore({
    client: client,
    query: nodeListQuery,
    variables: {
      cluster: cluster,
      subCluster: subCluster,
      nodeFilter: hostnameFilter,
      scopes: ["core", "accelerator"],
      metrics: selectedMetrics,
      from: from.toISOString(),
      to: to.toISOString(),
      paging: paging,
    },
  });

  $: matchedNodes = $nodesQuery.data?.nodeMetricsList.totalNodes || 0;
</script>

{#if $nodesQuery.error}
  <Row>
    <Col>
      <Card body color="danger">{$nodesQuery.error.message}</Card>
    </Col>
  </Row>
{:else if $nodesQuery.fetching }
  <Row>
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if $initq?.data && $nodesQuery?.data}
  <Row>
    <div class="col cc-table-wrapper">
      <Table cellspacing="0px" cellpadding="0px">
        <thead>
          <tr>
            <th
              class="position-sticky top-0 text-capitalize"
              scope="col"
              style="padding-top: {headerPaddingTop}px;"
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
          {#each $nodesQuery.data.nodeMetricsList.items as nodeData (nodeData.host)}
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
{/if}

{#if true} <!-- usePaging -->
  <Pagination
    bind:page
    {itemsPerPage}
    itemText="Nodes"
    totalItems={matchedNodes}
    on:update-paging={({ detail }) => {
      paging = { itemsPerPage: detail.itemsPerPage, page: detail.page }
      // if (detail.itemsPerPage != itemsPerPage) {
      //   updateConfiguration(detail.itemsPerPage.toString(), detail.page);
      // } else {
      //   // nodes = []
      //   paging = { itemsPerPage: detail.itemsPerPage, page: detail.page };
      // }
    }}
  />
{/if}

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
