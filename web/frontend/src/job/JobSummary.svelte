<!--
  @component Job Summary component; Displays aggregated job footprint statistics and performance indicators

  Properties:
  - `job Object`: The GQL job object
  - `width String?`: Width of the card [Default: 'auto']
  - `height String?`: Height of the card [Default: '400px']
-->

<script>
  import { getContext } from "svelte";
  import {
    Card,
    TabContent,
    TabPane
  } from "@sveltestrap/sveltestrap";
  import JobFootprintBars from "./jobsummary/JobFootprintBars.svelte";
  import JobFootprintPolar from "./jobsummary/JobFootprintPolar.svelte";

  /* Svelte 5 Props */
  let {
    job,
    width = "auto",
    height = "auto",
  } = $props();

  /* Const Init */
  const showFootprintTab = !!getContext("cc-config")[`jobView_showFootprint`];
  const showPolarTab = !!getContext("cc-config")[`jobView_showPolarPlot`];
</script>

<Card style="width: {width}; height: {height}">
  {#if showFootprintTab  && !showPolarTab}
    <JobFootprintBars {job} />
  {:else if !showFootprintTab  && showPolarTab}
    <JobFootprintPolar {job}/>
  {:else if showFootprintTab  && showPolarTab}
    <TabContent>
      <TabPane tabId="foot" tab="Footprint" active>
        <!-- Bars CardBody Here-->
        <JobFootprintBars {job} />
      </TabPane>
      <TabPane tabId="polar" tab="Polar">
        <!-- Polar Plot CardBody Here -->
        <JobFootprintPolar {job} showLegend={false}/>
      </TabPane>
    </TabContent>
  {:else}
    <Card color="info" class="m-2">
      <CardHeader class="mb-0">
        <b>Config</b>
      </CardHeader>
      <CardBody>
        <p class="mb-1">Footprint and PolarPlot Display Disabled.</p>
      </CardBody>
    </Card>
  {/if}
</Card>
