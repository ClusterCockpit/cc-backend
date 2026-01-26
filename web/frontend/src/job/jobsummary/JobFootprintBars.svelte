<!--
  @component Job Footprint Bar component; Displays job footprint db data as bars relative to thresholds. Displays quality indicators and tooltips.

  Properties:
  - `job Object`: The GQL job object
-->

<script>
  import { getContext } from "svelte";
  import {
    CardBody,
    Progress,
    Icon,
    Tooltip,
    Row,
    Col
  } from "@sveltestrap/sveltestrap";
  import { findJobFootprintThresholds } from "../../generic/utils.js";

  /* Svelte 5 Props */
  let {
    job
  } = $props();

  /* Derived */
  // Prepare Job Footprint Data Based On Values Saved In Database
  const jobFootprintData = $derived(buildFootprint(job?.footprint));

  /* Functions */
  function buildFootprint(input) {
    let result = input?.map((jf) => {
      const fmc = getContext("getMetricConfig")(job.cluster, job.subCluster, jf.name);
      if (fmc) {
        // Unit
        const unit = (fmc?.unit?.prefix ? fmc.unit.prefix : "") + (fmc?.unit?.base ? fmc.unit.base : "")

        // Threshold / -Differences
        const fmt = findJobFootprintThresholds(job, jf.stat, fmc);

        // Define basic data -> Value: Use as Provided
        const fmBase = {
          name: jf.name,
          stat: jf.stat,
          value: jf.value,
          unit: unit,
          peak: fmt.peak,
          dir: fmc.lowerIsBetter
        };

        if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "alert")) {
          return {
            ...fmBase,
            color: "danger",
            message: `Footprint value way ${fmc.lowerIsBetter ? "above" : "below"} expected normal threshold.`,
            impact: 3
          };
        } else if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "caution")) {
          return {
            ...fmBase,
            color: "warning",
            message: `Footprint value ${fmc.lowerIsBetter ? "above" : "below"} expected normal threshold.`,
            impact: 2,
          };
        } else if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "normal")) {
          return {
            ...fmBase,
            color: "success",
            message: "Footprint value within expected thresholds.",
            impact: 1,
          };
        } else if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "peak")) {
          return {
            ...fmBase,
            color: "info",
            message:
              "Footprint value above expected normal threshold: Check for artifacts recommended.",
            impact: 0,
          };
        } else {
          return {
            ...fmBase,
            color: "secondary",
            message:
              "Footprint value above expected peak threshold: Check for artifacts!",
            impact: -1,
          };
        }
      } else { // No matching metric config: display as single value
        return {
          name: jf.name,
          stat: jf.stat,
          value: jf.value,
          message:
            `No config for metric ${jf.name} found.`,
          impact: 4,
        };
      }
    }).sort(function (a, b) { // Sort by impact value primarily, within impact sort name alphabetically
      return a.impact - b.impact || ((a.name > b.name) ? 1 : ((b.name > a.name) ? -1 : 0));
    });;

    return result
  };

  function evalFootprint(value, thresholds, lowerIsBetter, level) {
    // Handle Metrics in which less value is better
    switch (level) {
      case "peak":
        if (lowerIsBetter)
          return false; // metric over peak -> return false to trigger impact -1
        else return value <= thresholds.peak && value > thresholds.normal;
      case "alert":
        if (lowerIsBetter)
          return value <= thresholds.peak && value >= thresholds.alert;
        else return value <= thresholds.alert && value >= 0;
      case "caution":
        if (lowerIsBetter)
          return value < thresholds.alert && value >= thresholds.caution;
        else return value <= thresholds.caution && value > thresholds.alert;
      case "normal":
        if (lowerIsBetter)
          return value < thresholds.caution && value >= 0;
        else return value <= thresholds.normal && value > thresholds.caution;
      default:
        return false;
    }
  }
</script>

<CardBody class="overflow-auto" style="height:380px;">
  {#if jobFootprintData.length === 0}
    <div class="text-center">No footprint data for job available.</div>
  {:else}
    {#each jobFootprintData as fpd, index}
      {#if fpd.impact !== 4}
        <div class="mb-1 d-flex justify-content-between">
          <div>&nbsp;<b>{fpd.name} ({fpd.stat})</b></div>
          <div
            class="cursor-help d-inline-flex"
            id={`footprint-${job.jobId}-${index}`}
          >
            <div class="mx-1">
              {#if fpd.impact === 3}
                <Icon name="exclamation-triangle-fill" class="text-danger" />
              {:else if fpd.impact === 2}
                <Icon name="exclamation-triangle" class="text-warning" />
              {:else if fpd.impact === 0}
                <Icon name="info-circle" class="text-info" />
              {:else if fpd.impact === -1}
                <Icon name="info-circle-fill" class="text-danger" />
              {/if}
              {#if fpd.impact === 3}
                <Icon name="emoji-frown" class="text-danger" />
              {:else if fpd.impact === 2}
                <Icon name="emoji-neutral" class="text-warning" />
              {:else if fpd.impact === 1}
                <Icon name="emoji-smile" class="text-success" />
              {:else if fpd.impact === 0}
                <Icon name="emoji-smile" class="text-info" />
              {:else if fpd.impact === -1}
                <Icon name="emoji-dizzy" class="text-danger" />
              {/if}
            </div>
            <div>
              {fpd.value} / {fpd.peak}
              {fpd.unit} &nbsp;
            </div>
          </div>
          <Tooltip
            target={`footprint-${job.jobId}-${index}`}
            placement="right"
          >{fpd.message}</Tooltip>
        </div>
        <Row cols={12} class={(jobFootprintData.length == (index + 1)) ? 'mb-0' : 'mb-2'}>
          {#if fpd.dir}
            <Col xs="1">
              <Icon name="caret-left-fill" />
            </Col>
          {/if}
          <Col xs="11" class="align-content-center">
            <Progress value={fpd.value} max={fpd.peak} color={fpd.color} />
          </Col>
          {#if !fpd.dir}
            <Col xs="1">
              <Icon name="caret-right-fill" />
            </Col>
          {/if}
        </Row>
      {:else}
        <div class="mb-1 d-flex justify-content-between">
          <div>
            &nbsp;<b>{fpd.name} ({fpd.stat})</b>
          </div>
          <div
            class="cursor-help d-inline-flex"
            id={`footprint-${job.jobId}-${index}`}
          >
            <div class="mx-1">
              <Icon name="info-circle"/>
            </div>
            <div>
              {fpd.value}&nbsp;
            </div>
          </div>
        </div>
        <Tooltip
          target={`footprint-${job.jobId}-${index}`}
          placement="right"
        >{fpd.message}</Tooltip>
      {/if}
    {/each}
  {/if}
</CardBody>

<style>
  .cursor-help {
    cursor: help;
  }
</style>