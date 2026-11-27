<!--
  @component Energy Summary component; Displays job energy information.

  Properties:
  - `job Object`: The job; uses `jobId, cluster, subCluster, shared, numNodes, numAcc, numHWThreads, energy, energyFootprint`
-->

<script>
  import {
    getContext,
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
    job
  } = $props();

  /* Const Init */
  const carbonPerkWh = getContext("emission") || 0;
  const clusters = getContext("clusters");

  /* Derived Init */
  const subCluster = $derived(clusters
    ?.find((c) => c.name == job.cluster)
    ?.subClusters?.find((sc) => sc.name == job.subCluster)
  );
  // Stable display order: the backend builds the energy footprint from a map
  const energyFootprint = $derived([...(job?.energyFootprint ?? [])].sort((a, b) =>
    a.hardware.localeCompare(b.hardware) || a.metric.localeCompare(b.metric)
  ));
  // Whole-node energy: an energy metric with native scope "node" in the subcluster metric config.
  // Several (e.g. a node-level fan reading): take the largest. Accept it only if no other energy metric exceeds it,
  // as a whole-node reading cannot be smaller than one of its components; otherwise fall back to the summed energy.
  const nodeEnergy = $derived.by(() => {
    const isNodeScope = (efp) => subCluster?.metricConfig?.find((mc) => mc.name == efp.metric)?.scope === "node";
    const candidates = energyFootprint.filter(isNodeScope);
    if (candidates.length === 0) return null;
    const best = candidates.reduce((a, b) => (b.value > a.value ? b : a));
    const maxComponent = Math.max(0, ...energyFootprint.filter((efp) => !isNodeScope(efp)).map((efp) => efp.value));
    return (best.value > 0 && best.value >= maxComponent) ? best : null;
  });
  // Job share of its nodes: node-scope metrics cover the whole node, all other metrics only the job's own resources.
  const nodeShare = $derived.by(() => {
    if (job.shared === "none") return { fraction: 1 };
    const numNodes = job.numNodes || 1;
    const nodeAccs = subCluster?.topology?.accelerators?.length || 0;
    const nodeHWThreads = subCluster?.topology?.node?.length || 0;
    if (job.numAcc > 0 && nodeAccs > 0) {
      const total = numNodes * nodeAccs;
      return { fraction: Math.min(1, job.numAcc / total), used: job.numAcc, total, unit: "accelerators" };
    }
    if (job.numHWThreads > 0 && nodeHWThreads > 0) {
      const total = numNodes * nodeHWThreads;
      return { fraction: Math.min(1, job.numHWThreads / total), used: job.numHWThreads, total, unit: "hwthreads" };
    }
    return null; // Share unknown: no node-based job energy
  });
  const nodeJobEnergy = $derived((nodeEnergy && nodeShare) ? round(nodeEnergy.value * nodeShare.fraction * 100) / 100 : null);
  // ( kWh * g/kWh) / 1000 = kg || Rounded to 2 Digits via [ round(x * 100) / 100 ]
  const carbonMass = $derived(nodeJobEnergy !== null ? round((nodeJobEnergy * carbonPerkWh) / 10) / 100 : null);
</script>

<Card>
  <CardBody>
    <Row>
      {#each energyFootprint as efp (efp.metric)}
        <Col class="text-center" id={`energy-footprint-${job.jobId}-${efp.metric}`}>
          <div class="cursor-help">
            {#if efp.hardware === 'CPU'}
              <Icon name="cpu-fill" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Accelerator'}
              <Icon name="gpu-card" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Memory'}
              <Icon name="memory" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Core'}
              <Icon name="grid-3x3-gap-fill" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Fan'}
              <Icon name="fan" style="font-size: 1.5rem;"/>
            {:else if efp.hardware === 'Node'}
              <Icon name="pc-horizontal" style="font-size: 1.5rem;"/>
            {:else}
              <Icon name="pci-card" style="font-size: 1.5rem;"/>
            {/if}
            <Icon name="plug-fill" style="font-size: 1.5rem;"/>
          </div>
          <hr class="mt-0 mb-1"/>
          <div class="mr-2"><b>{efp.hardware}:</b> {efp.value} kWh (<i>{efp.metric}</i>)</div>
        </Col>
        <Tooltip
          target={`energy-footprint-${job.jobId}-${efp.metric}`}
          placement="top"
          >Estimated energy consumption based on metric {efp.metric} and job runtime.
        </Tooltip>
      {/each}
      <Col class="text-center cursor-help" id={`energy-footprint-${job.jobId}-total`}>
        <div class="cursor-help">
          <Icon name="lightning-charge-fill" style="font-size: 1.5rem;"/>
        </div>
        <hr class="mt-0 mb-1"/>
        {#if nodeJobEnergy !== null}
          <div><b>Job Energy:</b> {nodeJobEnergy} kWh</div>
        {:else if job.energy}
          <div><b>Summed Energy:</b> {job.energy} kWh</div>
        {:else}
          <div><b>Job Energy:</b> n/a</div>
        {/if}
      </Col>
      <!-- Only Use Carbon Emission with measured Nodepower-->
      {#if carbonPerkWh && carbonMass !== null}
        <Col class="text-center cursor-help" id={`energy-footprint-${job.jobId}-carbon`}>
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
  target={`energy-footprint-${job.jobId}-total`}
  placement="top"
>
  {#if nodeJobEnergy !== null}
    Estimated job energy consumption based on whole-node metric {nodeEnergy.metric}{#if job.numNodes > 1}, summed over all {job.numNodes} nodes,{/if}
      and job runtime{#if nodeShare.fraction < 1}, scaled to the job's share of the node{#if job.numNodes > 1}s{/if}
      ({nodeShare.used} of {nodeShare.total} {nodeShare.unit}, {round(nodeShare.fraction * 100)} %){/if}.
  {:else if job.energy}
    Rough job energy consumption based on simple sum of all energy metrics{#if job.numNodes > 1} over all {job.numNodes} nodes{/if}. Use with caution!
  {:else}
    No job energy consumption available.
  {/if}
</Tooltip>

{#if carbonPerkWh && carbonMass !== null}
  <Tooltip
    target={`energy-footprint-${job.jobId}-carbon`}
    placement="top"
  >Estimated emission based on supplier energy mix ({carbonPerkWh} g/kWh) and job energy consumption.
  </Tooltip>
{/if}

<style>
  .cursor-help {
    cursor: help;
  }
</style>
