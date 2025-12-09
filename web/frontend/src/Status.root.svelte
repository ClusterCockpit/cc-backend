<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
    Card,
  } from "@sveltestrap/sveltestrap";

  import DashDetails from "./status/DashDetails.svelte";
  import DashInternal from "./status/DashInternal.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
    displayType
  } = $props();

  /*Const Init */
  const displayStatusDetail = (displayType === 'DETAILS');
</script>

<!-- <Row cols={1} class="mb-2">
  <Col>
    <h3 class="mb-0">Current Status of Cluster "{presetCluster.charAt(0).toUpperCase() + presetCluster.slice(1)}"</h3>
  </Col>
</Row> -->

{#if displayType !== "DASHBOARD" && displayType !== "DETAILS"}
  <Row>
    <Col>
      <Card body color="danger">Unknown displayList type! </Card>
    </Col>
  </Row>
{:else}
  {#if displayStatusDetail}
    <!-- ROW2-1: Node Overview (Grid Included)-->
    <DashDetails {presetCluster}/>
  {:else}
    <!-- ROW2-2: Node List (Grid Included)-->
    <DashInternal {presetCluster}/>
  {/if}
{/if}
