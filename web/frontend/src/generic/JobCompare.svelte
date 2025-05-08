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
  import uPlot from "uplot";
  import {
    queryStore,
    gql,
    getContextClient,
    // mutationStore,
  } from "@urql/svelte";
  import { Row, Col, Card, Spinner, Table, Input, InputGroup, InputGroupText, Icon } from "@sveltestrap/sveltestrap";
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
  let jobClusters = [];
  let tableJobIDFilter = "";

  /*uPlot*/
  let plotSync = uPlot.sync("compareJobsView");

  /* GQL */
  const client = getContextClient();  
  // Pull All Series For Metrics Statistics Only On Node Scope
  const compareQuery = gql`
  query ($filter: [JobFilter!]!, $metrics: [String!]!) {
    jobsMetricStats(filter: $filter, metrics: $metrics) {
      id
      jobId
      startTime
      duration
      cluster
      subCluster
      numNodes
      numHWThreads
      numAccelerators
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
    jobClusters = [];
    comparePlotData = {};
    jobs2uplot($compareData.data.jobsMetricStats, metrics);
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
    // Resources Init
    comparePlotData['resources'] = {unit:'', data: [[],[],[],[],[],[]]} // data: [X, XST, XRT, YNODES, YTHREADS, YACCS]
    // Metric Init
    for (let m of metrics) {
      // Get Unit
      const rawUnit = globalMetrics.find((gm) => gm.name == m)?.unit
      const metricUnit = (rawUnit?.prefix ? rawUnit.prefix : "") + (rawUnit?.base ? rawUnit.base : "")
      comparePlotData[m] = {unit: metricUnit, data: [[],[],[],[],[],[]]} // data: [X, XST, XRT, YMIN, YAVG, YMAX]
    }

    // Iterate jobs if exists
    if (jobs) {
        let plotIndex = 0
        jobs.forEach((j) => {
            // Collect JobIDs & Clusters for X-Ticks and Legend
            jobIds.push(j.jobId)
            jobClusters.push(`${j.cluster} ${j.subCluster}`)
            // Resources
            comparePlotData['resources'].data[0].push(plotIndex)
            comparePlotData['resources'].data[1].push(j.startTime)
            comparePlotData['resources'].data[2].push(j.duration)
            comparePlotData['resources'].data[3].push(j.numNodes)
            comparePlotData['resources'].data[4].push(j?.numHWThreads?j.numHWThreads:0)
            comparePlotData['resources'].data[5].push(j?.numAccelerators?j.numAccelerators:0)
            // Metrics
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
  {#key comparePlotData}
    <Row>
      <Col>
        <Comparogram
          title={'Compare Resources'}
          xlabel="JobIDs"
          xticks={jobIds}
          xinfo={jobClusters}
          ylabel={'Resource Counts'}
          data={comparePlotData['resources'].data}
          {plotSync}
          forResources
        />
      </Col>
    </Row>
    {#each metrics as m}
      <Row>
        <Col>
          <Comparogram
            title={`Compare Metric '${m}'`}
            xlabel="JobIDs"
            xticks={jobIds}
            xinfo={jobClusters}
            ylabel={m}
            metric={m}
            yunit={comparePlotData[m].unit}
            data={comparePlotData[m].data}
            {plotSync}
          />
        </Col>
      </Row>
    {/each}
  {/key}
  <hr/>
  <Card>
    <Table hover>
      <thead>
        <!-- Header Row 1 -->
        <tr>
          <th>Index</th>
          <th style="width:10%">JobID</th>
          <th>Cluster</th>
          <th>StartTime</th>
          <th>Duration</th>
          <th colspan="3">Resources</th>
          {#each metrics as metric}
            <th colspan="3">{metric}</th>
          {/each}
        </tr>
        <!-- Header Row 2: Fields -->
        <tr>
          <th/>
          <th style="width:10%">
            <InputGroup>
              <InputGroupText>
                <Icon name="search"></Icon>
              </InputGroupText>
              <Input type="text" bind:value={tableJobIDFilter}/>
            </InputGroup>
          </th>
          <th/>
          <th/>
          <th/>
          {#each ["Nodes", "Threads", "Accs"] as res}
            <th>{res}</th>
          {/each}
          {#each metrics as metric}
            {#each ["min", "avg", "max"] as stat}
              <th>{stat}</th>
            {/each}
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each $compareData.data.jobsMetricStats.filter((j) => j.jobId.includes(tableJobIDFilter)) as job, jindex (job.jobId)}
          <tr>
            <td>{jindex}</td>
            <td><a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a></td>
            <td>{job.cluster} ({job.subCluster})</td>
            <td>{new Date(job.startTime * 1000).toISOString()}</td>
            <td>{formatTime(job.duration)}</td>
            <td>{job.numNodes}</td>
            <td>{job.numHWThreads}</td>
            <td>{job.numAccelerators}</td>
            {#each metrics as metric}
              <td>{job.stats.find((s) => s.name == metric).data.min}</td>
              <td>{job.stats.find((s) => s.name == metric).data.avg}</td>
              <td>{job.stats.find((s) => s.name == metric).data.max}</td>
            {/each}
          </tr>
        {:else}
          <tr> No jobs found </tr>
        {/each}
      </tbody>
    </Table>
  </Card>
{/if}