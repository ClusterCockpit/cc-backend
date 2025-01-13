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
  import { getContext } from "svelte";
  import { queryStore, gql, getContextClient, mutationStore } from "@urql/svelte";
  import { Row, Col, Card, Table, Spinner } from "@sveltestrap/sveltestrap";
  import { stickyHeader } from "../generic/utils.js";
  import NodeListRow from "./nodelist/NodeListRow.svelte";
  import Pagination from "../generic/joblist/Pagination.svelte";

  export let cluster;
  export let subCluster = "";
  export let ccconfig = null;
  export let selectedMetrics = [];
  export let selectedResolution = 0;
  export let hostnameFilter = "";
  export let systemUnits = null;
  export let from = null;
  export let to = null;

  // Decouple from Job List Paging Params?
  let usePaging = ccconfig.job_list_usePaging
  let itemsPerPage = usePaging ? ccconfig.plot_list_jobsPerPage : 10;
  let page = 1;
  let paging = { itemsPerPage, page };

  let headerPaddingTop = 0;
  stickyHeader(
    ".cc-table-wrapper > table.table >thead > tr > th.position-sticky:nth-child(1)",
    (x) => (headerPaddingTop = x),
  );

  // const { query: initq } = init();
  const initialized = getContext("initialized");
  const client = getContextClient();
  const nodeListQuery = gql`
    query ($cluster: String!, $subCluster: String!, $nodeFilter: String!, $metrics: [String!], $scopes: [MetricScope!]!, $from: Time!, $to: Time!, $paging: PageRequest!, $selectedResolution: Int) {
      nodeMetricsList(
        cluster: $cluster
        subCluster: $subCluster
        nodeFilter: $nodeFilter
        scopes: $scopes
        metrics: $metrics
        from: $from
        to: $to
        page: $paging
        resolution: $selectedResolution
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
                id
                hostname
                data
                statistics {
                  min
                  avg
                  max
                }
              }
              statisticsSeries {
                min
                median
                max
              }
            }
          }
        }
        totalNodes
        hasNextPage
      }
    }
  `

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

  // Decouple from Job List Paging Params?
  function updateConfiguration(value, page) {
    updateConfigurationMutation({
      name: "plot_list_jobsPerPage",
      value: value,
    }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        nodes = [] // Empty List
        paging = { itemsPerPage: value, page: page }; // Trigger reload of nodeList
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

  if (!usePaging) {
    window.addEventListener('scroll', () => {
      let {
        scrollTop,
        scrollHeight,
        clientHeight
      } = document.documentElement;

      // Add 100 px offset to trigger load earlier
      if (scrollTop + clientHeight >= scrollHeight - 100 && $nodesQuery?.data != null && $nodesQuery.data?.nodeMetricsList.hasNextPage) {
        let pendingPaging = { ...paging }
        pendingPaging.page += 1
        paging = pendingPaging
      };
    });
  };

  $: nodesQuery = queryStore({
    client: client,
    query: nodeListQuery,
    variables: {
      cluster: cluster,
      subCluster: subCluster,
      nodeFilter: hostnameFilter,
      scopes: ["core", "socket", "accelerator"],
      metrics: selectedMetrics,
      from: from.toISOString(),
      to: to.toISOString(),
      paging: paging,
      selectedResolution: selectedResolution,
    },
    requestPolicy: "network-only", // Resolution queries are cached, but how to access them? For now: reload on every change
  });

  let nodes = [];
  $: if ($initialized && $nodesQuery.data) {
    if (usePaging) {
      nodes = [...$nodesQuery.data.nodeMetricsList.items].sort((a, b) => a.host.localeCompare(b.host));
    } else { 
      nodes = nodes.concat([...$nodesQuery.data.nodeMetricsList.items].sort((a, b) => a.host.localeCompare(b.host)))
    }
  }

  $: if (!usePaging && selectedMetrics) {
    // Continous Scroll: Reset list and paging if sleectedMetrics change: Existing entries will not match new metric selection
    nodes = [];
    paging = { itemsPerPage, page: 1 };
  }

  $: matchedNodes = $nodesQuery.data?.nodeMetricsList.totalNodes || matchedNodes;
</script>

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
        {#if $nodesQuery.error}
          <Row>
            <Col>
              <Card body color="danger">{$nodesQuery.error.message}</Card>
            </Col>
          </Row>
        {:else}
          {#each nodes as nodeData (nodeData.host)}
            <NodeListRow {nodeData} {cluster} {selectedMetrics}/>
          {:else}
            <tr>
              <td colspan={selectedMetrics.length + 1}> No nodes found </td>
            </tr>
          {/each}
          {/if}
        {#if $nodesQuery.fetching || !$nodesQuery.data}
          <tr>
            <td colspan={selectedMetrics.length + 1}>
              <div style="text-align:center;">
                <p><b>
                  Loading nodes {nodes.length + 1} to 
                  { matchedNodes 
                    ? `${(nodes.length + paging.itemsPerPage) > matchedNodes ? matchedNodes : (nodes.length + paging.itemsPerPage)} of ${matchedNodes} total`
                    : (nodes.length + paging.itemsPerPage)
                  }
                </b></p>
                <Spinner secondary />
              </div>
            </td>
          </tr>
        {/if}
      </tbody>
    </Table>
  </div>
</Row>

{#if usePaging}
  <Pagination
    bind:page
    {itemsPerPage}
    itemText="Nodes"
    totalItems={matchedNodes}
    on:update-paging={({ detail }) => {
      if (detail.itemsPerPage != itemsPerPage) {
        updateConfiguration(detail.itemsPerPage.toString(), detail.page);
      } else {
        nodes = []
        paging = { itemsPerPage: detail.itemsPerPage, page: detail.page };
      }
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
