<!--
    @component Energy Summary component; Displays job.footprint data as bars in relation to thresholds, as polar plot, and summariziong comment

    Properties:
    - `job Object`: The GQL job object
 -->

<script>
  import { 
    getContext 
  } from "svelte";
  import {
    Card,
    CardBody,
    Tooltip,
    Row,
    Col,
  } from "@sveltestrap/sveltestrap";
  import { round } from "mathjs";

  export let job;
  
  const carbonPerkWh = getContext("emission");
  let carbonMass;

  $: if (carbonPerkWh) {
    // (( Wh / 1000 )* g/kWh) / 1000 = kg || Rounded to 2 Digits via [ round(x * 100) / 100 ]
    carbonMass = round( (((job?.energy ? job.energy : 0.0) / 1000 ) * carbonPerkWh) / 10 ) / 100;
  }
</script>

<Card>
  <CardBody>
    <Row>
      {#each job.energyFootprint as efp}
        <Col class="text-center" id={`energy-footprint-${job.jobId}-${efp.hardware}`}>
          <div class="cursor-help mr-2"><b>{efp.hardware}:</b> {efp.value} Wh (<i>{efp.metric}</i>)</div>
        </Col>
        <Tooltip
          target={`energy-footprint-${job.jobId}-${efp.hardware}`}
          placement="top"
          >Estimated energy consumption based on metric {efp.metric} and job runtime.
        </Tooltip>
      {/each}
      <Col class="text-center" id={`energy-footprint-${job.jobId}-total`}>
        <div class="cursor-help"><b>Total Energy:</b> {job?.energy? job.energy : 0} Wh</div>
      </Col>
      {#if carbonPerkWh}
        <Col class="text-center" id={`energy-footprint-${job.jobId}-carbon`}>
          <div class="cursor-help"><b>Carbon Emission:</b> {carbonMass} kg</div>
        </Col>
      {/if}
    </Row>
  </CardBody>
</Card>

<Tooltip
  target={`energy-footprint-${job.jobId}-total`}
  placement="top"
  >Estimated total energy consumption of job.
</Tooltip>

{#if carbonPerkWh}
  <Tooltip
    target={`energy-footprint-${job.jobId}-carbon`}
    placement="top"
  >Estimated emission based on supplier energy mix and total energy consumption.
  </Tooltip>
{/if}

<style>
  .cursor-help {
    cursor: help;
  }
</style>
