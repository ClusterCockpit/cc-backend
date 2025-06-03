<!--
    @component Job-View subcomponent; Wraps the statsTable in a TabPane and contains GQL query for scoped statsData

    Properties:
    - `job Object`: The job object
    - `clusters Object`: The clusters object
    - `tabActive bool`: Boolean if StatsTabe Tab is Active on Creation
 -->

<script>
  import { 
    queryStore,
    gql,
    getContextClient 
  } from "@urql/svelte";
  import { getContext } from "svelte";
  import {
    Card,
    Button,
    Row,
    Col,
    TabPane,
    Spinner,
    Icon
  } from "@sveltestrap/sveltestrap";
  import MetricSelection from "../generic/select/MetricSelection.svelte";
  import StatsTable from "./statstab/StatsTable.svelte";

  export let job;
  export let clusters;
  export let tabActive;

  let loadScopes = false;
  let selectedScopes = [];
  let selectedMetrics = [];
  let totalMetrics = 0; // For Info Only, filled by MetricSelection Component
  let isMetricSelectionOpen = false;

  const client = getContextClient();
  const query = gql`
    query ($dbid: ID!, $selectedMetrics: [String!]!, $selectedScopes: [MetricScope!]!) {
      scopedJobStats(id: $dbid, metrics: $selectedMetrics, scopes: $selectedScopes) {
        name
        scope
        stats {
          hostname
          id
          data {
            min
            avg
            max
          }
        }
      }
    }
  `;

  $: scopedStats = queryStore({
    client: client,
    query: query,
    variables: { dbid: job.id, selectedMetrics, selectedScopes },
  });

  $: if (loadScopes) {
    selectedScopes = ["node", "socket", "core", "hwthread", "accelerator"];
  }

  // Handle Job Query on Init -> is not executed anymore
  getContext("on-init")(() => {
    if (!job) return;

    const pendingMetrics = (
      getContext("cc-config")[`job_view_nodestats_selectedMetrics:${job.cluster}:${job.subCluster}`] ||
      getContext("cc-config")[`job_view_nodestats_selectedMetrics:${job.cluster}`]
    ) || getContext("cc-config")["job_view_nodestats_selectedMetrics"];

    // Select default Scopes to load: Check before if any metric has accelerator scope by default
    const accScopeDefault = [...pendingMetrics].some(function (m) {
      const cluster = clusters.find((c) => c.name == job.cluster);
      const subCluster = cluster.subClusters.find((sc) => sc.name == job.subCluster);
      return subCluster.metricConfig.find((smc) => smc.name == m)?.scope === "accelerator";
    });

    const pendingScopes = ["node"]
    if (job.numNodes === 1) {
      pendingScopes.push("socket")
      pendingScopes.push("core")
      pendingScopes.push("hwthread")
      if (accScopeDefault) { pendingScopes.push("accelerator") }
    }

    selectedMetrics = [...pendingMetrics];
    selectedScopes = [...pendingScopes];
  });

</script>

<TabPane tabId="stats" tab="Statistics Table" class="overflow-x-auto" active={tabActive}>
  <Row>
    <Col class="m-2">
      <Button outline on:click={() => (isMetricSelectionOpen = true)} class="px-2" color="primary" style="margin-right:0.5rem">
        Select Metrics (Selected {selectedMetrics.length} of {totalMetrics} available)
      </Button>
      {#if job.numNodes > 1}
        <Button class="px-2 ml-auto" color="success" outline on:click={() => (loadScopes = !loadScopes)} disabled={loadScopes}>
          {#if !loadScopes}
            <Icon name="plus-square-fill" style="margin-right:0.25rem"/> Add More Scopes
          {:else}
            <Icon name="check-square-fill" style="margin-right:0.25rem"/> OK: Scopes Added
          {/if}
        </Button>
     {/if}
    </Col>
  </Row>
  <hr class="mb-1 mt-1"/>
  <!-- ROW1: Status-->
  {#if $scopedStats.fetching}
    <Row>
      <Col class="m-3" style="text-align: center;">
        <Spinner secondary/>
      </Col>
    </Row>
  {:else if $scopedStats.error}
    <Row>
      <Col class="m-2">
        <Card body color="danger">{$scopedStats.error.message}</Card>
      </Col>
    </Row>
  {:else}
    <StatsTable 
      hosts={job.resources.map((r) => r.hostname).sort()}
      data={$scopedStats?.data?.scopedJobStats}
      {selectedMetrics}
    />
  {/if}
</TabPane>

<MetricSelection
  bind:isOpen={isMetricSelectionOpen}
  bind:totalMetrics
  presetMetrics={selectedMetrics}
  cluster={job.cluster}
  subCluster={job.subCluster}
  configName="job_view_nodestats_selectedMetrics"
  applyMetrics={(newMetrics) => 
    selectedMetrics = [...newMetrics]
  }
/>
