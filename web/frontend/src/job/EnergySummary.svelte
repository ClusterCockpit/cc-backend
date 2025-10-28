<!--
  @component Energy Summary component; Displays job energy information.

  Properties:
  - `jobId Number`: The job id
  - `jobEnergy Number?`: The total job energy [Default: null]
  - `jobEnergyFootprint [Object]?`: The partial job energy contributions [Default: null]
-->

<script>
  import { 
    getContext,
    onMount
  } from "svelte";
  import {
    Card,
    CardBody,
    Tooltip,
    Row,
    Col,
    Icon
  } from "@sveltestrap/sveltestrap";
  import { round } from "mathjs";

  /* Svelte 5 Props */
  let {
    jobId,
    jobEnergy = null,
    jobEnergyFootprint = null
  } = $props();

  /* Const Init */
  const carbonPerkWh = getContext("emission");
  
  /* State Init */
  let carbonMass = $state(0);

  /* On Mount */
  onMount(() => {
    if (carbonPerkWh) {
      // ( kWh * g/kWh) / 1000 = kg || Rounded to 2 Digits via [ round(x * 100) / 100 ]
      carbonMass = round( ((jobEnergy ? jobEnergy : 0.0) * carbonPerkWh) / 10 ) / 100;
    }
  });
</script>

<Card>
  <CardBody>
    <Row>
      {#each jobEnergyFootprint as efp}
        <Col class="text-center" id={`energy-footprint-${jobId}-${efp.hardware}`}>
          <div class="cursor-help">
            {#if efp.hardware === 'CPU'}
              <Icon name="cpu-fill" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Accelerator'}
              <Icon name="gpu-card" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Memory'}
              <Icon name="memory" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Core'}
              <Icon name="grid-fill" style="font-size: 1.5rem;"/>
            {:else}
              <Icon name="pci-card" style="font-size: 1.5rem;"/>
            {/if}
            <Icon name="plug-fill" style="font-size: 1.5rem;"/>
          </div>
          <hr class="mt-0 mb-1"/>
          <div class="mr-2"><b>{efp.hardware}:</b> {efp.value} kWh (<i>{efp.metric}</i>)</div>
        </Col>
        <Tooltip
          target={`energy-footprint-${jobId}-${efp.hardware}`}
          placement="top"
          >Estimated energy consumption based on metric {efp.metric} and job runtime.
        </Tooltip>
      {/each}
      <Col class="text-center cursor-help" id={`energy-footprint-${jobId}-total`}>
        <div class="cursor-help">
          <Icon name="lightning-charge-fill" style="font-size: 1.5rem;"/>
        </div>
        <hr class="mt-0 mb-1"/>
        <div><b>Total Energy:</b> {jobEnergy? jobEnergy : 0} kWh</div>
      </Col>
      {#if carbonPerkWh}
        <Col class="text-center cursor-help" id={`energy-footprint-${jobId}-carbon`}>
          <div class="cursor-help">
            <Icon name="cloud-fog2-fill" style="font-size: 1.5rem;"/>
          </div>
          <hr class="mt-0 mb-1"/>
          <div><b>Carbon Emission:</b> {carbonMass} kg</div>
        </Col>
      {/if}
    </Row>
  </CardBody>
</Card>

<Tooltip
  target={`energy-footprint-${jobId}-total`}
  placement="top"
  >Estimated total energy consumption of job.
</Tooltip>

{#if carbonPerkWh}
  <Tooltip
    target={`energy-footprint-${jobId}-carbon`}
    placement="top"
  >Estimated emission based on supplier energy mix ({carbonPerkWh} g/kWh) and total energy consumption.
  </Tooltip>
{/if}

<style>
  .cursor-help {
    cursor: help;
  }
</style>
