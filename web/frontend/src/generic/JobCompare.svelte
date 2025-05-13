<!--
    @component jobCompare component; compares jobs according to set filters or job selection

    Properties:
    - `matchedJobs Number?`: Number of matched jobs for selected filters [Default: 0]
    - `metrics [String]?`: The currently selected metrics [Default: User-Configured Selection]
    - `showFootprint Bool`: If to display the jobFootprint component

    Functions:
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
  import { formatTime, roundTwoDigits } from "./units.js";
  import Comparogram from "./plots/Comparogram.svelte";

  const ccconfig = getContext("cc-config"),
    // initialized = getContext("initialized"),
    globalMetrics = getContext("globalMetrics");

  export let matchedCompareJobs = 0;
  export let metrics = ccconfig.plot_list_selectedMetrics;
  export let filterBuffer = [];

  let filter = [...filterBuffer] || [];
  let comparePlotData = {};
  let compareTableData = [];
  let compareTableSorting = {};
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
    compareTableData = [...$compareData.data.jobsMetricStats];
    jobs2uplot($compareData.data.jobsMetricStats, metrics);
  }

  $: if ((!$compareData.fetching && !$compareData.error) && metrics) {
    // Meta
    compareTableSorting['meta'] = {
      startTime: { dir: "down", active: true },
      duration:  { dir: "up", active: false },
      cluster:   { dir: "up", active: false },
    };
    // Resources
    compareTableSorting['resources'] = {
      Nodes:   { dir: "up", active: false },
      Threads: { dir: "up", active: false },
      Accs:    { dir: "up", active: false },
    };
    // Metrics
    for (let metric of metrics) {
      compareTableSorting[metric] = {
        min: { dir: "up", active: false },
        avg: { dir: "up", active: false },
        max: { dir: "up", active: false },
      };
    }
  }

 /* FUNCTIONS */
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

  function sortBy(key, field) {
    let s = compareTableSorting[key][field];
    if (s.active) {
      s.dir = s.dir == "up" ? "down" : "up";
    } else {
      for (let key in compareTableSorting)
        for (let field in compareTableSorting[key]) compareTableSorting[key][field].active = false;
      s.active = true;
    }
    compareTableSorting = { ...compareTableSorting };

    if (key == 'resources') {
      let longField = "";
      switch (field) {
        case "Nodes": 
          longField = "numNodes"
          break
        case "Threads":
          longField = "numHWThreads"
          break
        case "Accs":
          longField = "numAccelerators"
          break
        default:
          console.log("Unknown Res Field", field)
      } 
      compareTableData = compareTableData.sort((j1, j2) => {
        if (j1[longField] == null || j2[longField] == null) return -1;
        return s.dir != "up" ? j1[longField] - j2[longField] : j2[longField] - j1[longField];
      });
    } else if (key == 'meta') {
      compareTableData = compareTableData.sort((j1, j2) => {
        if (j1[field] == null || j2[field] == null) return -1;
        if (field == 'cluster') {
          let c1 = `${j1.cluster} (${j1.subCluster})`
          let c2 = `${j2.cluster} (${j2.subCluster})`
          return s.dir != "up" ? c1.localeCompare(c2) : c2.localeCompare(c1) 
        } else {
          return s.dir != "up" ? j1[field] - j2[field] : j2[field] - j1[field];
        }
      });
    } else {
      compareTableData = compareTableData.sort((j1, j2) => {
        let s1 = j1.stats.find((m) => m.name == key)?.data;
        let s2 = j2.stats.find((m) => m.name == key)?.data;
        if (s1 == null || s2 == null) return -1;
        return s.dir != "up" ? s1[field] - s2[field] : s2[field] - s1[field];
      });
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
          <th style="width:8%; max-width:10%;">JobID</th>
          <th>StartTime</th>
          <th>Duration</th>
          <th>Cluster</th>
          <th colspan="3">Resources</th>
          {#each metrics as metric}
            <th colspan="3">{metric} {comparePlotData[metric]?.unit? `(${comparePlotData[metric]?.unit})` : ''}</th>
          {/each}
        </tr>
        <!-- Header Row 2: Fields -->
        <tr>
          <th>
            <InputGroup size="sm">
              <Input type="text" bind:value={tableJobIDFilter}/>
              <InputGroupText>
                <Icon name="search"></Icon>
              </InputGroupText>
            </InputGroup>
          </th>
          <th on:click={() => sortBy('meta', 'startTime')}>
            Sort
            <Icon
              name="caret-{compareTableSorting['meta']['startTime'].dir}{compareTableSorting['meta']['startTime']
                .active
                ? '-fill'
                : ''}"
            />
          </th>
          <th on:click={() => sortBy('meta', 'duration')}>
            Sort
            <Icon
              name="caret-{compareTableSorting['meta']['duration'].dir}{compareTableSorting['meta']['duration']
                .active
                ? '-fill'
                : ''}"
            />
          </th>
          <th on:click={() => sortBy('meta', 'cluster')}>
            Sort
            <Icon
              name="caret-{compareTableSorting['meta']['cluster'].dir}{compareTableSorting['meta']['cluster']
                .active
                ? '-fill'
                : ''}"
            />
          </th>
          {#each ["Nodes", "Threads", "Accs"] as res}
            <th on:click={() => sortBy('resources', res)}>
              {res}
              <Icon
                name="caret-{compareTableSorting['resources'][res].dir}{compareTableSorting['resources'][res]
                  .active
                  ? '-fill'
                  : ''}"
              />
            </th>
          {/each}
          {#each metrics as metric}
            {#each ["min", "avg", "max"] as stat}
              <th on:click={() => sortBy(metric, stat)}>
                {stat.charAt(0).toUpperCase() + stat.slice(1)}
                <Icon
                  name="caret-{compareTableSorting[metric][stat].dir}{compareTableSorting[metric][stat]
                    .active
                    ? '-fill'
                    : ''}"
                />
              </th>
            {/each}
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each compareTableData.filter((j) => j.jobId.includes(tableJobIDFilter)) as job (job.id)}
          <tr>
            <td><b><a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a></b></td>
            <td>{new Date(job.startTime * 1000).toLocaleString()}</td>
            <td>{formatTime(job.duration)}</td>
            <td>{job.cluster} ({job.subCluster})</td>
            <td>{job.numNodes}</td>
            <td>{job.numHWThreads}</td>
            <td>{job.numAccelerators}</td>
            {#each metrics as metric}
              <td>{roundTwoDigits(job.stats.find((s) => s.name == metric).data.min)}</td>
              <td>{roundTwoDigits(job.stats.find((s) => s.name == metric).data.avg)}</td>
              <td>{roundTwoDigits(job.stats.find((s) => s.name == metric).data.max)}</td>
            {/each}
          </tr>
        {:else}
          <tr> 
            <td colspan={7 + (metrics.length * 3)}><b>No jobs found.</b></td>
          </tr>
        {/each}
      </tbody>
    </Table>
  </Card>
{/if}