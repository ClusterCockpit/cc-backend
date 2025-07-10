<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
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
    // mutationStore,
  } from "@urql/svelte";
  import {
    init,
    convert2uplot,
  } from "../generic/utils.js";
  import PlotGrid from "../generic/PlotGrid.svelte";
  import Histogram from "../generic/plots/Histogram.svelte";
  import HistogramSelection from "../generic/select/HistogramSelection.svelte";

  /* Svelte 5 Props */
  let {
    cluster
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const ccconfig = getContext("cc-config");
  const client = getContextClient();

  /* State Init */

  // Histrogram
  let isHistogramSelectionOpen = $state(false);

  // TODO: Originally Uses User View Selection! -> Change to Status View 
  let selectedHistograms = $state(cluster
    ? ccconfig[`user_view_histogramMetrics:${cluster}`] || ( ccconfig['user_view_histogramMetrics'] || [] )
    : ccconfig['user_view_histogramMetrics'] || []);

  /* Derived */
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
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      selectedHistograms: selectedHistograms,
    },
  }));

  /* Functions */
  // TODO: Originally Uses User View Selection! -> Change to Status View : Adapt Mutations from TopUserSelect
  // function updateTopUserConfiguration(select) {
  //   if (ccconfig[`status_view_selectedHistograms:${cluster}`] != select) {
  //     updateConfigurationMutation({
  //       name: `status_view_selectedHistograms:${cluster}`,
  //       value: JSON.stringify(select),
  //     }).subscribe((res) => {
  //       if (res.fetching === false && res.error) {
  //         throw res.error;
  //       }
  //     });
  //   }
  // };

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

</script>

<!-- Loading indicators & Metric Sleect -->
<Row cols={{ xs: 1 }}>
  <Col class="text-md-end">
    <Button
      outline
      color="secondary"
      onclick={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
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
  applyChange={(newSelection) => {
    selectedHistograms = [...newSelection];
  }}
/>
