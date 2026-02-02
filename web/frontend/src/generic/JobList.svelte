<!--
  @component Main jobList component; lists jobs according to set filters

  Properties:
  - `sorting Object?`: Currently active sorting [Default: {field: "startTime", type: "col", order: "DESC"}]
  - `matchedListJobs Number?`: Number of matched jobs for selected filters [Bindable, Default: 0]
  - `metrics [String]?`: The currently selected metrics [Default: User-Configured Selection]
  - `showFootprint Bool?`: If to display the jobFootprint component [Default: false]
  - `selectedJobs [Number]?`: IDs of jobs selected for job comparison [Bindable, Default: []]
  - `filterBuffer [Object]?`: Latest selected filters to keep for view switch to job compare [Default: []]

  Functions:
  - `refreshJobs()`: Load jobs data with unchanged parameters and 'network-only' keyword
  - `refreshAllMetrics()`: Trigger downstream refresh of all running jobs' metric data
  - `queryJobs(filters?: [JobFilter])`: Load jobs data with new filters, starts from page 1
-->

<script>
  import { getContext, untrack } from "svelte";
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
    metrics = [],
    sorting = { field: "startTime", type: "col", order: "DESC" },
    showFootprint = false,
    filterBuffer = [],
  } = $props();

  /* Const Init */
  const ccconfig = getContext("cc-config");
  const initialized = getContext("initialized");
  const globalMetrics = getContext("globalMetrics");
  const usePaging = ccconfig?.jobList_usePaging || false;
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
          shared
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

  /* State Init */
  let headerPaddingTop = $state(0);
  let jobs = $state([]);
  let page = $state(1);
  let itemsPerPage = $state(usePaging ? (ccconfig?.jobList_jobsPerPage || 10) : 10);
  let triggerMetricRefresh = $state(false);
  let tableWidth = $state(0);

  /* Derived */
  let filter = $derived([...filterBuffer]);
  let paging = $derived({ itemsPerPage, page });
  const plotWidth = $derived.by(() => {
    return Math.floor(
      (tableWidth - jobInfoColumnWidth) / (metrics.length + (showFootprint ? 2 : 1)) - 10,
    );
  });
  let jobsStore = $derived(queryStore({
      client: client,
      query: query,
      variables: { paging, sorting, filter },
      requestPolicy: "network-only",
    })
  );

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
        if (scrollTop + clientHeight >= scrollHeight - 100  && $jobsStore?.data?.jobs?.hasNextPage) {
          page += 1
        };
      });
    };
  });

  $effect(() => {
    //Triggers
    filter
    sorting
    // Reset Continous Jobs
    if (!usePaging) {
      page = 1;
    }
  });

  $effect(() => {
    if ($jobsStore?.data) {
      untrack(() => {
        handleJobs($jobsStore.data.jobs.items);
      });
    };
  });

  /* Functions */
  // Force refresh list with existing unchanged variables
  export function refreshJobs() {
    if (usePaging) {
      paging = {...paging}
    } else {
      page = 1;
    }
  };

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
      let minRunningFor = ccconfig.jobList_hideShortRunningJobs;
      if (minRunningFor && minRunningFor > 0) {
        filters.push({ minRunningFor });
      }
      filter = [...filters];
    }
  };

  function handleJobs(newJobs) {
    if (newJobs) {
      if (usePaging) {
        // console.log('New Paging', $state.snapshot(paging))
        jobs = [...newJobs]
      } else {
        if ($state.snapshot(page) == 1) {
          // console.log('Page 1 Reset', [...newJobs])
          jobs = [...newJobs]
        } else {
          // console.log('Add Jobs', $state.snapshot(jobs), [...newJobs])
          jobs = jobs.concat([...newJobs])
        }
      }
      matchedListJobs = $jobsStore.data.jobs.count;
    } else {
      matchedListJobs = -1
    }
  };

  function updateConfiguration(value, newPage) {
    updateConfigurationMutation({
      name: "jobList_jobsPerPage",
      value: value.toString(),
    }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        itemsPerPage =  value
        page = newPage // Trigger reload of jobList
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
            {#if $jobsStore.fetching}
              <Spinner size="sm" style="margin-left:10px;" secondary />
            {/if}
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
            <JobListRow {triggerMetricRefresh} {job} {metrics} {plotWidth} {showFootprint} previousSelect={selectedJobs.includes(job.id)}
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
        updateConfiguration(detail.itemsPerPage, detail.page);
      } else {
        itemsPerPage = detail.itemsPerPage
        page = detail.page
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
