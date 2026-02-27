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
    convert2uplot,
  } from "../../generic/utils.js";
  import PlotGrid from "../../generic/PlotGrid.svelte";
  import Histogram from "../../generic/plots/Histogram.svelte";
  import HistogramSelection from "../../generic/select/HistogramSelection.svelte";
  import Refresher from "../../generic/helper/Refresher.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
    loadMe = false,
  } = $props();

  /* Const Init */
  const ccconfig = getContext("cc-config");
  const globalMetrics = getContext("globalMetrics");
  const client = getContextClient();

  /* State Init */
  // Histogram
  let isHistogramSelectionOpen = $state(false);

  /* Derived */
  let cluster = $derived(presetCluster);
  let selectedHistograms = $derived(cluster
    ? ccconfig[`statusView_selectedHistograms:${cluster}`] || ( ccconfig['statusView_selectedHistograms'] || [] )
    : ccconfig['statusView_selectedHistograms'] || []);

  // Note: nodeMetrics are requested on configured $timestep resolution
  const metricStatusQuery = $derived(loadMe ? queryStore({
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
      filter: [{ state: ["running"] }, { cluster: { eq: cluster} }],
      selectedHistograms: selectedHistograms
    },
    requestPolicy: "network-only"
  }) : null);
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
        selectedHistograms = [...$state.snapshot(selectedHistograms)]
      }}
    />
  </Col>
</Row>

<Row cols={1} class="text-center mt-3">
  {#if $metricStatusQuery?.fetching}
    <Col>
      <Spinner />
    </Col>
  {:else if $metricStatusQuery?.error}
    <Col>  
      <Card body color="danger">{$metricStatusQuery.error.message}</Card>
    </Col>
  {/if}
</Row>

{#if $metricStatusQuery?.data}
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
        enableFlip
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
  {globalMetrics}
  bind:isOpen={isHistogramSelectionOpen}
  presetSelectedHistograms={selectedHistograms}
  configName="statusView_selectedHistograms"
  applyChange={(newSelection) => {
    selectedHistograms = [...newSelection];
  }}
/>
