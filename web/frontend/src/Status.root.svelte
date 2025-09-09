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
    Row,
    Col,
    Card,
    CardBody,
    TabContent,
    TabPane
  } from "@sveltestrap/sveltestrap";

  import StatusDash from "./status/StatusDash.svelte";
  import UsageDash from "./status/UsageDash.svelte";
  import StatisticsDash from "./status/StatisticsDash.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster
  } = $props();

  /*Const Init */
  const useCbColors = getContext("cc-config")?.plot_general_colorblindMode || false

</script>

<!-- Loading indicator & Refresh -->

<Row cols={1} class="mb-2">
  <Col>
    <h3 class="mb-0">Current Status of Cluster "{presetCluster.charAt(0).toUpperCase() + presetCluster.slice(1)}"</h3>
  </Col>
</Row>

<Card class="overflow-auto" style="height: auto;">
  <TabContent>
    <TabPane tabId="status-dash" tab="Status" active>
      <CardBody>
        <StatusDash {presetCluster} {useCbColors} useAltColors></StatusDash>
      </CardBody>
    </TabPane>

    <TabPane tabId="usage-dash" tab="Usage">
      <CardBody>
        <UsageDash {presetCluster} {useCbColors}></UsageDash>
      </CardBody>
    </TabPane>
    
    <TabPane tabId="metric-dash" tab="Statistics">
      <CardBody>
        <StatisticsDash {presetCluster} {useCbColors}></StatisticsDash>
      </CardBody>
    </TabPane>
  </TabContent>
</Card>