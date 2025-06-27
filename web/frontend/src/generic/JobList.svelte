<!--
    @component Main jobList component; lists jobs according to set filters

    Properties:
    - `sorting Object?`: Currently active sorting [Default: {field: "startTime", type: "col", order: "DESC"}]
    - `matchedListJobs Number?`: Number of matched jobs for selected filters [Default: 0]
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

  /* Svelte 5 Props */
  let {
    matchedListJobs = $bindable(0),
    selectedJobs = $bindable([]),
    metrics = getContext("cc-config").plot_list_selectedMetrics,
    sorting = { field: "startTime", type: "col", order: "DESC" },
    showFootprint = false,
    filterBuffer = [],
  } = $props();

  /* Const Init */
  const ccconfig = getContext("cc-config");
  const initialized = getContext("initialized");
  const globalMetrics = getContext("globalMetrics");
  const usePaging = ccconfig?.job_list_usePaging || false;
  const jobInfoColumnWidth = 250;
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

  /* Var Init */
  let lastFilter = [];
  let lastSorting = null;

  /* State Init */
  let headerPaddingTop = $state(0);
  let jobs = $state([]);
  let filter = $state([...filterBuffer]);
  let page = $state(1);
  let itemsPerPage = $state(usePaging ? (ccconfig?.plot_list_jobsPerPag || 10) : 10);
  let triggerMetricRefresh = $state(false);
  let tableWidth = $state(0);

  /* Derived */
  let paging = $derived({ itemsPerPage, page });
  const plotWidth = $derived.by(() => {
    return Math.floor(
      (tableWidth - jobInfoColumnWidth) / (metrics.length + (showFootprint ? 1 : 0)) - 10,
    );
  });
  let jobsStore = $derived(queryStore({
      client: client,
      query: query,
      variables: { paging, sorting, filter },
    })
  );

  /* Effects */
  $effect(() => {
    if ($jobsStore?.data) {
      matchedListJobs = $jobsStore.data.jobs.count;
    } else {
      matchedListJobs = -1
    }
  });

  $effect(() => {
    if (!usePaging) {
      window.addEventListener('scroll', () => {
        let {
          scrollTop,
          scrollHeight,
          clientHeight
        } = document.documentElement;

        // Add 100 px offset to trigger load earlier
        if (scrollTop + clientHeight >= scrollHeight - 100  && $jobsStore?.data?.jobs?.hasNextPage) {
          page += 1
        };
      });
    };
  });

  $effect(() => {
    // Triggers (Except Paging)
    sorting
    filter
    // Continous Scroll: Reset jobs and paging if parameters change: Existing entries will not match new selections
    if (!usePaging) {
      jobs = [];
      page = 1;
    }
  });

  $effect(() => {
    if ($initialized && $jobsStore?.data) {
      if (usePaging) {
        jobs = [...$jobsStore.data.jobs.items]
      } else { // Prevents jump to table head in continiuous mode, only if no change in sort or filter
        if (equalsCheck(filter, lastFilter) && equalsCheck(sorting, lastSorting)) {
          // console.log('Both Equal: Continuous Addition ... Set None')
          jobs = jobs.concat([...$jobsStore.data.jobs.items])
        } else if (equalsCheck(filter, lastFilter)) {
          // console.log('Filter Equal: Continuous Reset ... Set lastSorting')
          lastSorting = { ...sorting }
          jobs = [...$jobsStore.data.jobs.items]
        } else if (equalsCheck(sorting, lastSorting)) {
          // console.log('Sorting Equal: Continuous Reset ... Set lastFilter')
          lastFilter = [ ...filter ]
          jobs = [...$jobsStore.data.jobs.items]
        } else {
          // console.log('None Equal: Continuous Reset ... Set lastBoth')
          lastSorting = { ...sorting }
          lastFilter = [ ...filter ]
          jobs = [...$jobsStore.data.jobs.items]
        }
      }
    };
  });

  /* Functions */
  // Force refresh list with existing unchanged variables (== usually would not trigger reactivity)
  export function refreshJobs() {
    if (!usePaging) {
      jobs = []; // Empty Joblist before refresh, prevents infinite buildup
      page = 1;
    }
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
  };

  function updateConfiguration(value, newPage) {
    updateConfigurationMutation({
      name: "plot_list_jobsPerPage",
      value: value,
    }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        jobs = [] // Empty List
        paging = { itemsPerPage: value, page: newPage }; // Trigger reload of jobList
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  };

  function getUnit(m) {
    const rawUnit = globalMetrics.find((gm) => gm.name === m)?.unit
    return (rawUnit?.prefix ? rawUnit.prefix : "") + (rawUnit?.base ? rawUnit.base : "")
  };

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

  const equalsCheck = (a, b) => {
    return JSON.stringify(a) === JSON.stringify(b);
  }

  /* Init Header */
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
          {#each jobs as job (job.id)}
            <JobListRow bind:triggerMetricRefresh {job} {metrics} {plotWidth} {showFootprint} previousSelect={selectedJobs.includes(job.id)}
              selectJob={(detail) => selectedJobs = [...selectedJobs, detail]}
              unselectJob={(detail) => selectedJobs = selectedJobs.filter(item => item !== detail)}
            />
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
    {page}
    {itemsPerPage}
    itemText="Jobs"
    totalItems={matchedListJobs}
    updatePaging={(detail) => {
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
