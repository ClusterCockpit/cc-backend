<!--
    @component Main cluster node status view component; renders overview or list depending on type

    Properties:
    - `displayType String?`: The type of node display ['OVERVIEW' || 'LIST']
    - `cluster String`: The cluster to show status information for
    - `from Date?`: Custom Time Range selection 'from' [Default: null]
    - `to Date?`: Custom Time Range selection 'to' [Default: null]
 -->

<script>
  import {
    Row,
    Col,
    Card,
  } from "@sveltestrap/sveltestrap";

  import NodeOverview from "./systems/NodeOverview.svelte";
  import NodeList from "./systems/NodeList.svelte";

  export let displayType;
  export let cluster;
  export let from = null;
  export let to = null;

  console.assert(
    displayType == "OVERVIEW" || displayType == "LIST",
    "Invalid nodes displayType provided!",
  );
</script>

{#if displayType === 'OVERVIEW'}
  <NodeOverview {cluster} {from} {to}/>
{:else if displayType === 'LIST'}
  <NodeList {cluster} {from} {to}/>
{:else}
<Row>
  <Col>
    <Card color="danger">
      Unknown displayList type!
    </Card>
  </Col>
</Row>
{/if}
