<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import { getContext } from "svelte";
  import {
    Row,
    Col,
    Spinner,
    Card,
    Icon,
    Button,
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    convert2uplot,
  } from "../generic/utils.js";
  import PlotGrid from "../generic/PlotGrid.svelte";
  import Histogram from "../generic/plots/Histogram.svelte";
  import HistogramSelection from "../generic/select/HistogramSelection.svelte";
  import Refresher from "../generic/helper/Refresher.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const ccconfig = getContext("cc-config");
  const client = getContextClient();

  /* State Init */
  let cluster = $state(presetCluster);
  // Histogram
  let isHistogramSelectionOpen = $state(false);
  let from = $state(new Date(Date.now() - (30 * 24 * 60 * 60 * 1000))); // Simple way to retrigger GQL: Jobs Started last Month
  let to = $state(new Date(Date.now()));

  /* Derived */
  let selectedHistograms = $derived(cluster
    ? ccconfig[`statusView_selectedHistograms:${cluster}`] || ( ccconfig['statusView_selectedHistograms'] || [] )
    : ccconfig['statusView_selectedHistograms'] || []);

  // Note: nodeMetrics are requested on configured $timestep resolution
  const metricStatusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $selectedHistograms: [String!]
      ) {
        jobsStatistics(filter: $filter, metrics: $selectedHistograms) {
          histMetrics {
            metric
            unit
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
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster}}, {startTime: { from, to }}],
      selectedHistograms: selectedHistograms,
    },
  }));

</script>

<!-- Loading indicators & Metric Sleect -->
<Row class="justify-content-between">
  <Col class="mb-2 mb-md-0" xs="12" md="5" lg="4" xl="3">
    <Button
      outline
      color="secondary"
      onclick={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
  </Col>
  <Col xs="12" md="5" lg="4" xl="3">
    <Refresher
      initially={120}
      onRefresh={() => {
        from = new Date(Date.now() - (30 * 24 * 60 * 60 * 1000)); // Triggers GQL
        to = new Date(Date.now());
      }}
    />
  </Col>
</Row>

<Row cols={1} class="text-center mt-3">
  <Col>
    {#if $initq.fetching || $metricStatusQuery.fetching}
      <Spinner />
    {:else if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else}
      <!-- ... -->
    {/if}
  </Col>
</Row>
{#if $metricStatusQuery.error}
  <Row cols={1}>
    <Col>
      <Card body color="danger">{$metricStatusQuery.error.message}</Card>
    </Col>
  </Row>
{/if}

{#if $initq.data && $metricStatusQuery.data}
  <!-- Selectable Stats as Histograms : Average Values of Running Jobs -->
  {#if selectedHistograms}
    <!-- Note: Ignore '#snippet' Error in IDE -->
    {#snippet gridContent(item)}
      <Histogram
        data={convert2uplot(item.data)}
        title="Distribution of '{item.metric}' averages"
        xlabel={`${item.metric} bin maximum ${item?.unit ? `[${item.unit}]` : ``}`}
        xunit={item.unit}
        ylabel="Number of Jobs"
        yunit="Jobs"
        usesBins
      />
    {/snippet}
    
    {#key $metricStatusQuery.data.jobsStatistics[0].histMetrics}
      <PlotGrid
        items={$metricStatusQuery.data.jobsStatistics[0].histMetrics}
        itemsPerRow={2}
        {gridContent}
      />
    {/key}
  {/if}
{/if}

<HistogramSelection
  {cluster}
  bind:isOpen={isHistogramSelectionOpen}
  presetSelectedHistograms={selectedHistograms}
  configName="statusView_selectedHistograms"
  applyChange={(newSelection) => {
    selectedHistograms = [...newSelection];
  }}
/>
