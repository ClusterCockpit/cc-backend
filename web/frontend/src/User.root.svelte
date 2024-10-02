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
  let sorting = { field: "startTime", type: "col", order: "DESC" },
    isSortingOpen = false;
  let metrics = ccconfig.plot_list_selectedMetrics,
    isMetricsSelectionOpen = false;
  let w1,
    w2,
    histogramHeight = 250,
    isHistogramSelectionOpen = false;
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

<Row>
  {#if $initq.fetching}
    <Col>
      <Spinner />
    </Col>
  {:else if $initq.error}
    <Col xs="auto">
      <Card body color="danger">{$initq.error.message}</Card>
    </Col>
  {/if}

  <Col xs="auto">
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

    <Button
      outline
      color="secondary"
      on:click={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
  </Col>
  <Col xs="auto">
    <Filters
      {filterPresets}
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
  <Col xs="auto" style="margin-left: auto;">
    <TextFilter
      on:set-filter={({ detail }) => filterComponent.updateFilters(detail)}
    />
  </Col>
  <Col xs="auto">
    <Refresher on:refresh={() => {
      jobList.refreshJobs()
      jobList.refreshAllMetrics()
    }} />
  </Col>
</Row>
<br />
<Row cols={{ xs: 1, md: 3}}>
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
    <Col class="text-center">
      <div bind:clientWidth={w1}>
        {#key $stats.data.jobsStatistics[0].histDuration}
          <Histogram
            data={convert2uplot($stats.data.jobsStatistics[0].histDuration)}
            width={w1 - 25}
            height={histogramHeight}
            title="Duration Distribution"
            xlabel="Current Runtimes"
            xunit="Hours"
            ylabel="Number of Jobs"
            yunit="Jobs"
          />
        {/key}
      </div>
    </Col>
    <Col class="text-center">
      <div bind:clientWidth={w2}>
        {#key $stats.data.jobsStatistics[0].histNumNodes}
          <Histogram
            data={convert2uplot($stats.data.jobsStatistics[0].histNumNodes)}
            width={w2 - 25}
            height={histogramHeight}
            title="Number of Nodes Distribution"
            xlabel="Allocated Nodes"
            xunit="Nodes"
            ylabel="Number of Jobs"
            yunit="Jobs"
          />
        {/key}
      </div>
    </Col>
  {/if}
</Row>

{#if metricsInHistograms}
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
    {#key $stats.data.jobsStatistics[0].histMetrics}
      <PlotGrid
        let:item
        let:width
        renderFor="user"
        items={$stats.data.jobsStatistics[0].histMetrics}
        itemsPerRow={3}
      >
        <Histogram
          data={convert2uplot(item.data)}
          usesBins={true}
          {width}
          height={250}
          title="Distribution of '{item.metric} ({item.stat})' footprints"
          xlabel={`${item.metric} bin maximum ${item?.unit ? `[${item.unit}]` : ``}`}
          xunit={item.unit}
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      </PlotGrid>
    {/key}
  {/if}
{/if}
<br />
<Row>
  <Col>
    <JobList bind:metrics bind:sorting bind:this={jobList} bind:showFootprint />
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
