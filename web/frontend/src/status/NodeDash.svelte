<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
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
  import Histogram from "../generic/plots/Histogram.svelte";

  /* Svelte 5 Props */
  let {
    cluster
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const client = getContextClient();

  /* Derived */
  // Note: nodeMetrics are requested on configured $timestep resolution
  const nodeStatusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $selectedHistograms: [String!]
      ) {
        jobsStatistics(filter: $filter, metrics: $selectedHistograms) {
          histDuration {
            count
            value
          }
          histNumNodes {
            count
            value
          }
          histNumCores {
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
      selectedHistograms: [], // No Metrics requested for node hardware stats
    },
  }));
</script>

{#if $initq.data && $nodeStatusQuery.data}
  <!-- Static Stats as Histograms : Running Duration && Allocated Hardware Counts-->
  <Row cols={{ lg: 2, md: 1 }}>
    <Col class="p-2">
      {#key $nodeStatusQuery.data.jobsStatistics}
        <Histogram
          data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histDuration)}
          title="Duration Distribution"
          xlabel="Current Job Runtimes"
          xunit="Runtime"
          ylabel="Number of Jobs"
          yunit="Jobs"
          usesBins
          xtime
        />
      {/key}
    </Col>
    <Col class="p-2">
      {#key $nodeStatusQuery.data.jobsStatistics}
        <Histogram
          data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumNodes)}
          title="Number of Nodes Distribution"
          xlabel="Allocated Nodes"
          xunit="Nodes"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
  </Row>
  <Row cols={{ lg: 2, md: 1 }}>
    <Col class="p-2">
      {#key $nodeStatusQuery.data.jobsStatistics}
        <Histogram
          data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumCores)}
          title="Number of Cores Distribution"
          xlabel="Allocated Cores"
          xunit="Cores"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
    <Col class="p-2">
      {#key $nodeStatusQuery.data.jobsStatistics}
        <Histogram
          data={convert2uplot($nodeStatusQuery.data.jobsStatistics[0].histNumAccs)}
          title="Number of Accelerators Distribution"
          xlabel="Allocated Accs"
          xunit="Accs"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
  </Row>
{/if}


