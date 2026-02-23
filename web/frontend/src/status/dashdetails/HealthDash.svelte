<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
    Card,
    Input,
    InputGroup,
    InputGroupText,
    Table,
    Icon,
    Spinner
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import Refresher from "../../generic/helper/Refresher.svelte";
  import Pie, { colors } from "../../generic/plots/Pie.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
  } = $props();

  /* Const Init */
  const client = getContextClient();
  const healthOptions = [
    "all",
    "full",
    "partial",
    "failed",
  ];

  /* State Init */
  let pieWidth = $state(0);
  let querySorting = $state({ field: "startTime", type: "col", order: "DESC" })
  let tableHostFilter = $state("");
  let tableHealthFilter = $state(healthOptions[0]);
  let healthTableSorting = $state(
    {
      healthState:  { dir: "up", active: true },
      hostname:  { dir: "down", active: false },
    }
  );

  /* Derived */
  let cluster = $derived(presetCluster);

  const statusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $nodeFilter: [NodeFilter!]!
        $sorting: OrderByInput!
      ) {
        # $sorting unused in backend: Use placeholder
        nodes: nodesWithMeta(filter: $nodeFilter, order: $sorting) {
          count
          items {
            hostname
            cluster
            subCluster
            healthState
            healthData
          }
        }
        # Get Current States for Pie Charts
        nodeStates(filter: $nodeFilter) {
          state
          count
        },
      }
    `,
    variables: {
      nodeFilter: { cluster: { eq: cluster }},
      sorting: querySorting,
    },
    requestPolicy: "network-only"
  }));

  let healthTableData = $derived.by(() => {
    if ($statusQuery?.data) {
      return [...$statusQuery.data.nodes.items].sort((n1, n2) => {
        return n1['healthState'].localeCompare(n2['healthState'])
      });
    } else {
      return [];
    }
  });

  let filteredTableData = $derived.by(() => {
    let pendingTableData = [...healthTableData];
    if (tableHostFilter != "") {
          pendingTableData = pendingTableData.filter((e) => e.hostname.includes(tableHostFilter))
    }
    if (tableHealthFilter != "all") {
          pendingTableData = pendingTableData.filter((e) => e.healthState.includes(tableHealthFilter))
    }
    return pendingTableData
  });

  const refinedHealthData = $derived.by(() => {
    return $statusQuery?.data?.nodeStates.
      filter((e) => ['full', 'partial', 'failed'].includes(e.state)).
      sort((a, b) => b.count - a.count)
  });

  /* Functions */
  function sortBy(field) {
    const s = healthTableSorting[field];
    if (s.active) {
      s.dir = s.dir == "up" ? "down" : "up";
    } else {
      for (let key in healthTableSorting)
        healthTableSorting[key].active = false;
      s.active = true;
    }

    const pendingHealthData = healthTableData.sort((n1, n2) => {
      if (n1[field] == null || n2[field] == null) return -1;
      else if (s.dir == "down") return n1[field].localeCompare(n2[field])
      else return n2[field].localeCompare(n1[field])
    });
  
    healthTableSorting = {...healthTableSorting};
    healthTableData = [...pendingHealthData];
  }

</script>

<!-- Refresher and space for other options -->
<Row class="justify-content-between">
  <Col xs="12" md="5" lg="4" xl="3">
    <Refresher
      initially={120}
      onRefresh={(interval) => {
        querySorting = { field: "startTime", type: "col", order: "DESC" };
      }}
    />
  </Col>
</Row>

<hr/>

<!-- Node Health Pis, later Charts -->
{#if $statusQuery.fetching}
  <Row cols={1} class="text-center mt-3">
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if $statusQuery.error}
  <Row cols={1} class="text-center mt-3">
    <Col>  
      <Card body color="danger">Status Query (States): {$statusQuery.error.message}</Card>
    </Col>
  </Row>
{:else if $statusQuery?.data?.nodeStates}
  <Row cols={{ lg: 4, md: 2 , sm: 1}} class="mb-3 justify-content-center">
    <Col class="px-3 mt-2 mt-lg-0">
      <div bind:clientWidth={pieWidth}>
        {#key refinedStateData}
          <h4 class="text-center">
            Current {cluster.charAt(0).toUpperCase() + cluster.slice(1)} Node States
          </h4>
          <Pie
            canvasId="hpcpie-slurm"
            size={pieWidth * 0.55}
            sliceLabel="Nodes"
            quantities={refinedStateData.map(
              (sd) => sd.count,
            )}
            entities={refinedStateData.map(
              (sd) => sd.state,
            )}
            fixColors={refinedStateData.map(
              (sd) => colors['nodeStates'][sd.state],
            )}
          />
        {/key}
      </div>
    </Col>
    <Col class="px-4 py-2">
      {#key refinedStateData}
        <Table>
          <tr class="mb-2">
            <th></th>
            <th>Current State</th>
            <th>Nodes</th>
          </tr>
          {#each refinedStateData as sd, i}
            <tr>
              <td><Icon name="circle-fill" style="color: {colors['nodeStates'][sd.state]};"/></td>
              <td>{sd.state}</td>
              <td>{sd.count}</td>
            </tr>
          {/each}
        </Table>
      {/key}
    </Col>

    <Col class="px-3 mt-2 mt-lg-0">
      <div bind:clientWidth={pieWidth}>
        {#key refinedHealthData}
          <h4 class="text-center">
            Current {cluster.charAt(0).toUpperCase() + cluster.slice(1)} Node Health
          </h4>
          <Pie
            canvasId="hpcpie-health"
            size={pieWidth * 0.55}
            sliceLabel="Nodes"
            quantities={refinedHealthData.map(
              (hd) => hd.count,
            )}
            entities={refinedHealthData.map(
              (hd) => hd.state,
            )}
            fixColors={refinedHealthData.map(
              (hd) => colors['healthStates'][hd.state],
            )}
          />
        {/key}
      </div>
    </Col>
    <Col class="px-4 py-2">
      {#key refinedHealthData}
        <Table>
          <tr class="mb-2">
            <th></th>
            <th>Current Health</th>
            <th>Nodes</th>
          </tr>
          {#each refinedHealthData as hd, i}
            <tr>
              <td><Icon name="circle-fill"style="color: {colors['healthStates'][hd.state]};" /></td>
              <td>{hd.state}</td>
              <td>{hd.count}</td>
            </tr>
          {/each}
        </Table>
      {/key}
    </Col>
  </Row>
{/if}

<hr/>

<!-- Tabular Info About Node States and Missing Metrics -->
{#if $statusQuery.fetching}
  <Row cols={1} class="text-center mt-3">
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if $statusQuery.error}
  <Row cols={1} class="text-center mt-3">
    <Col>  
      <Card body color="danger">Status Query (Details): {$statusQuery.error.message}</Card>
    </Col>
  </Row>
{:else if $statusQuery.data}
  <Row>
    <Col>
      <Card>
        <Table hover responsive>
          <thead>
            <!-- Header Row 1: Titles and Sorting -->
            <tr>
              <th style="width: 10%; min-width: 100px; max-width:12%;" onclick={() => sortBy('hostname')}>
                Hosts ({filteredTableData.length})
                <Icon
                  name="caret-{healthTableSorting['hostname'].dir}{healthTableSorting['hostname']
                    .active
                    ? '-fill'
                    : ''}"
                />
              </th>
              <th style="width: 10%; min-width: 100px; max-width:12%;" onclick={() => sortBy('healthState')}>
                Health State
                <Icon
                  name="caret-{healthTableSorting['healthState'].dir}{healthTableSorting['healthState']
                    .active
                    ? '-fill'
                    : ''}"
                />
              </th>
              <th>Metric Availability</th>
            </tr>
            <!-- Header Row 2: Filters -->
            <tr>
              <th>
                <InputGroup size="sm">
                  <Input type="text" bind:value={tableHostFilter}/>
                  <InputGroupText>
                    <Icon name="search"></Icon>
                  </InputGroupText>
                </InputGroup>
              </th>
              <th>
                  <Input size="sm" type="select" bind:value={tableHealthFilter}>
                    {#each healthOptions as ho}
                      <option value={ho}>{ho}</option>
                    {/each}
                  </Input>
              </th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each filteredTableData as host (host.hostname)}
              <tr>
                <th scope="row"><b><a href="/monitoring/node/{cluster}/{host.hostname}" target="_blank">{host.hostname}</a></b></th>
                <td>{host.healthState}</td>
                <td style="max-width: 76%;">
                  {#each Object.keys(host.healthData) as hkey}
                    <p>
                      <b>{hkey}</b>: {host.healthData[hkey]}
                    </p>
                  {/each}
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      </Card>
    </Col>
  </Row>
{:else}
  <Card class="mx-4" body color="warning">Cannot render metric health info: No data!</Card>
{/if}
