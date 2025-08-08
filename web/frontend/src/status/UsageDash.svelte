<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
-->

 <script>
  import { getContext } from "svelte";
  import {
    Row,
    Col,
    Spinner,
    Card,
    Table,
    Icon,
    Tooltip
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
    mutationStore,
  } from "@urql/svelte";
  import {
    init,
    scramble,
    scrambleNames,
  } from "../generic/utils.js";
  import Pie, { colors, cbColors } from "../generic/plots/Pie.svelte";

  /* Svelte 5 Props */
  let {
    cluster
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const ccconfig = getContext("cc-config");
  const client = getContextClient();
  const paging = { itemsPerPage: 10, page: 1 }; // Top 10
  const topOptions = [
    { key: "totalJobs", label: "Jobs" },
    { key: "totalNodes", label: "Nodes" },
    { key: "totalCores", label: "Cores" },
    { key: "totalAccs", label: "Accelerators" },
  ];

  /* State Init */
  let colWidth = $state(0);
  let cbmode = $state(ccconfig?.plot_general_colorblindMode || false)

  // Pie Charts
  let topProjectSelection = $state(
    topOptions.find(
      (option) =>
        option.key ==
        ccconfig[`status_view_selectedTopProjectCategory:${cluster}`],
    ) ||
    topOptions.find(
      (option) => option.key == ccconfig.status_view_selectedTopProjectCategory,
    )
  );
  let topUserSelection = $state(
    topOptions.find(
      (option) =>
        option.key ==
        ccconfig[`status_view_selectedTopUserCategory:${cluster}`],
    ) ||
    topOptions.find(
      (option) => option.key == ccconfig.status_view_selectedTopUserCategory,
    )
  );

  /* Derived */
  const topUserQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
        $sortBy: SortByAggregate!
      ) {
        topUser: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: $sortBy
          groupBy: USER
        ) {
          id
          name
          totalJobs
          totalNodes
          totalCores
          totalAccs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      paging,
      sortBy: topUserSelection.key.toUpperCase(),
    },
  }));

  const topProjectQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
        $sortBy: SortByAggregate!
      ) {
        topProjects: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: $sortBy
          groupBy: PROJECT
        ) {
          id
          totalJobs
          totalNodes
          totalCores
          totalAccs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      paging,
      sortBy: topProjectSelection.key.toUpperCase(),
    },
  }));

  /* Effects */
  $effect(() => {
    updateTopUserConfiguration(topUserSelection.key);
  });

  $effect(() => {
    updateTopProjectConfiguration(topProjectSelection.key);
  });

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

  /* Functions */
  function updateTopUserConfiguration(select) {
    if (ccconfig[`status_view_selectedTopUserCategory:${cluster}`] != select) {
      updateConfigurationMutation({
        name: `status_view_selectedTopUserCategory:${cluster}`,
        value: JSON.stringify(select),
      }).subscribe((res) => {
        if (res.fetching === false && res.error) {
          throw res.error;
        }
      });
    }
  }

  function updateTopProjectConfiguration(select) {
    if (
      ccconfig[`status_view_selectedTopProjectCategory:${cluster}`] != select
    ) {
      updateConfigurationMutation({
        name: `status_view_selectedTopProjectCategory:${cluster}`,
        value: JSON.stringify(select),
      }).subscribe((res) => {
        if (res.fetching === false && res.error) {
          throw res.error;
        }
      });
    }
  }

  function legendColor(targetIdx) {
    // Reuses first color if targetIdx overflows
    if (cbmode) {
      return cbColors[(colors.length + targetIdx) % colors.length]
    } else {
      return colors[(colors.length + targetIdx) % colors.length]
    }
  }
</script>

{#if $initq.data}
  <!-- User and Project Stats as Pie-Charts -->
  <Row cols={{ lg: 4, md: 2, sm: 1 }}>
    <Col class="p-2">
      <div bind:clientWidth={colWidth}>
        <h4 class="text-center">
          Top Users on {cluster.charAt(0).toUpperCase() + cluster.slice(1)}
        </h4>
        {#key $topUserQuery.data}
          {#if $topUserQuery.fetching}
            <Spinner />
          {:else if $topUserQuery.error}
            <Card body color="danger">{$topUserQuery.error.message}</Card>
          {:else}
            <Pie
              canvasId="hpcpie-users"
              size={colWidth}
              sliceLabel={topUserSelection.label}
              quantities={$topUserQuery.data.topUser.map(
                (tu) => tu[topUserSelection.key],
              )}
              entities={$topUserQuery.data.topUser.map((tu) => scrambleNames ? scramble(tu.id) : tu.id)}
            />
          {/if}
        {/key}
      </div>
    </Col>
    <Col class="px-4 py-2">
      {#key $topUserQuery.data}
        {#if $topUserQuery.fetching}
          <Spinner />
        {:else if $topUserQuery.error}
          <Card body color="danger">{$topUserQuery.error.message}</Card>
        {:else}
          <Table>
            <tr class="mb-2">
              <th>Legend</th>
              <th>User Name</th>
              <th
                >Number of
                <select class="p-0" bind:value={topUserSelection}>
                  {#each topOptions as option}
                    <option value={option}>
                      {option.label}
                    </option>
                  {/each}
                </select>
              </th>
            </tr>
            {#each $topUserQuery.data.topUser as tu, i}
              <tr>
                <td><Icon name="circle-fill" style="color: {legendColor(i)};" /></td>
                <th scope="col" id="topName-{tu.id}"
                  ><a
                    href="/monitoring/user/{tu.id}?cluster={cluster}&state=running"
                    >{scrambleNames ? scramble(tu.id) : tu.id}</a
                  ></th
                >
                {#if tu?.name}
                  <Tooltip
                    target={`topName-${tu.id}`}
                    placement="left"
                    >{scrambleNames ? scramble(tu.name) : tu.name}</Tooltip
                  >
                {/if}
                <td>{tu[topUserSelection.key]}</td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/key}
    </Col>
    <Col class="p-2">
      <h4 class="text-center">
        Top Projects on {cluster.charAt(0).toUpperCase() + cluster.slice(1)}
      </h4>
      {#key $topProjectQuery.data}
        {#if $topProjectQuery.fetching}
          <Spinner />
        {:else if $topProjectQuery.error}
          <Card body color="danger">{$topProjectQuery.error.message}</Card>
        {:else}
          <Pie
            canvasId="hpcpie-projects"
            size={colWidth}
            sliceLabel={topProjectSelection.label}
            quantities={$topProjectQuery.data.topProjects.map(
              (tp) => tp[topProjectSelection.key],
            )}
            entities={$topProjectQuery.data.topProjects.map((tp) => scrambleNames ? scramble(tp.id) : tp.id)}
          />
        {/if}
      {/key}
    </Col>
    <Col class="px-4 py-2">
      {#key $topProjectQuery.data}
        {#if $topProjectQuery.fetching}
          <Spinner />
        {:else if $topProjectQuery.error}
          <Card body color="danger">{$topProjectQuery.error.message}</Card>
        {:else}
          <Table>
            <tr class="mb-2">
              <th>Legend</th>
              <th>Project Code</th>
              <th
                >Number of
                <select class="p-0" bind:value={topProjectSelection}>
                  {#each topOptions as option}
                    <option value={option}>
                      {option.label}
                    </option>
                  {/each}
                </select>
              </th>
            </tr>
            {#each $topProjectQuery.data.topProjects as tp, i}
              <tr>
                <td><Icon name="circle-fill" style="color: {legendColor(i)};" /></td>
                <th scope="col"
                  ><a
                    href="/monitoring/jobs/?cluster={cluster}&state=running&project={tp.id}&projectMatch=eq"
                    >{scrambleNames ? scramble(tp.id) : tp.id}</a
                  ></th
                >
                <td>{tp[topProjectSelection.key]}</td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/key}
    </Col>
  </Row>
{/if}
