<!--
    @component Job Summary component; Displays job.footprint data as bars in relation to thresholds, as polar plot, and summariziong comment

    Properties:
    - `job Object`: The GQL job object
    - `displayTitle Bool?`: If to display cardHeader with title [Default: true]
    - `width String?`: Width of the card [Default: 'auto']
    - `height String?`: Height of the card [Default: '310px']
 -->

<script context="module">
  function findJobThresholds(job, metricConfig) {
    if (!job || !metricConfig) {
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

    // Job_Exclusivity does not matter, only aggregation
    if (metricConfig.aggregation === "avg") {
      return defaultThresholds;
    } else if (metricConfig.aggregation === "sum") {
      const topol = getContext("getHardwareTopology")(job.cluster, job.subCluster)
      const jobFraction = job.numHWThreads / topol.node.length;

      return {
        peak: round(defaultThresholds.peak * jobFraction, 0),
        normal: round(defaultThresholds.normal * jobFraction, 0),
        caution: round(defaultThresholds.caution * jobFraction, 0),
        alert: round(defaultThresholds.alert * jobFraction, 0),
      };
    } else {
      console.warn(
        "Missing or unkown aggregation mode (sum/avg) for metric:",
        metricConfig,
      );
      return defaultThresholds;
    }
  }
</script>

<script>
  import { getContext } from "svelte";
  import {
    Card,
    CardBody,
    Progress,
    Icon,
    Tooltip,
    Row,
    Col,
    TabContent,
    TabPane
  } from "@sveltestrap/sveltestrap";
  import Polar from "../generic/plots/Polar.svelte";
  import { round } from "mathjs";

  export let job;
  export let jobMetrics;
  export let width = "auto";
  export let height = "400px";

  const ccconfig = getContext("cc-config")

  const footprintData = job?.footprint?.map((jf) => {
    const fmc = getContext("getMetricConfig")(job.cluster, job.subCluster, jf.name);
    if (fmc) {
      // Unit
      const unit = (fmc?.unit?.prefix ? fmc.unit.prefix : "") + (fmc?.unit?.base ? fmc.unit.base : "")

      // Threshold / -Differences
      const fmt = findJobThresholds(job, fmc);
      if (jf.name === "flops_any") fmt.peak = round(fmt.peak * 0.85, 0);

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
          message: `Metric average way ${fmc.lowerIsBetter ? "above" : "below"} expected normal thresholds.`,
          impact: 3
        };
      } else if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "caution")) {
        return {
          ...fmBase,
          color: "warning",
          message: `Metric average ${fmc.lowerIsBetter ? "above" : "below"} expected normal thresholds.`,
          impact: 2,
        };
      } else if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "normal")) {
        return {
          ...fmBase,
          color: "success",
          message: "Metric average within expected thresholds.",
          impact: 1,
        };
      } else if (evalFootprint(jf.value, fmt, fmc.lowerIsBetter, "peak")) {
        return {
          ...fmBase,
          color: "info",
          message:
            "Metric average above expected normal thresholds: Check for artifacts recommended.",
          impact: 0,
        };
      } else {
        return {
          ...fmBase,
          color: "secondary",
          message:
            "Metric average above expected peak threshold: Check for artifacts!",
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

  function evalFootprint(mean, thresholds, lowerIsBetter, level) {
    // Handle Metrics in which less value is better
    switch (level) {
      case "peak":
        if (lowerIsBetter)
          return false; // metric over peak -> return false to trigger impact -1
        else return mean <= thresholds.peak && mean > thresholds.normal;
      case "alert":
        if (lowerIsBetter)
          return mean <= thresholds.peak && mean >= thresholds.alert;
        else return mean <= thresholds.alert && mean >= 0;
      case "caution":
        if (lowerIsBetter)
          return mean < thresholds.alert && mean >= thresholds.caution;
        else return mean <= thresholds.caution && mean > thresholds.alert;
      case "normal":
        if (lowerIsBetter)
          return mean < thresholds.caution && mean >= 0;
        else return mean <= thresholds.normal && mean > thresholds.caution;
      default:
        return false;
    }
  }

  function writeSummary(fpd) {
    // Hardcoded! Needs to be retrieved from globalMetrics
    const performanceMetrics = ['flops_any', 'mem_bw'];
    const utilizationMetrics = ['cpu_load', 'acc_utilization'];
    const energyMetrics = ['cpu_power'];

    let performanceScore = 0;
    let utilizationScore = 0;
    let energyScore = 0;

    let performanceMetricsCounted = 0;
    let utilizationMetricsCounted = 0;
    let energyMetricsCounted = 0;

    fpd.forEach(metric => {
      console.log('Metric, Impact', metric.name, metric.impact)
      if (performanceMetrics.includes(metric.name)) {
        performanceScore += metric.impact
        performanceMetricsCounted += 1
      } else if (utilizationMetrics.includes(metric.name)) {
        utilizationScore += metric.impact
        utilizationMetricsCounted += 1
      } else if (energyMetrics.includes(metric.name)) {
        energyScore += metric.impact
        energyMetricsCounted += 1 
      }
    });

    performanceScore = (performanceMetricsCounted == 0) ? performanceScore : (performanceScore / performanceMetricsCounted);
    utilizationScore = (utilizationMetricsCounted == 0) ? utilizationScore : (utilizationScore / utilizationMetricsCounted);
    energyScore = (energyMetricsCounted == 0) ? energyScore : (energyScore / energyMetricsCounted);

    let res = [];

    console.log('Perf', performanceScore, performanceMetricsCounted)
    console.log('Util', utilizationScore, utilizationMetricsCounted)
    console.log('Energy', energyScore, energyMetricsCounted)

    if (performanceScore == 1) {
      res.push('<b>Performance:</b> Your job performs well.')
    } else if (performanceScore != 0) {
      res.push('<b>Performance:</b> Your job performs suboptimal.')
    }

    if (utilizationScore == 1) {
      res.push('<b>Utilization:</b> Your job utilizes resources well.')
    } else if (utilizationScore != 0) {
      res.push('<b>Utilization:</b> Your job utilizes resources suboptimal.')
    }

    if (energyScore == 1) {
      res.push('<b>Energy:</b> Your job has good energy values.')
    } else if (energyScore != 0) {
      res.push('<b>Energy:</b> Your job consumes more energy than necessary.')
    }

    return res;
  };

  $: summaryMessages = writeSummary(footprintData)
</script>

<Card class="overflow-auto" style="width: {width}; height: {height}">
  <TabContent> <!-- on:tab={(e) => (status = e.detail)} -->
    <TabPane tabId="foot" tab="Footprint" active>
      <CardBody>
        {#each footprintData as fpd, index}
          {#if fpd.impact !== 4}
            <div class="mb-1 d-flex justify-content-between">
              <div>&nbsp;<b>{fpd.name} ({fpd.stat})</b></div>
              <div
                class="cursor-help d-inline-flex"
                id={`footprint-${job.jobId}-${index}`}
              >
                <div class="mx-1">
                  {#if fpd.impact === 3 || fpd.impact === -1}
                    <Icon name="exclamation-triangle-fill" class="text-danger" />
                  {:else if fpd.impact === 2}
                    <Icon name="exclamation-triangle" class="text-warning" />
                  {/if}
                  {#if fpd.impact === 3}
                    <Icon name="emoji-frown" class="text-danger" />
                  {:else if fpd.impact === 2}
                    <Icon name="emoji-neutral" class="text-warning" />
                  {:else if fpd.impact === 1}
                    <Icon name="emoji-smile" class="text-success" />
                  {:else if fpd.impact === 0}
                    <Icon name="emoji-laughing" class="text-info" />
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
            >{fpd.message}</Tooltip
            >
          {/if}
        {/each}
      </CardBody>
    </TabPane>
    <TabPane tabId="polar" tab="Polar">
      <CardBody>
        <Polar
          {footprintData}
          {jobMetrics}
        />
      </CardBody>
    </TabPane>
    <TabPane tabId="summary" tab="Summary">
      <CardBody>
        <p>Based on footprint data, this job performs as follows:</p>
        <hr/>
        <ul>
        {#each summaryMessages as sm}
          <li>
            {@html sm}
          </li>
        {/each}
        </ul>
      </CardBody>
    </TabPane>
  </TabContent>
</Card>

<style>
  .cursor-help {
    cursor: help;
  }
</style>
