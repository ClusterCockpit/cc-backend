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
    height = "400px",
  } = $props();

  /* Const Init */
  const showFootprintTab = !!getContext("cc-config")[`job_view_showFootprint`];
</script>

<Card class="overflow-auto" style="width: {width}; height: {height}">
  <TabContent>
    {#if showFootprintTab}
      <TabPane tabId="foot" tab="Footprint" active>
        <!-- Bars CardBody Here-->
        <JobFootprintBars {job} />
      </TabPane>
    {/if}
    <TabPane tabId="polar" tab="Polar" active={!showFootprintTab}>
      <!-- Polar Plot CardBody Here -->
       <JobFootprintPolar {job} />
    </TabPane>
  </TabContent>
</Card>
