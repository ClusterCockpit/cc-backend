<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
-->

 <script>
  import {
    getContext
  } from "svelte"
  import {
    Row,
    Col,
    Card,
    CardBody,
    TabContent,
    TabPane
  } from "@sveltestrap/sveltestrap";

  import Refresher from "./generic/helper/Refresher.svelte";
  import StatusDash from "./status/StatusDash.svelte";
  import UsageDash from "./status/UsageDash.svelte";
  import NodeDash from "./status/NodeDash.svelte";
  import StatisticsDash from "./status/StatisticsDash.svelte";
  import DevelDash from "./status/DevelDash.svelte";

  /* Svelte 5 Props */
  let {
    cluster
  } = $props();

  /*Const Init */
  const useCbColors = getContext("cc-config")?.plot_general_colorblindMode || false

  /* State Init */
  let from = $state(new Date(Date.now() - 5 * 60 * 1000));
  let to = $state(new Date(Date.now()));

</script>

<!-- Loading indicator & Refresh -->

<Row cols={{ xs: 2 }} class="mb-2">
  <Col>
    <h4 class="mb-0">Current utilization of cluster "{cluster}"</h4>
  </Col>
  <Col class="mt-2 mt-md-0">
    <Refresher
      initially={120}
      onRefresh={() => {
        from = new Date(Date.now() - 5 * 60 * 1000);
        to = new Date(Date.now());
      }}
    />
  </Col>
</Row>

<!-- <Row cols={1} class="text-center mt-3">
  <Col>
    {#if $initq.fetching || $mainQuery.fetching}
      <Spinner />
    {:else if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {/if}
  </Col>
</Row>
{#if $mainQuery.error}
  <Row cols={1}>
    <Col>
      <Card body color="danger">{$mainQuery.error.message}</Card>
    </Col>
  </Row>
{/if} -->

<Card class="overflow-auto" style="height: auto;">
  <TabContent>
    <TabPane tabId="status-dash" tab="Status" active>
      <CardBody>
        <StatusDash {cluster} {useCbColors} useAltColors></StatusDash>
      </CardBody>
    </TabPane>

    <TabPane tabId="usage-dash" tab="Usage">
      <CardBody>
        <UsageDash {cluster} {useCbColors}></UsageDash>
      </CardBody>
    </TabPane>
    
    <TabPane tabId="metric-dash" tab="Statistics">
      <CardBody>
        <StatisticsDash {cluster} {useCbColors}></StatisticsDash>
      </CardBody>
    </TabPane>
  </TabContent>
</Card>