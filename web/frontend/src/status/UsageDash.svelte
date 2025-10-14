<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
    Spinner,
    Card,
    Table,
    Icon,
    Tooltip,
    Input,
    InputGroup,
    InputGroupText
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
    convert2uplot,
  } from "../generic/utils.js";
  import Pie, { colors } from "../generic/plots/Pie.svelte";
  import Histogram from "../generic/plots/Histogram.svelte";
  import Refresher from "../generic/helper/Refresher.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
    useCbColors = false,
    useAltColors = false
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const client = getContextClient();
  const durationBinOptions = ["1m","10m","1h","6h","12h"];

  /* State Init */
  let cluster = $state(presetCluster)
  let pagingState = $state({page: 1, itemsPerPage: 10}) // Top 10
  let selectedHistograms = $state([]) // Dummy For Refresh 
  let colWidthJobs = $state(0);
  let colWidthNodes = $state(0);
  let colWidthAccs = $state(0);
  let numDurationBins = $state("1h");

  /* Derived */
  const topJobsQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
      ) {
        topUser: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALJOBS
          groupBy: USER
        ) {
          id
          name
          totalJobs
        }
        topProjects: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALJOBS
          groupBy: PROJECT
        ) {
          id
          totalJobs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster} }],
      paging: pagingState // Top 10
    },
    requestPolicy: "network-only"
  }));

  const topNodesQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
      ) {
        topUser: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALNODES
          groupBy: USER
        ) {
          id
          name
          totalNodes
        }
        topProjects: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALNODES
          groupBy: PROJECT
        ) {
          id
          totalNodes
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      paging: pagingState
    },
    requestPolicy: "network-only"
  }));

  const topAccsQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
      ) {
        topUser: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALACCS
          groupBy: USER
        ) {
          id
          name
          totalAccs
        }
        topProjects: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: TOTALACCS
          groupBy: PROJECT
        ) {
          id
          totalAccs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      paging: pagingState
    },
    requestPolicy: "network-only"
  }));

  // Note: nodeMetrics are requested on configured $timestep resolution
  const nodeStatusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $selectedHistograms: [String!]
        $numDurationBins: String
      ) {
        jobsStatistics(filter: $filter, metrics: $selectedHistograms, numDurationBins: $numDurationBins) {
          histDuration {
            count
            value
          }
          histNumNodes {
            count
            value
          }
          histNumAccs {
            count
            value
          }
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      selectedHistograms: selectedHistograms, // No Metrics requested for node hardware stats
      numDurationBins: numDurationBins,
    },
    requestPolicy: "network-only"
  }));

  /* Functions */
  function legendColors(targetIdx) {
    // Reuses first color if targetIdx overflows
    let c;
      if (useCbColors) {
        c = [...colors['colorblind']];
      } else if (useAltColors) {
        c = [...colors['alternative']];
      } else {
        c = [...colors['default']];
      }
    return  c[(c.length + targetIdx) % c.length];
  }
</script>

<!-- Refresher and space for other options -->
<Row class="justify-content-between">
    <Col class="mb-2 mb-md-0" xs="12" md="5" lg="4" xl="3">
    <InputGroup>
      <InputGroupText>
        <Icon name="bar-chart-line-fill" />
      </InputGroupText>
      <InputGroupText>
        Duration Bin Size
      </InputGroupText>
      <Input type="select" bind:value={numDurationBins}>
        {#each durationBinOptions as dbin}
          <option value={dbin}>{dbin}</option>
        {/each}
      </Input>
    </InputGroup>
  </Col>
  <Col xs="12" md="5" lg="4" xl="3">
    <Refresher
      initially={120}
      onRefresh={() => {
        pagingState = { page:1, itemsPerPage: 10 };
        selectedHistograms = [...$state.snapshot(selectedHistograms)];
      }}
    />
  </Col>
</Row>

<hr/>

<!-- Job Duration, Top Users and Projects-->
{#if $topJobsQuery.fetching || $nodeStatusQuery.fetching}
  <Spinner />
{:else if $topJobsQuery.data && $nodeStatusQuery.data}
  <Row>
    <Col xs="12" lg="4" class="p-2">
      {#key $nodeStatusQuery.data.jobsStatistics[0].histDuration}
        <Histogram
          data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histDuration)}
          title="Duration Distribution"
          xlabel="Current Job Runtimes"
          xunit="Runtime"
          ylabel="Number of Jobs"
          yunit="Jobs"
          height="275"
          usesBins
          xtime
        />
      {/key}
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <div bind:clientWidth={colWidthJobs}>
        <h4 class="text-center">
          Top Users: Jobs
        </h4>
        <Pie
          {useAltColors}
          canvasId="hpcpie-jobs-users"
          size={colWidthJobs * 0.75}
          sliceLabel="Jobs"
          quantities={$topJobsQuery.data.topUser.map(
            (tu) => tu['totalJobs'],
          )}
          entities={$topJobsQuery.data.topUser.map((tu) => scrambleNames ? scramble(tu.id) : tu.id)}
        />
      </div>
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <Table>
        <tr class="mb-2">
          <th></th>
          <th style="padding-left: 0.5rem;">User</th>
          <th>Jobs</th>
        </tr>
        {#each $topJobsQuery.data.topUser as tu, i}
          <tr>
            <td><Icon name="circle-fill" style="color: {legendColors(i)};" /></td>
            <td id="topName-jobs-{tu.id}">
              <a target="_blank" href="/monitoring/user/{tu.id}?cluster={cluster}&state=running"
                >{scrambleNames ? scramble(tu.id) : tu.id}
              </a>
            </td>
            {#if tu?.name}
              <Tooltip
                target={`topName-jobs-${tu.id}`}
                placement="left"
                >{scrambleNames ? scramble(tu.name) : tu.name}</Tooltip
              >
            {/if}
            <td>{tu['totalJobs']}</td>
          </tr>
        {/each}
      </Table>
    </Col>

    <Col xs="6" md="3" lg="2" class="p-2">
      <h4 class="text-center">
        Top Projects: Jobs
      </h4>
      <Pie
        {useAltColors}
        canvasId="hpcpie-jobs-projects"
        size={colWidthJobs * 0.75}
        sliceLabel={'Jobs'}
        quantities={$topJobsQuery.data.topProjects.map(
          (tp) => tp['totalJobs'],
        )}
        entities={$topJobsQuery.data.topProjects.map((tp) => scrambleNames ? scramble(tp.id) : tp.id)}
      />
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <Table>
        <tr class="mb-2">
          <th></th>
          <th style="padding-left: 0.5rem;">Project</th>
          <th>Jobs</th>
        </tr>
        {#each $topJobsQuery.data.topProjects as tp, i}
          <tr>
            <td><Icon name="circle-fill" style="color: {legendColors(i)};" /></td>
            <td>
              <a target="_blank" href="/monitoring/jobs/?cluster={cluster}&state=running&project={tp.id}&projectMatch=eq"
                >{scrambleNames ? scramble(tp.id) : tp.id}
              </a>
            </td>
            <td>{tp['totalJobs']}</td>
          </tr>
        {/each}
      </Table>
    </Col>
  </Row>
{:else}
  <Card class="mx-4" body color="warning">Cannot render job status charts: No data!</Card>
{/if}

<hr/>

<!-- Node Distribution, Top Users and Projects-->
{#if $topNodesQuery.fetching || $nodeStatusQuery.fetching}
  <Spinner />
{:else if $topNodesQuery.data && $nodeStatusQuery.data}
  <Row>
    <Col xs="12" lg="4" class="p-2">
      <Histogram
        data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumNodes)}
        title="Number of Nodes Distribution"
        xlabel="Allocated Nodes"
        xunit="Nodes"
        ylabel="Number of Jobs"
        yunit="Jobs"
        height="275"
      />
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <div bind:clientWidth={colWidthNodes}>
        <h4 class="text-center">
          Top Users: Nodes
        </h4>
        <Pie
          {useAltColors}
          canvasId="hpcpie-nodes-users"
          size={colWidthNodes * 0.75}
          sliceLabel="Nodes"
          quantities={$topNodesQuery.data.topUser.map(
            (tu) => tu['totalNodes'],
          )}
          entities={$topNodesQuery.data.topUser.map((tu) => scrambleNames ? scramble(tu.id) : tu.id)}
        />
      </div>
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <Table>
        <tr class="mb-2">
          <th></th>
          <th style="padding-left: 0.5rem;">User</th>
          <th>Nodes</th>
        </tr>
        {#each $topNodesQuery.data.topUser as tu, i}
          <tr>
            <td><Icon name="circle-fill" style="color: {legendColors(i)};" /></td>
            <td id="topName-nodes-{tu.id}">
              <a target="_blank" href="/monitoring/user/{tu.id}?cluster={cluster}&state=running"
                >{scrambleNames ? scramble(tu.id) : tu.id}
              </a>
            </td>
            {#if tu?.name}
              <Tooltip
                target={`topName-nodes-${tu.id}`}
                placement="left"
                >{scrambleNames ? scramble(tu.name) : tu.name}</Tooltip
              >
            {/if}
            <td>{tu['totalNodes']}</td>
          </tr>
        {/each}
      </Table>
    </Col>

    <Col xs="6" md="3" lg="2" class="p-2">
      <h4 class="text-center">
        Top Projects: Nodes
      </h4>
      <Pie
        {useAltColors}
        canvasId="hpcpie-nodes-projects"
        size={colWidthNodes * 0.75}
        sliceLabel={'Nodes'}
        quantities={$topNodesQuery.data.topProjects.map(
          (tp) => tp['totalNodes'],
        )}
        entities={$topNodesQuery.data.topProjects.map((tp) => scrambleNames ? scramble(tp.id) : tp.id)}
      />
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <Table>
        <tr class="mb-2">
          <th></th>
          <th style="padding-left: 0.5rem;">Project</th>
          <th>Nodes</th>
        </tr>
        {#each $topNodesQuery.data.topProjects as tp, i}
          <tr>
            <td><Icon name="circle-fill" style="color: {legendColors(i)};" /></td>
            <td>
              <a target="_blank" href="/monitoring/jobs/?cluster={cluster}&state=running&project={tp.id}&projectMatch=eq"
                >{scrambleNames ? scramble(tp.id) : tp.id}
              </a>
            </td>
            <td>{tp['totalNodes']}</td>
          </tr>
        {/each}
      </Table>
    </Col>
  </Row>
{:else}
  <Card class="mx-4" body color="warning">Cannot render node status charts: No data!</Card>
{/if}

<hr/>

<!-- Acc Distribution, Top Users and Projects-->
{#if $topAccsQuery.fetching || $nodeStatusQuery.fetching}
  <Spinner />
{:else if $topAccsQuery.data && $nodeStatusQuery.data}
  <Row>
    <Col xs="12" lg="4" class="p-2">
      <Histogram
        data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumAccs)}
        title="Number of Accelerators Distribution"
        xlabel="Allocated Accs"
        xunit="Accs"
        ylabel="Number of Jobs"
        yunit="Jobs"
        height="275"
      />
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <div bind:clientWidth={colWidthAccs}>
        <h4 class="text-center">
          Top Users: GPUs
        </h4>
        <Pie
          {useAltColors}
          canvasId="hpcpie-accs-users"
          size={colWidthAccs * 0.75}
          sliceLabel="GPUs"
          quantities={$topAccsQuery.data.topUser.map(
            (tu) => tu['totalAccs'],
          )}
          entities={$topAccsQuery.data.topUser.map((tu) => scrambleNames ? scramble(tu.id) : tu.id)}
        />
      </div>
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <Table>
        <tr class="mb-2">
          <th></th>
          <th style="padding-left: 0.5rem;">User</th>
          <th>GPUs</th>
        </tr>
        {#each $topAccsQuery.data.topUser as tu, i}
          <tr>
            <td><Icon name="circle-fill" style="color: {legendColors(i)};" /></td>
            <td id="topName-accs-{tu.id}">
              <a target="_blank" href="/monitoring/user/{tu.id}?cluster={cluster}&state=running"
                >{scrambleNames ? scramble(tu.id) : tu.id}
              </a>
            </td>
            {#if tu?.name}
              <Tooltip
                target={`topName-accs-${tu.id}`}
                placement="left"
                >{scrambleNames ? scramble(tu.name) : tu.name}</Tooltip
              >
            {/if}
            <td>{tu['totalAccs']}</td>
          </tr>
        {/each}
      </Table>
    </Col>

    <Col xs="6" md="3" lg="2" class="p-2">
      <h4 class="text-center">
        Top Projects: GPUs
      </h4>
      <Pie
        {useAltColors}
        canvasId="hpcpie-accs-projects"
        size={colWidthAccs * 0.75}
        sliceLabel={'GPUs'}
        quantities={$topAccsQuery.data.topProjects.map(
          (tp) => tp['totalAccs'],
        )}
        entities={$topAccsQuery.data.topProjects.map((tp) => scrambleNames ? scramble(tp.id) : tp.id)}
      />
    </Col>
    <Col xs="6" md="3" lg="2" class="p-2">
      <Table>
        <tr class="mb-2">
          <th></th>
          <th style="padding-left: 0.5rem;">Project</th>
          <th>GPUs</th>
        </tr>
        {#each $topAccsQuery.data.topProjects as tp, i}
          <tr>
            <td><Icon name="circle-fill" style="color: {legendColors(i)};" /></td>
            <td>
              <a target="_blank" href="/monitoring/jobs/?cluster={cluster}&state=running&project={tp.id}&projectMatch=eq"
                >{scrambleNames ? scramble(tp.id) : tp.id}
              </a>
            </td>
            <td>{tp['totalAccs']}</td>
          </tr>
        {/each}
      </Table>
    </Col>
  </Row>
{:else}
  <Card class="mx-4" body color="warning">Cannot render accelerator status charts: No data!</Card>
{/if}