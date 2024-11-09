<!--
    @component Main jobList component; lists jobs according to set filters

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
  import { getContext } from "svelte";
  import {
    queryStore,
    gql,
    getContextClient,
    mutationStore,
  } from "@urql/svelte";
  import { Row, Table, Card, Spinner } from "@sveltestrap/sveltestrap";
  import { stickyHeader } from "./utils.js";
  import Pagination from "./joblist/Pagination.svelte";
  import JobListRow from "./joblist/JobListRow.svelte";

  const ccconfig = getContext("cc-config"),
    initialized = getContext("initialized"),
    globalMetrics = getContext("globalMetrics");

  export let sorting = { field: "startTime", type: "col", order: "DESC" };
  export let matchedJobs = 0;
  export let metrics = ccconfig.plot_list_selectedMetrics;
  export let showFootprint;

  let usePaging = ccconfig.job_list_usePaging
  let itemsPerPage = usePaging ? ccconfig.plot_list_jobsPerPage : 10;
  let page = 1;
  let paging = { itemsPerPage, page };
  let filter = [];
  let triggerMetricRefresh = false;

  function getUnit(m) {
    const rawUnit = globalMetrics.find((gm) => gm.name === m)?.unit
    return (rawUnit?.prefix ? rawUnit.prefix : "") + (rawUnit?.base ? rawUnit.base : "")
  } 

  const client = getContextClient();
  const query = gql`
    query (
      $filter: [JobFilter!]!
      $sorting: OrderByInput!
      $paging: PageRequest!
    ) {
      jobs(filter: $filter, order: $sorting, page: $paging) {
        items {
          id
          jobId
          user
          project
          cluster
          subCluster
          startTime
          duration
          numNodes
          numHWThreads
          numAcc
          walltime
          resources {
            hostname
          }
          SMT
          exclusive
          partition
          arrayJobId
          monitoringStatus
          state
          tags {
            id
            type
            name
            scope
          }
          userData {
            name
          }
          metaData
          footprint {
            name
            stat
            value
          }
        }
        count
        hasNextPage
      }
    }
  `;

  $: jobsStore = queryStore({
    client: client,
    query: query,
    variables: { paging, sorting, filter },
  });

  let jobs = []
  $: if ($initialized && $jobsStore.data) {
    jobs = [...$jobsStore.data.jobs.items]
  }

  $: matchedJobs = $jobsStore.data != null ? $jobsStore.data.jobs.count : -1;

  // Force refresh list with existing unchanged variables (== usually would not trigger reactivity)
  export function refreshJobs() {
    jobsStore = queryStore({
      client: client,
      query: query,
      variables: { paging, sorting, filter },
      requestPolicy: "network-only",
    });
  }

  export function refreshAllMetrics() {
    // Refresh Job Metrics (Downstream will only query for running jobs)
    triggerMetricRefresh = true
    setTimeout(function () {
      triggerMetricRefresh = false;
    }, 100);
  }

  // (Re-)query and optionally set new filters; Query will be started reactively.
  export function queryJobs(filters) {
    if (filters != null) {
      let minRunningFor = ccconfig.plot_list_hideShortRunningJobs;
      if (minRunningFor && minRunningFor > 0) {
        filters.push({ minRunningFor });
      }
      filter = filters;
    }
    page = 1;
    paging = paging = { page, itemsPerPage };
  }

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

  function updateConfiguration(value, page) {
    updateConfigurationMutation({
      name: "plot_list_jobsPerPage",
      value: value,
    }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        jobs = [] // Empty List
        paging = { itemsPerPage: value, page: page }; // Trigger reload of jobList
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

  if (!usePaging) {
    let scrollMultiplier = 1
    window.addEventListener('scroll', () => {
      let {
        scrollTop,
        scrollHeight,
        clientHeight
      } = document.documentElement;

      // Add 100 px offset to trigger load earlier
      if (scrollTop + clientHeight >= scrollHeight - 100 && $jobsStore.data != null && $jobsStore.data.jobs.hasNextPage) {
        let pendingPaging = { ...paging }
        scrollMultiplier += 1
        pendingPaging.itemsPerPage = itemsPerPage * scrollMultiplier
        paging = pendingPaging
      };
    });
  };

  let plotWidth = null;
  let tableWidth = null;
  let jobInfoColumnWidth = 250;

  $: if (showFootprint) {
    plotWidth = Math.floor(
      (tableWidth - jobInfoColumnWidth) / (metrics.length + 1) - 10,
    );
  } else {
    plotWidth = Math.floor(
      (tableWidth - jobInfoColumnWidth) / metrics.length - 10,
    );
  }

  let headerPaddingTop = 0;
  stickyHeader(
    ".cc-table-wrapper > table.table >thead > tr > th.position-sticky:nth-child(1)",
    (x) => (headerPaddingTop = x),
  );
</script>

<Row>
  <div class="col cc-table-wrapper" bind:clientWidth={tableWidth}>
    <Table cellspacing="0px" cellpadding="0px">
      <thead>
        <tr>
          <th
            class="position-sticky top-0"
            scope="col"
            style="width: {jobInfoColumnWidth}px; padding-top: {headerPaddingTop}px"
          >
            Job Info
          </th>
          {#if showFootprint}
            <th
              class="position-sticky top-0"
              scope="col"
              style="width: {plotWidth}px; padding-top: {headerPaddingTop}px"
            >
              Job Footprint
            </th>
          {/if}
          {#each metrics as metric (metric)}
            <th
              class="position-sticky top-0 text-center"
              scope="col"
              style="width: {plotWidth}px; padding-top: {headerPaddingTop}px"
            >
              {metric}
              {#if $initialized}
                ({getUnit(metric)})
              {/if}
            </th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#if $jobsStore.error}
          <tr>
            <td colspan={metrics.length + 1}>
              <Card body color="danger" class="mb-3"
                ><h2>{$jobsStore.error.message}</h2></Card
              >
            </td>
          </tr>
        {:else}
          {#each jobs as job (job)}
            <JobListRow bind:triggerMetricRefresh {job} {metrics} {plotWidth} {showFootprint} />
          {:else}
            <tr>
              <td colspan={metrics.length + 1}> No jobs found </td>
            </tr>
          {/each}
        {/if}
        {#if $jobsStore.fetching || !$jobsStore.data}
          <tr>
            <td colspan={metrics.length + 1}>
              <div style="text-align:center;">
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
    itemText="Jobs"
    totalItems={matchedJobs}
    on:update-paging={({ detail }) => {
      if (detail.itemsPerPage != itemsPerPage) {
        updateConfiguration(detail.itemsPerPage.toString(), detail.page);
      } else {
        jobs = []
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
