<!--
    @component Main plot component, based on uPlot; metricdata values by time

    Only width/height should change reactively.

    Properties:
    - `metric String`: The metric name
    - `scope String?`: Scope of the displayed data [Default: node]
    - `width Number`: The plot width
    - `height Number`: The plot height
    - `timestep Number`: The timestep used for X-axis rendering
    - `series [GraphQL.Series]`: The metric data object
    - `useStatsSeries Bool?`: If this plot uses the statistics Min/Max/Median representation; automatically set to according bool [Default: null]
    - `statisticsSeries [GraphQL.StatisticsSeries]?`: Min/Max/Median representation of metric data [Default: null]
    - `cluster GraphQL.Cluster`: Cluster Object of the parent job
    - `subCluster String`: Name of the subCluster of the parent job
    - `isShared Bool?`: If this job used shared resources; will adapt threshold indicators accordingly [Default: false]
    - `forNode Bool?`: If this plot is used for node data display; will ren[data, err := metricdata.LoadNodeData(cluster, metrics, nodes, scopes, from, to, ctx)](https://github.com/ClusterCockpit/cc-backend/blob/9fe7cdca9215220a19930779a60c8afc910276a3/internal/graph/schema.resolvers.go#L391-L392)der x-axis as negative time with $now as maximum [Default: false]
    - `numhwthreads Number?`: Number of job HWThreads [Default: 0]
    - `numaccs Number?`: Number of job Accelerators [Default: 0]
 -->

<script context="module">
  function formatTime(t, forNode = false) {
    if (t !== null) {
      if (isNaN(t)) {
        return t;
      } else {
        const tAbs = Math.abs(t);
        const h = Math.floor(tAbs / 3600);
        const m = Math.floor((tAbs % 3600) / 60);
        // Re-Add "negativity" to time ticks only as string, so that if-cases work as intended
        if (h == 0) return `${forNode && m != 0 ? "-" : ""}${m}m`;
        else if (m == 0) return `${forNode ? "-" : ""}${h}h`;
        else return `${forNode ? "-" : ""}${h}:${m}h`;
      }
    }
  }

  function timeIncrs(timestep, maxX, forNode) {
    if (forNode === true) {
      return [60, 300, 900, 1800, 3600, 7200, 14400, 21600]; // forNode fixed increments
    } else {
      let incrs = [];
      for (let t = timestep; t < maxX; t *= 10)
        incrs.push(t, t * 2, t * 3, t * 5);

      return incrs;
    }
  }

  // removed arg "subcluster": input metricconfig and topology now directly derived from subcluster
  function findThresholds(
    subClusterTopology,
    metricConfig,
    scope,
    isShared,
    numhwthreads,
    numaccs
  ) {

    if (!subClusterTopology || !metricConfig || !scope) {
      console.warn("Argument missing for findThresholds!");
      return null;
    }

    if (
      (scope == "node" && isShared == false) ||
      metricConfig?.aggregation == "avg"
    ) {
        return {
          normal: metricConfig.normal,
          caution: metricConfig.caution,
          alert: metricConfig.alert,
          peak: metricConfig.peak,
        };
    }


    if (metricConfig?.aggregation == "sum") {
      let divisor = 1
      if (isShared == true) { // Shared
        if (numaccs > 0) divisor = subClusterTopology.accelerators.length / numaccs;
        else if (numhwthreads > 0) divisor = subClusterTopology.node.length / numhwthreads;
      }
      else if (scope == 'socket') divisor = subClusterTopology.socket.length;
      else if (scope == "core") divisor = subClusterTopology.core.length;
      else if (scope == "accelerator")
        divisor = subClusterTopology.accelerators.length;
      else if (scope == "hwthread") divisor = subClusterTopology.node.length;
      else {
        // console.log('TODO: how to calc thresholds for ', scope)
        return null;
      }

      return {
        peak: metricConfig.peak / divisor,
        normal: metricConfig.normal / divisor,
        caution: metricConfig.caution / divisor,
        alert: metricConfig.alert / divisor,
      };
    }

    console.warn(
      "Missing or unkown aggregation mode (sum/avg) for metric:",
      metricConfig,
    );
    return null;
  }
</script>

<script>
  import uPlot from "uplot";
  import { formatNumber } from "../units.js";
  import { getContext, onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  export let metric;
  export let scope = "node";
  export let width;
  export let height;
  export let timestep;
  export let series;
  export let useStatsSeries = null;
  export let statisticsSeries = null;
  export let cluster;
  export let subCluster;
  export let isShared = false;
  export let forNode = false;
  export let numhwthreads = 0;
  export let numaccs = 0;

  if (useStatsSeries == null) useStatsSeries = statisticsSeries != null;

  if (useStatsSeries == false && series == null) useStatsSeries = true;

  const subClusterTopology = getContext("getHardwareTopology")(cluster, subCluster);
  const metricConfig = getContext("getMetricConfig")(cluster, subCluster, metric);
  const clusterCockpitConfig = getContext("cc-config");
  const resizeSleepTime = 250;
  const normalLineColor = "#000000";
  const lineWidth =
    clusterCockpitConfig.plot_general_lineWidth / window.devicePixelRatio;
  const lineColors = clusterCockpitConfig.plot_general_colorscheme;
  const backgroundColors = {
    normal: "rgba(255, 255, 255, 1.0)",
    caution: "rgba(255, 128, 0, 0.3)",
    alert: "rgba(255, 0, 0, 0.3)",
  };
  const thresholds = findThresholds(
    subClusterTopology,
    metricConfig,
    scope,
    isShared,
    numhwthreads,
    numaccs
  );

  // converts the legend into a simple tooltip
  function legendAsTooltipPlugin({
    className,
    style = { backgroundColor: "rgba(255, 249, 196, 0.92)", color: "black" },
  } = {}) {
    let legendEl;
    const dataSize = series.length;

    function init(u, opts) {
      legendEl = u.root.querySelector(".u-legend");

      legendEl.classList.remove("u-inline");
      className && legendEl.classList.add(className);

      uPlot.assign(legendEl.style, {
        textAlign: "left",
        pointerEvents: "none",
        display: "none",
        position: "absolute",
        left: 0,
        top: 0,
        zIndex: 100,
        boxShadow: "2px 2px 10px rgba(0,0,0,0.5)",
        ...style,
      });

      // conditional hide series color markers:
      if (
        useStatsSeries === true || // Min/Max/Median Self-Explanatory
        dataSize === 1 || // Only one Y-Dataseries
        dataSize > 6
      ) {
        // More than 6 Y-Dataseries
        const idents = legendEl.querySelectorAll(".u-marker");
        for (let i = 0; i < idents.length; i++)
          idents[i].style.display = "none";
      }

      const overEl = u.over;
      overEl.style.overflow = "visible";

      // move legend into plot bounds
      overEl.appendChild(legendEl);

      // show/hide tooltip on enter/exit
      overEl.addEventListener("mouseenter", () => {
        legendEl.style.display = null;
      });
      overEl.addEventListener("mouseleave", () => {
        legendEl.style.display = "none";
      });

      // let tooltip exit plot
      // overEl.style.overflow = "visible";
    }

    function update(u) {
      const { left, top } = u.cursor;
      const width = u.over.querySelector(".u-legend").offsetWidth;
      legendEl.style.transform =
        "translate(" + (left - width - 15) + "px, " + (top + 15) + "px)";
    }

    if (dataSize <= 12 || useStatsSeries === true) {
      return {
        hooks: {
          init: init,
          setCursor: update,
        },
      };
    } else {
      // Setting legend-opts show/live as object with false here will not work ...
      return {};
    }
  }

  function backgroundColor() {
    if (
      clusterCockpitConfig.plot_general_colorBackground == false ||
      !thresholds ||
      !(series && series.every((s) => s.statistics != null))
    )
      return backgroundColors.normal;

    let cond =
      thresholds.alert < thresholds.caution
        ? (a, b) => a <= b
        : (a, b) => a >= b;

    let avg =
      series.reduce((sum, series) => sum + series.statistics.avg, 0) /
      series.length;

    if (Number.isNaN(avg)) return backgroundColors.normal;

    if (cond(avg, thresholds.alert)) return backgroundColors.alert;

    if (cond(avg, thresholds.caution)) return backgroundColors.caution;

    return backgroundColors.normal;
  }

  function lineColor(i, n) {
    if (n >= lineColors.length) return lineColors[i % lineColors.length];
    else return lineColors[Math.floor((i / n) * lineColors.length)];
  }

  const longestSeries = useStatsSeries
    ? statisticsSeries.median.length
    : series.reduce((n, series) => Math.max(n, series.data.length), 0);
  const maxX = longestSeries * timestep;
  let maxY = null;

  if (thresholds !== null) {
    maxY = useStatsSeries
      ? statisticsSeries.max.reduce(
          (max, x) => Math.max(max, x),
          thresholds.normal,
        ) || thresholds.normal
      : series.reduce(
          (max, series) => Math.max(max, series.statistics?.max),
          thresholds.normal,
        ) || thresholds.normal;

    if (maxY >= 10 * thresholds.peak) {
      // Hard y-range render limit if outliers in series data
      maxY = 10 * thresholds.peak;
    }
  }

  const plotSeries = [
    {
      label: "Runtime",
      value: (u, ts, sidx, didx) =>
        didx == null ? null : formatTime(ts, forNode),
    },
  ];
  const plotData = [new Array(longestSeries)];

  if (forNode === true) {
    // Negative Timestamp Buildup
    for (let i = 0; i <= longestSeries; i++) {
      plotData[0][i] = (longestSeries - i) * timestep * -1;
    }
  } else {
    // Positive Timestamp Buildup
    for (
      let j = 0;
      j < longestSeries;
      j++ // TODO: Cache/Reuse this array?
    )
      plotData[0][j] = j * timestep;
  }

  let plotBands = undefined;
  if (useStatsSeries) {
    plotData.push(statisticsSeries.min);
    plotData.push(statisticsSeries.max);
    plotData.push(statisticsSeries.median);
    // plotData.push(statisticsSeries.mean);

    if (forNode === true) {
      // timestamp 0 with null value for reversed time axis
      if (plotData[1].length != 0) plotData[1].push(null);
      if (plotData[2].length != 0) plotData[2].push(null);
      if (plotData[3].length != 0) plotData[3].push(null);
      // if (plotData[4].length != 0) plotData[4].push(null);
    }

    plotSeries.push({
      label: "min",
      scale: "y",
      width: lineWidth,
      stroke: "red",
    });
    plotSeries.push({
      label: "max",
      scale: "y",
      width: lineWidth,
      stroke: "green",
    });
    plotSeries.push({
      label: "median",
      scale: "y",
      width: lineWidth,
      stroke: "black",
    });
    // plotSeries.push({
    //   label: "mean",
    //   scale: "y",
    //   width: lineWidth,
    //   stroke: "blue",
    // });

    plotBands = [
      { series: [2, 3], fill: "rgba(0,255,0,0.1)" },
      { series: [3, 1], fill: "rgba(255,0,0,0.1)" },
    ];
  } else {
    for (let i = 0; i < series.length; i++) {
      plotData.push(series[i].data);
      if (forNode === true && plotData[1].length != 0) plotData[1].push(null); // timestamp 0 with null value for reversed time axis
      plotSeries.push({
        label:
          scope === "node"
            ? series[i].hostname
            : scope + " #" + (i + 1),
        scale: "y",
        width: lineWidth,
        stroke: lineColor(i, series.length),
      });
    }
  }

  const opts = {
    width,
    height,
    plugins: [legendAsTooltipPlugin()],
    series: plotSeries,
    axes: [
      {
        scale: "x",
        space: 35,
        incrs: timeIncrs(timestep, maxX, forNode),
        values: (_, vals) => vals.map((v) => formatTime(v, forNode)),
      },
      {
        scale: "y",
        grid: { show: true },
        labelFont: "sans-serif",
        values: (u, vals) => vals.map((v) => formatNumber(v)),
      },
    ],
    bands: plotBands,
    padding: [5, 10, -20, 0],
    hooks: {
      draw: [
        (u) => {
          // Draw plot type label:
          let textl = `${scope}${plotSeries.length > 2 ? "s" : ""}${
            useStatsSeries
              ? ": min/median/max"
              : metricConfig != null && scope != metricConfig.scope
                ? ` (${metricConfig.aggregation})`
                : ""
          }`;
          let textr = `${isShared && scope != "core" && scope != "accelerator" ? "[Shared]" : ""}`;
          u.ctx.save();
          u.ctx.textAlign = "start"; // 'end'
          u.ctx.fillStyle = "black";
          u.ctx.fillText(textl, u.bbox.left + 10, u.bbox.top + 10);
          u.ctx.textAlign = "end";
          u.ctx.fillStyle = "black";
          u.ctx.fillText(
            textr,
            u.bbox.left + u.bbox.width - 10,
            u.bbox.top + 10,
          );
          // u.ctx.fillText(text, u.bbox.left + u.bbox.width - 10, u.bbox.top + u.bbox.height - 10) // Recipe for bottom right

          if (!thresholds) {
            u.ctx.restore();
            return;
          }

          let y = u.valToPos(thresholds.normal, "y", true);
          u.ctx.save();
          u.ctx.lineWidth = lineWidth;
          u.ctx.strokeStyle = normalLineColor;
          u.ctx.setLineDash([5, 5]);
          u.ctx.beginPath();
          u.ctx.moveTo(u.bbox.left, y);
          u.ctx.lineTo(u.bbox.left + u.bbox.width, y);
          u.ctx.stroke();
          u.ctx.restore();
        },
      ],
    },
    scales: {
      x: { time: false },
      y: maxY ? { min: 0, max: (maxY * 1.1) } : {auto: true}, // Add some space to upper render limit
    },
    legend: {
      // Display legend until max 12 Y-dataseries
      show: series.length <= 12 || useStatsSeries === true ? true : false,
      live: series.length <= 12 || useStatsSeries === true ? true : false,
    },
    cursor: { drag: { x: true, y: true } },
  };

  let plotWrapper = null;
  let uplot = null;
  let timeoutId = null;
  let prevWidth = null,
    prevHeight = null;

  function render() {
    if (!width || Number.isNaN(width) || width < 0) return;

    if (prevWidth != null && Math.abs(prevWidth - width) < 10) return;

    prevWidth = width;
    prevHeight = height;

    if (!uplot) {
      opts.width = width;
      opts.height = height;
      uplot = new uPlot(opts, plotData, plotWrapper);
    } else {
      uplot.setSize({ width, height });
    }
  }

  function onSizeChange() {
    if (!uplot) return;

    if (timeoutId != null) clearTimeout(timeoutId);

    timeoutId = setTimeout(() => {
      timeoutId = null;
      render();
    }, resizeSleepTime);
  }

  $: if (series[0].data.length > 0) {
    onSizeChange(width, height);
  }

  onMount(() => {
    if (series[0].data.length > 0) {
      plotWrapper.style.backgroundColor = backgroundColor();
      render();
    }
  });

  onDestroy(() => {
    if (uplot) uplot.destroy();

    if (timeoutId != null) clearTimeout(timeoutId);
  });
</script>

{#if series[0].data.length > 0}
  <div bind:this={plotWrapper} class="cc-plot"></div>
{:else}
  <Card class="mx-4" body color="warning"
    >Cannot render plot: No series data returned for <code>{metric}</code></Card
  >
{/if}

<style>
  .cc-plot {
    border-radius: 5px;
  }
</style>
