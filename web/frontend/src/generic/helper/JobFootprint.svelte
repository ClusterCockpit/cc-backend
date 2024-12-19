<!--
    @component Footprint component; Displays job.footprint data as bars in relation to thresholds

    Properties:
    - `job Object`: The GQL job object
    - `displayTitle Bool?`: If to display cardHeader with title [Default: true]
    - `width String?`: Width of the card [Default: 'auto']
    - `height String?`: Height of the card [Default: '310px']
 -->

<script context="module">
  function findJobThresholds(job, stat, metricConfig) {
    if (!job || !metricConfig || !stat) {
      console.warn("Argument missing for findJobThresholds!");
      return null;
    }
    // metricConfig is on subCluster-Level
    const defaultThresholds = {
      peak: metricConfig.peak,
      normal: metricConfig.normal,
      caution: metricConfig.caution,
      alert: metricConfig.alert
    };
    /*
      Footprints should be comparable:
      Always use unchanged single node thresholds for exclusive jobs and "avg" Footprints.
      For shared jobs, scale thresholds by the fraction of the job's HWThreads to the node's HWThreads.
      'stat' is one of: avg, min, max
    */
    if (job.exclusive === 1 || stat === "avg") {
      return defaultThresholds
    } else {
      const topol = getContext("getHardwareTopology")(job.cluster, job.subCluster)
      const jobFraction = job.numHWThreads / topol.node.length;
      return {
        peak: round(defaultThresholds.peak * jobFraction, 0),
        normal: round(defaultThresholds.normal * jobFraction, 0),
        caution: round(defaultThresholds.caution * jobFraction, 0),
        alert: round(defaultThresholds.alert * jobFraction, 0),
      };
    }
  }
</script>

<script>
  import { getContext } from "svelte";
  import {
    Card,
    CardHeader,
    CardTitle,
    CardBody,
    Progress,
    Icon,
    Tooltip,
    Row,
    Col
  } from "@sveltestrap/sveltestrap";
  import { round } from "mathjs";

  export let job;
  export let displayTitle = true;
  export let width = "auto";
  export let height = "310px";

  const footprintData = job?.footprint?.map((jf) => {
    const fmc = getContext("getMetricConfig")(job.cluster, job.subCluster, jf.name);
    if (fmc) {
      // Unit
      const unit = (fmc?.unit?.prefix ? fmc.unit.prefix : "") + (fmc?.unit?.base ? fmc.unit.base : "")

      // Threshold / -Differences
      const fmt = findJobThresholds(job, jf.stat, fmc);
      if (jf.name === "flops_any") fmt.peak = round(fmt.peak * 0.85, 0);

      // Define basic data -> Value: Use as Provided
      const fmBase = {
        name: jf.name + ' (' + jf.stat + ')',
        avg: jf.value,
        unit: unit,
        max: fmt.peak,
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
        name: jf.name + ' (' + jf.stat + ')',
        avg: jf.value,
        message:
          `No config for metric ${jf.name} found.`,
        impact: 4,
      };
    }
  }).sort(function (a, b) { // Sort by impact value primarily, within impact sort name alphabetically
    return a.impact - b.impact || ((a.name > b.name) ? 1 : ((b.name > a.name) ? -1 : 0));
  });;

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

<Card class="mt-1 overflow-auto" style="width: {width}; height: {height}">
  {#if displayTitle}
    <CardHeader>
      <CardTitle class="mb-0 d-flex justify-content-center">
        Core Metrics Footprint
      </CardTitle>
    </CardHeader>
  {/if}
  <CardBody>
    {#each footprintData as fpd, index}
      {#if fpd.impact !== 4}
        <div class="mb-1 d-flex justify-content-between">
          <div>&nbsp;<b>{fpd.name}</b></div>
          <!-- For symmetry, see below ...-->
          <div
            class="cursor-help d-inline-flex"
            id={`footprint-${job.jobId}-${index}`}
          >
            <div class="mx-1">
              <!-- Alerts Only -->
              {#if fpd.impact === 3}
              <Icon name="exclamation-triangle-fill" class="text-danger" />
              {:else if fpd.impact === 2}
                <Icon name="exclamation-triangle" class="text-warning" />
              {:else if fpd.impact === 0}
                <Icon name="info-circle" class="text-info" />
              {:else if fpd.impact === -1}
                <Icon name="info-circle-fill" class="text-danger" />
              {/if}
              <!-- Emoji for all states-->
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
              <!-- Print Values -->
              {fpd.avg} / {fpd.max}
              {fpd.unit} &nbsp; <!-- To increase margin to tooltip: No other way manageable ... -->
            </div>
          </div>
          <Tooltip
            target={`footprint-${job.jobId}-${index}`}
            placement="right"
          >{fpd.message}</Tooltip
          >
        </div>
        <Row cols={12} class="{(footprintData.length == (index + 1)) ? 'mb-0' : 'mb-2'}">
          {#if fpd.dir}
            <Col xs="1">
              <Icon name="caret-left-fill" />
            </Col>
          {/if}
          <Col xs="11" class="align-content-center">
            <Progress value={fpd.avg} max={fpd.max} color={fpd.color} />
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
            &nbsp;<b>{fpd.name}</b>
          </div>
          <div
            class="cursor-help d-inline-flex"
            id={`footprint-${job.jobId}-${index}`}
          >
            <div class="mx-1">
              <Icon name="info-circle"/>
            </div>
            <div>
              {fpd.avg}&nbsp;
            </div>
          </div>
        </div>
        <Tooltip
          target={`footprint-${job.jobId}-${index}`}
          placement="right"
        >{fpd.message}</Tooltip
        >
      {/if}
    {/each}
    {#if job?.metaData?.message}
      <hr class="mt-1 mb-2" />
      {@html job.metaData.message}
    {/if}
  </CardBody>
</Card>

<style>
  .cursor-help {
    cursor: help;
  }
</style>
