<!--
    @component Main user jobs list display component; displays job list and additional information for a given user

    Properties:
    - `user Object`: The GraphQL user object
    - `filterPresets Object`: Optional predefined filter values
 -->

<script>
  import { onMount, getContext } from "svelte";
  import {
    Table,
    Row,
    Col,
    Button,
    ButtonGroup,
    Icon,
    Card,
    Spinner,
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    convert2uplot,
    scramble,
    scrambleNames,
  } from "./generic/utils.js";
  import JobList from "./generic/JobList.svelte";
  import Filters from "./generic/Filters.svelte";
  import PlotGrid from "./generic/PlotGrid.svelte";
  import Histogram from "./generic/plots/Histogram.svelte";
  import MetricSelection from "./generic/select/MetricSelection.svelte";
  import HistogramSelection from "./generic/select/HistogramSelection.svelte";
  import Sorting from "./generic/select/SortSelection.svelte";
  import TextFilter from "./generic/helper/TextFilter.svelte"
  import Refresher from "./generic/helper/Refresher.svelte";

  const { query: initq } = init();

  const ccconfig = getContext("cc-config");

  export let user;
  export let filterPresets;

  let filterComponent; // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
  let jobList;
  let jobFilters = [];
  let matchedJobs = 0;
  let sorting = { field: "startTime", type: "col", order: "DESC" },
    isSortingOpen = false;
  let metrics = ccconfig.plot_list_selectedMetrics,
    isMetricsSelectionOpen = false;
  let isHistogramSelectionOpen = false;
  let selectedCluster = filterPresets?.cluster ? filterPresets.cluster : null;
  let showFootprint = filterPresets.cluster
    ? !!ccconfig[`plot_list_showFootprint:${filterPresets.cluster}`]
    : !!ccconfig.plot_list_showFootprint;

  $: metricsInHistograms = selectedCluster
    ? ccconfig[`user_view_histogramMetrics:${selectedCluster}`] || []
    : ccconfig.user_view_histogramMetrics || [];

  const client = getContextClient();
  $: stats = queryStore({
    client: client,
    query: gql`
      query ($jobFilters: [JobFilter!]!, $metricsInHistograms: [String!]) {
        jobsStatistics(filter: $jobFilters, metrics: $metricsInHistograms) {
          totalJobs
          shortJobs
          totalWalltime
          totalCoreHours
          histDuration {
            count
            value
          }
          histNumNodes {
            count
            value
          }
          histMetrics {
            metric
            unit
            stat
            data {
              min
              max
              count
              bin
            }
          }
        }
      }
    `,
    variables: { jobFilters, metricsInHistograms },
  });

  onMount(() => filterComponent.updateFilters());
</script>

<!-- ROW1: Status-->
{#if $initq.fetching}
  <Row>
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if $initq.error}
  <Row>
    <Col>
      <Card body color="danger">{$initq.error.message}</Card>
    </Col>
  </Row>
{/if}

<!-- ROW2: Tools-->
<Row cols={{ xs: 1, md: 2, lg: 4}} class="mb-3">
  <Col lg="2" class="mb-2 mb-lg-0">
    <ButtonGroup class="w-100">
      <Button outline color="primary" on:click={() => (isSortingOpen = true)}>
        <Icon name="sort-up" /> Sorting
      </Button>
      <Button
        outline
        color="primary"
        on:click={() => (isMetricsSelectionOpen = true)}
      >
        <Icon name="graph-up" /> Metrics
      </Button>
    </ButtonGroup>
  </Col>
  <Col lg="4" xl="6" class="mb-1 mb-lg-0">
    <Filters
      {filterPresets}
      {matchedJobs}
      startTimeQuickSelect={true}
      bind:this={filterComponent}
      on:update-filters={({ detail }) => {
        jobFilters = [...detail.filters, { user: { eq: user.username } }];
        selectedCluster = jobFilters[0]?.cluster
          ? jobFilters[0].cluster.eq
          : null;
        jobList.queryJobs(jobFilters);
      }}
    />
  </Col>
  <Col lg="3" xl="2" class="mb-2 mb-lg-0">
    <TextFilter
      on:set-filter={({ detail }) => filterComponent.updateFilters(detail)}
    />
  </Col>
  <Col lg="3" xl="2" class="mb-1 mb-lg-0">
    <Refresher on:refresh={() => {
      jobList.refreshJobs()
      jobList.refreshAllMetrics()
    }} />
  </Col>
</Row>

<!-- ROW3: Base Information-->
<Row cols={{ xs: 1, md: 3}} class="mb-2">
  {#if $stats.error}
    <Col>
      <Card body color="danger">{$stats.error.message}</Card>
    </Col>
  {:else if !$stats.data}
    <Col>
      <Spinner secondary />
    </Col>
  {:else}
    <Col>
      <Table>
        <tbody>
          <tr>
            <th scope="row">Username</th>
            <td>{scrambleNames ? scramble(user.username) : user.username}</td>
          </tr>
          {#if user.name}
            <tr>
              <th scope="row">Name</th>
              <td>{scrambleNames ? scramble(user.name) : user.name}</td>
            </tr>
          {/if}
          {#if user.email}
            <tr>
              <th scope="row">Email</th>
              <td>{user.email}</td>
            </tr>
          {/if}
          <tr>
            <th scope="row">Total Jobs</th>
            <td>{$stats.data.jobsStatistics[0].totalJobs}</td>
          </tr>
          <tr>
            <th scope="row">Short Jobs</th>
            <td>{$stats.data.jobsStatistics[0].shortJobs}</td>
          </tr>
          <tr>
            <th scope="row">Total Walltime</th>
            <td>{$stats.data.jobsStatistics[0].totalWalltime}</td>
          </tr>
          <tr>
            <th scope="row">Total Core Hours</th>
            <td>{$stats.data.jobsStatistics[0].totalCoreHours}</td>
          </tr>
        </tbody>
      </Table>
    </Col>
    <Col class="px-1">
      {#key $stats.data.jobsStatistics[0].histDuration}
        <Histogram
          data={convert2uplot($stats.data.jobsStatistics[0].histDuration)}
          title="Duration Distribution"
          xlabel="Current Runtimes"
          xunit="Hours"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
    <Col class="px-1">
      {#key $stats.data.jobsStatistics[0].histNumNodes}
        <Histogram
          data={convert2uplot($stats.data.jobsStatistics[0].histNumNodes)}
          title="Number of Nodes Distribution"
          xlabel="Allocated Nodes"
          xunit="Nodes"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
  {/if}
</Row>

<!-- ROW4+5: Selectable Histograms -->
<Row cols={{ xs: 1, md: 5}}>
  <Col>
    <Button
      outline
      color="secondary"
      on:click={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
  </Col>
</Row>
{#if metricsInHistograms?.length > 0}
  {#if $stats.error}
    <Row>
      <Col>
        <Card body color="danger">{$stats.error.message}</Card>
      </Col>
    </Row>
  {:else if !$stats.data}
    <Row>
      <Col>
        <Spinner secondary />
      </Col>
    </Row>
  {:else}
    <hr class="my-2"/>
    {#key $stats.data.jobsStatistics[0].histMetrics}
      <PlotGrid
        let:item
        items={$stats.data.jobsStatistics[0].histMetrics}
        itemsPerRow={3}
      >
        <Histogram
          data={convert2uplot(item.data)}
          usesBins={true}
          title="Distribution of '{item.metric} ({item.stat})' footprints"
          xlabel={`${item.metric} bin maximum ${item?.unit ? `[${item.unit}]` : ``}`}
          xunit={item.unit}
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      </PlotGrid>
    {/key}
  {/if}
{:else}
  <Row class="mt-2">
    <Col>
      <Card body>No footprint histograms selected.</Card>
    </Col>
  </Row>
{/if}

<!-- ROW6: JOB LIST-->
<Row class="mt-3">
  <Col>
    <JobList
      bind:this={jobList} 
      bind:matchedJobs
      bind:metrics
      bind:sorting
      bind:showFootprint
    />
  </Col>
</Row>

<Sorting bind:sorting bind:isOpen={isSortingOpen} />

<MetricSelection
  bind:cluster={selectedCluster}
  configName="plot_list_selectedMetrics"
  bind:metrics
  bind:isOpen={isMetricsSelectionOpen}
  bind:showFootprint
  footprintSelect={true}
/>

<HistogramSelection
  bind:cluster={selectedCluster}
  bind:metricsInHistograms
  bind:isOpen={isHistogramSelectionOpen}
/>
