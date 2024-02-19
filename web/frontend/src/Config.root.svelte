<script>
  import { getContext } from "svelte";
  import { init } from "./utils.js";
  import {
    Card,
    CardHeader,
    CardTitle,
    Col,
    Icon,
    TabContent,
    TabPane,
    Row,
  } from "sveltestrap";

  import PlotSettings from "./config/PlotSettings.svelte";
  import AdminSettings from "./config/AdminSettings.svelte";
  //   import InfoxDbConf from "./config/admin/InfluxDbConf.svelte";
  import InfluxModalDefault from "./config/admin/InfluxModalDefault.svelte";
  import FileBrowser from "./config/admin/FileBrowser.svelte";

  const { query: initq } = init();

  const ccconfig = getContext("cc-config");

  export let isAdmin;
</script>

<TabContent>
  <TabPane tabId="admin-options" active class="mt-3">
    <span slot="tab">
      Admin Options
      <!-- <Icon name="gear" /> -->
    </span>
    {#if isAdmin == true}
      <Card style="margin-bottom: 1.5em;">
        <AdminSettings />
      </Card>
    {/if}
  </TabPane>
  <TabPane tabId="plotting-options" class="mt-3">
    <span slot="tab">
      Plotting Options
      <!-- <Icon name="hand-thumbs-up" /> -->
    </span>
    <Card>
      <PlotSettings config={ccconfig} />
    </Card>
  </TabPane>
  <TabPane tabId="influx-conf" class="mt-3">
    <span slot="tab">
      Default Configuration
      <!-- <Icon name="alarm" /> -->
    </span>
    <Row cols={2} class="p-2 g-2">
      <Col class="mb-1">
        <Card class="p-3">
          <CardHeader>Default InfluxDB Configuration</CardHeader>
          <br>
          <InfluxModalDefault />
        </Card>
      </Col>
      <Col class="mb-1">
        <FileBrowser />
      </Col>
    </Row>
  </TabPane>
</TabContent>
