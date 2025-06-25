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

  /* Svelte 5 Props */
  let { user, filterPresets } = $props();

  /* Const Init */
  const { query: initq } = init();
  const ccconfig = getContext("cc-config");
  const client = getContextClient();
  const durationBinOptions = ["1m","10m","1h","6h","12h"];
  const metricBinOptions = [10, 20, 50, 100];

  /* State Init */
  // List & Control Vars
  let filterComponent = $state(); // see why here: https://stackoverflow.com/questions/58287729/how-can-i-export-a-function-from-a-svelte-component-that-changes-a-value-in-the
  let jobFilters = $state([]);
  let jobList = $state(null);
  let matchedListJobs = $state(0);
  let isSortingOpen = $state(false);
  let isMetricsSelectionOpen = $state(false);
  let sorting = $state({ field: "startTime", type: "col", order: "DESC" });
  let selectedCluster = $state(filterPresets?.cluster ? filterPresets.cluster : null);
  let selectedHistogramsBuffer = $state({ all: (ccconfig['user_view_histogramMetrics'] || []) })
  let metrics = $state(filterPresets.cluster
    ? ccconfig[`plot_list_selectedMetrics:${filterPresets.cluster}`] ||
      ccconfig.plot_list_selectedMetrics
    : ccconfig.plot_list_selectedMetrics
  );
  let showFootprint = $state(filterPresets.cluster
    ? !!ccconfig[`plot_list_showFootprint:${filterPresets.cluster}`]
    : !!ccconfig.plot_list_showFootprint
  );

  // Histogram Vars
  let isHistogramSelectionOpen = $state(false);
  let numDurationBins = $state("1h");
  let numMetricBins = $state(10);

  // Compare Vars (TODO)
  // let jobCompare = $state(null);
  // let showCompare = $state(false);
  // let selectedJobs = $state([]);
  // let filterBuffer = $state([]);
  // let matchedCompareJobs = $state(0);

  /* Derived Vars */
  let selectedHistograms = $derived(selectedCluster ? selectedHistogramsBuffer[selectedCluster] : selectedHistogramsBuffer['all']);
  let stats = $derived(
    queryStore({
      client: client,
      query: gql`
        query ($jobFilters: [JobFilter!]!, $selectedHistograms: [String!], $numDurationBins: String, $numMetricBins: Int) {
          jobsStatistics(filter: $jobFilters, metrics: $selectedHistograms, numDurationBins: $numDurationBins , numMetricBins: $numMetricBins ) {
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
      variables: { jobFilters, selectedHistograms, numDurationBins, numMetricBins },
    })
  );

  /* Effect */
  $effect(() => {
    if (!selectedHistogramsBuffer[selectedCluster]) {
      selectedHistogramsBuffer[selectedCluster] = ccconfig[`user_view_histogramMetrics:${selectedCluster}`];
    };
  });

  /* On Mount */
  onMount(() => {
    filterComponent.updateFilters();
    // Why? -> `$derived(ccconfig[$cluster])` only loads array from last Backend-Query if $cluster changed reactively (without reload)
    if (filterPresets?.cluster) {
      selectedHistogramsBuffer[filterPresets.cluster] = ccconfig[`user_view_histogramMetrics:${filterPresets.cluster}`];
    };
  });
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
<Row cols={{ xs: 1, md: 2, lg: 6}} class="mb-3">
  <Col class="mb-2 mb-lg-0">
    <ButtonGroup class="w-100">
      <Button outline color="primary" onclick={() => (isSortingOpen = true)}>
        <Icon name="sort-up" /> Sorting
      </Button>
      <Button
        outline
        color="primary"
        onclick={() => (isMetricsSelectionOpen = true)}
      >
        <Icon name="graph-up" /> Metrics
      </Button>
    </ButtonGroup>
  </Col>
  <Col lg="4" class="mb-1 mb-lg-0">
    <Filters
      bind:this={filterComponent}
      {filterPresets}
      matchedJobs={matchedListJobs}
      startTimeQuickSelect={true}
      applyFilters={(detail) => {
        jobFilters = [...detail.filters, { user: { eq: user.username } }];
        selectedCluster = jobFilters[0]?.cluster
          ? jobFilters[0].cluster.eq
          : null;
        jobList.queryJobs(jobFilters);
      }}
    />
  </Col>
  <Col class="mb-2 mb-lg-0">
    <InputGroup>
      <InputGroupText>
        <Icon name="bar-chart-line-fill" />
      </InputGroupText>
      <InputGroupText>
        Duration Bin Size
      </InputGroupText>
      <Input type="select" bind:value={numDurationBins} style="max-width: 120px;">
        {#each durationBinOptions as dbin}
          <option value={dbin}>{dbin}</option>
        {/each}
      </Input>
    </InputGroup>
  </Col>
  <Col class="mb-2 mb-lg-0">
    <TextFilter
      setFilter={(filter) => filterComponent.updateFilters(filter)}
    />
  </Col>
  <Col class="mb-1 mb-lg-0">
    <Refresher onRefresh={() => {
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
          xlabel="Job Runtimes"
          xunit="Runtime"
          ylabel="Number of Jobs"
          yunit="Jobs"
          usesBins
          xtime
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
<Row>
  <Col xs="12" md="3" lg="2" class="mb-2 mb-md-0">
    <Button
      outline
      color="secondary"
      class="w-100"
      onclick={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
  </Col>
  <Col xs="12" md="9" lg="10" class="mb-2 mb-md-0">
    <InputGroup>
      <InputGroupText>
        <Icon name="bar-chart-line-fill" />
      </InputGroupText>
      <InputGroupText>
        Metric Bins
      </InputGroupText>
      <Input type="select" bind:value={numMetricBins} style="max-width: 120px;">
        {#each metricBinOptions as mbin}
          <option value={mbin}>{mbin}</option>
        {/each}
      </Input>
    </InputGroup>
  </Col>
</Row>
{#if selectedHistograms?.length > 0}
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
    {#snippet gridContent(item)}
      <Histogram
        data={convert2uplot(item.data)}
        title="Distribution of '{item.metric} ({item.stat})' footprints"
        xlabel={`${item.metric} bin maximum ${item?.unit ? `[${item.unit}]` : ``}`}
        xunit={item.unit}
        ylabel="Number of Jobs"
        yunit="Jobs"
        usesBins
      />
    {/snippet}

    {#key $stats.data.jobsStatistics[0].histMetrics}
      <PlotGrid
        items={$stats.data.jobsStatistics[0].histMetrics}
        itemsPerRow={3}
        {gridContent}
      />
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
      bind:matchedListJobs
      bind:metrics
      bind:sorting
      bind:showFootprint
    />
  </Col>
</Row>

<Sorting
  bind:isOpen={isSortingOpen}
  presetSorting={sorting}
  applySorting={(newSort) =>
    sorting = {...newSort}
  }
/>

<MetricSelection
    bind:isOpen={isMetricsSelectionOpen}
    bind:showFootprint
    presetMetrics={metrics}
    cluster={selectedCluster}
    configName="plot_list_selectedMetrics"
    applyMetrics={(newMetrics) => 
      metrics = [...newMetrics]
    }
    footprintSelect
/>

<HistogramSelection
  cluster={selectedCluster}
  bind:isOpen={isHistogramSelectionOpen}
  presetSelectedHistograms={selectedHistograms}
  applyChange={(newSelection) => {
    selectedHistogramsBuffer[selectedCluster || 'all'] = [...newSelection];
  }}
/>
