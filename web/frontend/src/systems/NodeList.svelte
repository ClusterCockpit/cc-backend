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
  import { queryStore, gql, getContextClient, mutationStore } from "@urql/svelte";
  import { Row, Col, Card, Table, Spinner } from "@sveltestrap/sveltestrap";
  import { stickyHeader } from "../generic/utils.js";
  import NodeListRow from "./nodelist/NodeListRow.svelte";
  import Pagination from "../generic/joblist/Pagination.svelte";

  /* Svelte 5 Props */
  let {
    cluster,
    subCluster = "",
    ccconfig = null,
    selectedMetrics = [],
    selectedResolution = 0,
    hostnameFilter = "",
    presetSystemUnits = null,
    from = null,
    to = null
  } = $props();

  /* Const Init */
  const client = getContextClient();
  const usePaging = ccconfig?.node_list_usePaging || false;
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

  /* State Init */
  let nodes = $state([]);
  let page = $state(1);
  let itemsPerPage = $state(usePaging ? (ccconfig?.plot_list_nodesPerPage || 10) : 10);
  let headerPaddingTop = $state(0);
  let matchedNodes = $state(0);

  /* Derived */
  const paging = $derived({ itemsPerPage, page });
  const nodesQuery = $derived(queryStore({
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
  }));
  
  /* Effects */
    $effect(() => {
    if (!usePaging) {
      window.addEventListener('scroll', () => {
        let {
          scrollTop,
          scrollHeight,
          clientHeight
        } = document.documentElement;

        // Add 100 px offset to trigger load earlier
        if (scrollTop + clientHeight >= scrollHeight - 100  && $nodesQuery?.data?.nodeMetricsList?.hasNextPage) {
          page += 1
        };
      });
    };
  });

  $effect(() => {
    handleNodes($nodesQuery?.data?.nodeMetricsList);
  });

  $effect(() => {
    // Triggers (Except Paging)
    from, to
    selectedMetrics, selectedResolution
    hostnameFilter
    // Continous Scroll: Reset nodes and paging if parameters change: Existing entries will not match new selections
    if (!usePaging) {
      nodes = [];
      page = 1;
    }
  });

  /* Functions */
  function handleNodes(data) {
    if (data) {
      matchedNodes = data.totalNodes;
      if (usePaging || nodes.length == 0) {
        nodes = [...data.items].sort((a, b) => a.host.localeCompare(b.host));
      } else {
        // Workaround to ignore secondary store triggers (reason tbd)
        const oldNodes = $state.snapshot(nodes)
        const newNodes = [...data.items].map((d) => d.host)
        if (!oldNodes.some((n) => newNodes.includes(n.host))) {
          nodes = nodes.concat([...data.items].sort((a, b) => a.host.localeCompare(b.host)))
        };
      };
    };
  };

  // Decouple from Job List Paging Params?
  function updateConfiguration(newItems, newPage) {
    updateConfigurationMutation({
      name: "plot_list_nodesPerPage",
      value: newItems.toString(),
    }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        nodes = []; // Empty List
        itemsPerPage = newItems;
        page = newPage; // Trigger reload of nodeList
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

  /* Const Functions */
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

  /* Init Header */
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
            style="padding-top: {headerPaddingTop}px;"
          >
            {cluster} Node Info
            {#if $nodesQuery.fetching}
              <Spinner size="sm" style="margin-left:10px;" secondary />
            {/if}
          </th>

          {#each selectedMetrics as metric (metric)}
            <th
              class="position-sticky top-0 text-center"
              scope="col"
              style="padding-top: {headerPaddingTop}px"
            >
              {metric} ({presetSystemUnits[metric]})
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
                {#if !usePaging}
                  <p><b>
                    Loading nodes {nodes.length + 1} to 
                    { matchedNodes 
                      ? `${(nodes.length + paging.itemsPerPage) > matchedNodes ? matchedNodes : (nodes.length + paging.itemsPerPage)} of ${matchedNodes} total`
                      : (nodes.length + paging.itemsPerPage)
                    }
                  </b></p>
                {/if}
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
    updatePaging={(detail) => {
      if (detail.itemsPerPage != itemsPerPage) {
        updateConfiguration(detail.itemsPerPage, detail.page);
      } else {
        nodes = [];
        itemsPerPage = detail.itemsPerPage;
        page = detail.page;
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
