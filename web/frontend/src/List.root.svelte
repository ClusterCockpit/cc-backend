<!--
  @component Main component for listing users or projects

  Properties:
  - `type String?`: The type of list ['USER' || 'PROJECT']
  - `filterPresets Object?`: Optional predefined filter values [Default: {}]
-->

<script>
  import { getContext, onMount } from "svelte";
  import {
    Row,
    Col,
    Button,
    Icon,
    Table,
    Card,
    Spinner,
    InputGroup,
    Input,
    Tooltip
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    scramble,
    scrambleNames,
  } from "./generic/utils.js";
  import {
    formatDurationTime
  } from "./generic/units.js";
  import Filters from "./generic/Filters.svelte";

  /* Svelte 5 Props */
  let {
    type,
    filterPresets
  } = $props();

  /* Const Init */
  const {} = init();
  const client = getContextClient();
  const shortDuration = getContext("cc-config").jobList_hideShortRunningJobs; // Always configured

  /* State Init*/
  let filterComponent = $state(); // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
  let jobFilters = $state([]);
  let nameFilter = $state("");
  let sorting = $state({ field: "totalJobs", direction: "desc" });

  /* Derived Vars */
  const fetchRunning = $derived(jobFilters.some(jf => jf?.state?.length == 1 && jf?.state?.includes("running")));
  const numCols = $derived.by(() => {
    let colbase = 6
    if (fetchRunning) {
      colbase += 2
    }
    return colbase
  })

  let stats = $derived(
    queryStore({
      client: client,
      query: gql`
        query($jobFilters: [JobFilter!]!, $fetchRunning: Boolean!) {
          rows: jobsStatistics(filter: $jobFilters, groupBy: ${type}) {
            id
            name
            totalJobs
            shortJobs
            totalCores @include(if: $fetchRunning)
            totalAccs @include(if: $fetchRunning)
            totalWalltime
            totalCoreHours
            totalAccHours
          }
        }`,
      variables: {
        jobFilters,
        fetchRunning
      },
    })
  );

  /* Functions */
  function changeSorting(newField) {
    if (sorting.field == newField) {
      // Same Field, Change Direction
      sorting = { field: newField, direction: sorting.direction == "desc" ? "asc" : "desc" };
    } else {
      // Change Field, Apply Default Direction
      sorting = { field: newField, direction: "desc" };
    }
  }

  function sort(stats, sorting, nameFilter) {
    const idCmp = sorting.direction == "asc"
      ? (a, b) => b.id.localeCompare(a.id)
      : (a, b) => a.id.localeCompare(b.id)

    // Force empty or undefined strings to the end of the list
    const nameCmp = sorting.direction == "asc"
      ? (a, b) => !a?.name ? 1 : (!b?.name ? -1 : (b.name.localeCompare(a.name)))
      : (a, b) => !a?.name ? 1 : (!b?.name ? -1 : (a.name.localeCompare(b.name)))

    const intCmp = sorting.direction == "asc"
      ? (a, b) => a[sorting.field] - b[sorting.field]
      : (a, b) => b[sorting.field] - a[sorting.field];

    if (sorting.field == "id") {
      return stats.filter((u) => u.id.includes(nameFilter)).sort(idCmp)
    } else if (sorting.field == "name") {
      return stats.filter((u) => u.id.includes(nameFilter)).sort(nameCmp)
    } else {
      return stats.filter((u) => u.id.includes(nameFilter)).sort(intCmp)
    }
  }

  /* On Mount */
  onMount(() => filterComponent.updateFilters());
</script>

<Row cols={{ xs: 1, md: 2}}>
  <Col xs="12" md="5" lg="4" xl="3" class="mb-2 mb-md-0">
    <InputGroup>
      <Button disabled outline>
        Search {type.toLowerCase()}s
      </Button>
      <Input
        bind:value={nameFilter}
        placeholder="Filter by {{
          USER: 'username',
          PROJECT: 'project',
        }[type]}"
      />
    </InputGroup>
  </Col>
  <Col xs="12" md="7" lg="8" xl="9">
    <Filters
      startTimeQuickSelect
      bind:this={filterComponent}
      {filterPresets}
      menuText="Only {type.toLowerCase()}s with matching jobs will be displayed."
      applyFilters={(detail) => {
        jobFilters = detail.filters;
      }}
    />
  </Col>
</Row>
<Table>
  <thead>
    <tr>
      <th scope="col">
        {#if type === 'USER'}
          Username
        {:else if type === 'PROJECT'}
          Project Name
        {/if}
        <Button
          color={sorting.field == "id" ? "primary" : "light"}
          size="sm"
          onclick={() => changeSorting("id")}
        >
          {#if sorting?.field == "id"}
            <!-- Note on Icon-Name: Arrow-indicator always down, only alpha-indicator switches -->
            <Icon name={`sort-alpha-${sorting?.direction == 'desc' ? 'down' : 'down-alt'}`} />
          {:else}
            <Icon name="three-dots-vertical" />
          {/if}
        </Button>
      </th>
      {#if type == "USER"}
        <th scope="col">
          Name
          <Button
            color={sorting.field == "name" ? "primary" : "light"}
            size="sm"
            onclick={() => changeSorting("name")}
          >
            {#if sorting?.field == "name"}
              <Icon name={`sort-alpha-${sorting?.direction == 'desc' ? 'down' : 'down-alt'}`} />
            {:else}
              <Icon name="three-dots-vertical" />
            {/if}
          </Button>
        </th>
      {/if}
      <th scope="col">
        Total Jobs
        <Button
          color={sorting.field == "totalJobs" ? "primary" : "light"}
          size="sm"
          onclick={() => changeSorting("totalJobs")}
        >
          {#if sorting?.field == "totalJobs"}
            <!-- Note on Icon-Name: Arrow-indicator always down, only numeric-indicator switches -->
            <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
          {:else}
            <Icon name="three-dots-vertical" />
          {/if}
        </Button>
      </th>
      <th scope="col">
        <span class="mr-1">
          Short Jobs
          <Icon id="shortjobs-info" style="cursor:help;" size="sm" name="info-circle"/>
        </span>
        <Tooltip target={`shortjobs-info`} placement="top">
          Job duration less than {formatDurationTime(shortDuration)}
        </Tooltip>
        &#8239; <!-- Narrow Non-Breaking Space -->
        <Button
          color={sorting.field == "shortJobs" ? "primary" : "light"}
          size="sm"
          onclick={() => changeSorting("shortJobs")}
        >
          {#if sorting?.field == "shortJobs"}
            <!-- Note on Icon-Name: Arrow-indicator always down, only numeric-indicator switches -->
            <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
          {:else}
            <Icon name="three-dots-vertical" />
          {/if}
        </Button>
      </th>
      {#if fetchRunning}
        <th scope="col">
          Total Cores
          <Button
            color={sorting.field == "totalCores" ? "primary" : "light"}
            size="sm"
            onclick={() => changeSorting("totalCores")}
          >
            {#if sorting?.field == "totalJCores"}
              <!-- Note on Icon-Name: Arrow-indicator always down, only numeric-indicator switches -->
              <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
            {:else}
              <Icon name="three-dots-vertical" />
            {/if}
          </Button>
        </th>
        <th scope="col">
          Total Accelerators
          <Button
            color={sorting.field == "totalAccs" ? "primary" : "light"}
            size="sm"
            onclick={() => changeSorting("totalAccs")}
          >
            {#if sorting?.field == "totalAccs"}
              <!-- Note on Icon-Name: Arrow-indicator always down, only numeric-indicator switches -->
              <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
            {:else}
              <Icon name="three-dots-vertical" />
            {/if}
          </Button>
        </th>
      {/if}
      <th scope="col">
        Total Walltime
        <Button
          color={sorting.field == "totalWalltime" ? "primary" : "light"}
          size="sm"
          onclick={() => changeSorting("totalWalltime")}
        >
          {#if sorting?.field == "totalWalltime"}
            <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
          {:else}
            <Icon name="three-dots-vertical" />
          {/if}
        </Button>
      </th>
      <th scope="col">
        Total Core Hours
        <Button
          color={sorting.field == "totalCoreHours" ? "primary" : "light"}
          size="sm"
          onclick={() => changeSorting("totalCoreHours")}
        >
          {#if sorting?.field == "totalCoreHours"}
            <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
          {:else}
            <Icon name="three-dots-vertical" />
          {/if}
        </Button>
      </th>
      <th scope="col">
        Total Accelerator Hours
        <Button
          color={sorting.field == "totalAccHours" ? "primary" : "light"}
          size="sm"
          onclick={() => changeSorting("totalAccHours")}
        >
          {#if sorting?.field == "totalAccHours"}
            <Icon name={`sort-numeric-${sorting?.direction == 'desc' ? 'down-alt' : 'down'}`} />
          {:else}
            <Icon name="three-dots-vertical" />
          {/if}
        </Button>
      </th>
    </tr>
  </thead>
  <tbody>
    {#if $stats.fetching}
      <tr>
        <td colspan={numCols} style="text-align: center;"><Spinner secondary /></td>
      </tr>
    {:else if $stats.error}
      <tr>
        <td colspan={numCols}
          ><Card body color="danger" class="mb-3">{$stats.error.message}</Card
          ></td
        >
      </tr>
    {:else if $stats.data}
      {#each sort($stats.data.rows, sorting, nameFilter) as row (row.id)}
        <tr>
          <td>
            {#if type == "USER"}
              <a href="/monitoring/user/{row.id}"
                >{scrambleNames ? scramble(row.id) : row.id}</a
              >
            {:else if type == "PROJECT"}
              <a href="/monitoring/jobs/?project={row.id}"
                >{scrambleNames ? scramble(row.id) : row.id}</a
              >
            {:else}
              {row.id}
            {/if}
          </td>
          {#if type == "USER"}
            <td
              >{scrambleNames
                ? scramble(row?.name ? row.name : "-")
                : row?.name
                  ? row.name
                  : "-"}</td
            >
          {/if}
          <td>{row.totalJobs}</td>
          <td>{row.shortJobs}</td>
          {#if fetchRunning}
            <td>{row.totalCores}</td>
            <td>{row.totalAccs}</td>
          {/if}
          <td>{row.totalWalltime}</td>
          <td>{row.totalCoreHours}</td>
          <td>{row.totalAccHours}</td>
        </tr>
      {:else}
        <tr>
          <td colspan={numCols}><i>No {type.toLowerCase()}s/jobs found</i></td>
        </tr>
      {/each}
    {/if}
  </tbody>
</Table>
