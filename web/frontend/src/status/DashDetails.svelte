<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import {
    getContext
  } from "svelte"
  import {
    init,
  } from "../generic/utils.js";
  import {
    Row,
    Col,
    Card,
    CardBody,
    TabContent,
    TabPane,
    Spinner
  } from "@sveltestrap/sveltestrap";

  import StatusDash from "./dashdetails/StatusDash.svelte";
  import HealthDash from "./dashdetails/HealthDash.svelte";
  import UsageDash from "./dashdetails/UsageDash.svelte";
  import StatisticsDash from "./dashdetails/StatisticsDash.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
  } = $props();

  /*Const Init */
  const { query: initq } = init();
  const useCbColors = getContext("cc-config")?.plotConfiguration_colorblindMode || false

  /* Derived */
  const subClusters = $derived($initq?.data?.clusters?.find((c) => c.name == presetCluster)?.subClusters || []);
</script>

<!-- Loading indicator & Refresh -->

<Row cols={1} class="mb-2">
  <Col>
    <h3 class="mb-0">Current Status of Cluster "{presetCluster.charAt(0).toUpperCase() + presetCluster.slice(1)}"</h3>
  </Col>
</Row>


{#if $initq.fetching}
  <Row cols={1} class="text-center mt-3">
    <Col>
      <Spinner />
    </Col>
  </Row>
{:else if $initq.error}
  <Row cols={1} class="text-center mt-3">
    <Col>  
      <Card body color="danger">{$initq.error.message}</Card>
    </Col>
  </Row>
{:else}
  <Card class="overflow-auto" style="height: auto;">
    <TabContent>
      <TabPane tabId="status-dash" tab="Status" active>
        <CardBody>
          <StatusDash clusters={$initq.data.clusters} {presetCluster}></StatusDash>
        </CardBody>
      </TabPane>

      <TabPane tabId="health-dash" tab="Metric Status">
        <CardBody>
          <HealthDash {presetCluster}></HealthDash>
        </CardBody>
      </TabPane>

      <TabPane tabId="usage-dash" tab="Cluster Usage">
        <CardBody>
          <UsageDash {presetCluster} {useCbColors}></UsageDash>
        </CardBody>
      </TabPane>

      {#if subClusters?.length > 1}
        {#each subClusters.map(sc => sc.name) as scn}
        <TabPane tabId="{scn}-usage-dash" tab="{scn.charAt(0).toUpperCase() + scn.slice(1)} Usage">
          <CardBody>
            <UsageDash {presetCluster} presetSubCluster={scn} {useCbColors}></UsageDash>
          </CardBody>
        </TabPane>
        {/each}
      {/if}
      
      <TabPane tabId="metric-dash" tab="Statistics">
        <CardBody>
          <StatisticsDash {presetCluster} {useCbColors}></StatisticsDash>
        </CardBody>
      </TabPane>
    </TabContent>
  </Card>
{/if}
