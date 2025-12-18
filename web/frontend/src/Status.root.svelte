<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `presetCluster String`: The cluster to show status information for
  - `displayType String`: The type of status component to render
-->

 <script>
  import {
    Row,
    Col,
    Card,
    CardBody
  } from "@sveltestrap/sveltestrap";

  import DashDetails from "./status/DashDetails.svelte";
  import DashInternal from "./status/DashInternal.svelte";

  /* Svelte 5 Props */
  let {
    presetCluster,
    displayType
  } = $props();
</script>

{#if displayType === 'DETAILS'}
  <DashDetails {presetCluster}/>
{:else if displayType === 'DASHBOARD'}
  <DashInternal {presetCluster}/>
{:else}
  <Row>
    <Col>
      <Card color="danger">
        <CardBody>
          Unknown DisplayType for Status View!
        </CardBody>
      </Card>
    </Col>
  </Row>
{/if}
