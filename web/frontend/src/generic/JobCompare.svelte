<!--
    @component jobCompare component; compares jobs according to set filters

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
    // mutationStore,
  } from "@urql/svelte";
  import { Row, Col, Card, Spinner } from "@sveltestrap/sveltestrap";
  import { formatTime } from "./units.js";
  import Comparogram from "./plots/Comparogram.svelte";

  const ccconfig = getContext("cc-config"),
    // initialized = getContext("initialized"),
    globalMetrics = getContext("globalMetrics");

  const equalsCheck = (a, b) => {
    return JSON.stringify(a) === JSON.stringify(b);
  }

  export let matchedCompareJobs = 0;
  export let filterBuffer = [];
  export let metrics = ccconfig.plot_list_selectedMetrics;

  let filter = [...filterBuffer];
  let comparePlotData = {};
  let jobIds = [];
  const sorting = { field: "startTime", type: "col", order: "DESC" };

  /* GQL */

  const client = getContextClient();
  // Pull All Series For Metrics Statistics Only On Node Scope
  const compareQuery = gql`
  query ($filter: [JobFilter!]!, $metrics: [String!]!) {
    jobsMetricStats(filter: $filter, metrics: $metrics) {
      jobId
      startTime
      duration
      stats {
        name
        data {
          min
          avg
          max
        }
      }
    }
  }
  `;

  /* REACTIVES */

  $: compareData = queryStore({
    client: client,
    query: compareQuery,
    variables:{ filter, metrics },
  });

  $: matchedCompareJobs = $compareData.data != null ? $compareData.data.jobsMetricStats.length : -1;
  $: if ($compareData.data != null) {
    jobIds = [];
    comparePlotData = {}
    jobs2uplot($compareData.data.jobsMetricStats, metrics)
  }

 /* FUNCTIONS */
   // Force refresh list with existing unchanged variables (== usually would not trigger reactivity)
   export function refreshJobs() {
    compareData = queryStore({
      client: client,
      query: compareQuery,
      variables: { filter, metrics },
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
  }

  function jobs2uplot(jobs, metrics) {
    // Prep
    for (let m of metrics) {
      // Get Unit
      const rawUnit = globalMetrics.find((gm) => gm.name == m)?.unit
      const metricUnit = (rawUnit?.prefix ? rawUnit.prefix : "") + (rawUnit?.base ? rawUnit.base : "")
      // Init
      comparePlotData[m] = {unit: metricUnit, data: [[],[],[],[],[],[]]} // data: [X, XST, XRT, YMIN, YAVG, YMAX]
    }

    // Iterate jobs if exists
    if (jobs) {
        let plotIndex = 0
        jobs.forEach((j) => {
            jobIds.push(j.jobId)
            for (let s of j.stats) {
              comparePlotData[s.name].data[0].push(plotIndex)
              comparePlotData[s.name].data[1].push(j.startTime)
              comparePlotData[s.name].data[2].push(j.duration)
              comparePlotData[s.name].data[3].push(s.data.min)
              comparePlotData[s.name].data[4].push(s.data.avg)
              comparePlotData[s.name].data[5].push(s.data.max)
            }
            plotIndex++
        })
    }
}

  // Adapt for Persisting Job Selections in DB later down the line
  // const updateConfigurationMutation = ({ name, value }) => {
  //   return mutationStore({
  //     client: client,
  //     query: gql`
  //       mutation ($name: String!, $value: String!) {
  //         updateConfiguration(name: $name, value: $value)
  //       }
  //     `,
  //     variables: { name, value },
  //   });
  // };

  // function updateConfiguration(value, page) {
  //   updateConfigurationMutation({
  //     name: "plot_list_jobsPerPage",
  //     value: value,
  //   }).subscribe((res) => {
  //     if (res.fetching === false && !res.error) {
  //       jobs = [] // Empty List
  //       paging = { itemsPerPage: value, page: page }; // Trigger reload of jobList
  //     } else if (res.fetching === false && res.error) {
  //       throw res.error;
  //     }
  //   });
  // }

</script>

{#if $compareData.fetching}
  <Row>
    <Col>
      <Spinner secondary />
    </Col>
  </Row>
{:else if $compareData.error}
  <Row>
    <Col>
      <Card body color="danger" class="mb-3"
        ><h2>{$compareData.error.message}</h2></Card
      >
    </Col>
  </Row>
{:else}
  {#each metrics as m}
    <Comparogram
      title={'Compare '+ m}
      xlabel="JobIds"
      xticks={jobIds}
      ylabel={m}
      metric={m}
      yunit={comparePlotData[m].unit}
      data={comparePlotData[m].data}
    />
  {/each}
  <hr/><hr/>
  {#each $compareData.data.jobsMetricStats as job, jindex (job.jobId)}
    <Row>
      <Col><b>{jindex}: <i>{job.jobId}</i></b></Col>
      <Col><i>{new Date(job.startTime * 1000).toISOString()}</i></Col>
      <Col><i>{formatTime(job.duration)}</i></Col>
      {#each job.stats as stat (stat.name)}
        <Col><b>{stat.name}</b></Col>
        <Col>Min {stat.data.min}</Col>
        <Col>Avg {stat.data.avg}</Col>
        <Col>Max {stat.data.max}</Col>
      {/each}
    </Row>
    <hr/>
  {:else}
    <div> No jobs found </div>
  {/each}
{/if}