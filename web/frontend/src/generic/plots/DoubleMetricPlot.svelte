<!--
  @component Main plot component, based on uPlot; metricdata values by time

  Only width/height should change reactively.

  Properties:
  - `metricData [Data]`: Two series of metric data including unit info
  - `timestep Number`: Data timestep
  - `numNodes Number`: Number of nodes from which metric data is aggregated
  - `cluster String`: Cluster name of the parent job / data [Default: ""]
  - `forNode Bool?`: If this plot is used for node data display; will render x-axis as negative time with $now as maximum [Default: true]
  - `enableFlip Bool?`: Whether to use legend tooltip flipping based on canvas size [Default: false]
  - `publicMode Bool?`: Disables tooltip legend and enables larger colored axis labels [Default: false]
  - `height Number?`: The plot height [Default: 300]
-->

<script>
  import uPlot from "uplot";
  import { formatNumber, formatDurationTime } from "../units.js";
  import { getContext, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    metricData,
    timestep,
    numNodes,
    cluster,
    forNode = true,
    enableFlip = false,
    publicMode = false,
    height = 300,
  } = $props();

  /* Const Init */
  const clusterCockpitConfig = getContext("cc-config");
  const fixedLineColors = ["#0000ff", "#ff0000"]; // Plot only uses 2 Datasets: High Contrast
  const renderSleepTime = 200;

  /* Var Init */
  let timeoutId = null;

  /* State Init */
  let plotWrapper = $state(null);
  let width = $state(0); // Wrapper Width
  let uplot = $state(null);

  /* Derived */
  const maxX = $derived(longestSeries * timestep);
  const lineWidth = $derived(publicMode ? 2 : clusterCockpitConfig.plotConfiguration_lineWidth / window.devicePixelRatio);
  const longestSeries = $derived.by(() => {
    return metricData.reduce((n, m) => Math.max(n, m.data.length), 0);
  });

  // Derive Plot Params
  let plotData = $derived.by(() => {
    let pendingData = [new Array(longestSeries)];
    // X
    if (forNode === true) {
      // Negative Timestamp Buildup
      for (let i = 0; i <= longestSeries; i++) {
        pendingData[0][i] = (longestSeries - i) * timestep * -1;
      }
    } else {
      // Positive Timestamp Buildup
      for (let j = 0; j < longestSeries; j++) {
        pendingData[0][j] = j * timestep;
      };
    };
    // Y
    for (let i = 0; i < metricData.length; i++) {
      pendingData.push(metricData[i]?.data);
    };
    return pendingData;
  })

  let plotSeries = $derived.by(() => {
    // X
    let pendingSeries = [
      {
        label: "Runtime",
        value: (u, ts, sidx, didx) =>
        (didx == null) ? null : formatDurationTime(ts, forNode),
      }
    ];
    // Y
    for (let i = 0; i < metricData.length; i++) {
      pendingSeries.push({
        label: publicMode ? null : `${metricData[i]?.name} (${metricData[i]?.unit?.prefix}${metricData[i]?.unit?.base})`,
        scale: `y${i+1}`,
        width: lineWidth,
        stroke: fixedLineColors[i],
      });
    };
    return pendingSeries;
  })

  // Set Options
  function getOpts(optWidth, optHeight) {
    let baseOpts = {
      width: optWidth,
      height: optHeight,
      series: plotSeries,
      axes: [
        {
          scale: "x",
          incrs: timeIncrs(timestep, maxX, forNode),
          values: (_, vals) => vals.map((v) => formatDurationTime(v, forNode)),
        },
        {
          scale: "y1",
          grid: { show: true },
          values: (u, vals) => vals.map((v) => formatNumber(v)),
        },
        {
          side: 1,
          scale: "y2",
          grid: { show: false },
          values: (u, vals) => vals.map((v) => formatNumber(v)),
        },
      ],
      // bands: plotBands,
      padding: [5, 10, -20, 0],
      hooks: {},
      scales: {
        x: { time: false },
        y1: { auto: true },
        y2: { auto: true },
      },
      legend: {
        show: !publicMode,
        live: !publicMode
      },
      cursor: { 
        drag: { x: true, y: true },
      }
    }

    if (publicMode) {
      // X
      baseOpts.axes[0].space = 60;
      baseOpts.axes[0].font = '16px Arial';
      // Y1
      baseOpts.axes[1].space = 50;
      baseOpts.axes[1].size = 60;
      baseOpts.axes[1].font = '16px Arial';
      baseOpts.axes[1].stroke = fixedLineColors[0];
      // Y2
      baseOpts.axes[2].space = 40;
      baseOpts.axes[2].size = 60;
      baseOpts.axes[2].font = '16px Arial';
      baseOpts.axes[2].stroke = fixedLineColors[1];
    } else {
      baseOpts.title = 'Cluster Utilization';
      baseOpts.plugins = [legendAsTooltipPlugin()];
      // X
      baseOpts.axes[0].label = 'Time';
      // Y1
      baseOpts.axes[1].label = `${metricData[0]?.name} (${metricData[0]?.unit?.prefix}${metricData[0]?.unit?.base})`;
      // Y2
      baseOpts.axes[2].label = `${metricData[1]?.name} (${metricData[1]?.unit?.prefix}${metricData[1]?.unit?.base})`;
      baseOpts.hooks.draw = [
        (u) => {
          // Draw plot type label:
          let textl = `Cluster ${cluster}`
          let textr = `Sums of ${numNodes} nodes`
          u.ctx.save();
          u.ctx.textAlign = "start"; // 'end'
          u.ctx.fillStyle = "black";
          u.ctx.fillText(textl, u.bbox.left + 10, u.bbox.top + (forNode ? 0 : 10));
          u.ctx.textAlign = "end";
          u.ctx.fillStyle = "black";
          u.ctx.fillText(
            textr,
            u.bbox.left + u.bbox.width - 10,
            u.bbox.top + (forNode ? 0 : 10),
          );

          u.ctx.restore();
          return;
        },
      ]
    }

    return baseOpts;
  };

  /* Effects */
  // This updates plot on all size changes if wrapper (== data) exists
  $effect(() => {
    if (plotWrapper) {
      onSizeChange(width, height);
    }
  });

  /* Functions */
  function timeIncrs(timestep, maxX, forNode) {
    if (forNode === true) {
      return [60, 120, 240, 300, 360, 480, 600, 900, 1800, 3600, 7200, 14400, 21600]; // forNode fixed increments
    } else {
      let incrs = [];
      for (let t = timestep; t < maxX; t *= 10)
        incrs.push(t, t * 2, t * 3, t * 5);

      return incrs;
    }
  }

  // UPLOT PLUGIN // converts the legend into a simple tooltip
  function legendAsTooltipPlugin({
    className,
    style = { backgroundColor: "rgba(255, 249, 196, 0.92)", color: "black" },
  } = {}) {
    let legendEl;
    const dataSize = metricData.length;

    function init(u, opts) {
      legendEl = u.root.querySelector(".u-legend");

      legendEl.classList.remove("u-inline");
      className && legendEl.classList.add(className);

      uPlot.assign(legendEl.style, {
        minWidth: "100px",
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
        // useStatsSeries || // Min/Max/Median Self-Explanatory
        dataSize === 1 || // Only one Y-Dataseries
        dataSize > 8 // More than 8 Y-Dataseries
      ) {
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
      const internalWidth = u?.over?.querySelector(".u-legend")?.offsetWidth ? u.over.querySelector(".u-legend").offsetWidth : 0;
      if (enableFlip && (left < (width/2))) {
        legendEl.style.transform = "translate(" + (left + 15) + "px, " + (top + 15) + "px)";
      } else {
        legendEl.style.transform = "translate(" + (left - internalWidth - 15) + "px, " + (top + 15) + "px)";
      }
    }

    if (dataSize <= 12 ) { // || useStatsSeries) {
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

  function onSizeChange(chgWidth, chgHeight) {
    if (timeoutId != null) clearTimeout(timeoutId);
    timeoutId = setTimeout(() => {
      timeoutId = null;
      render(chgWidth, chgHeight);
    }, renderSleepTime);
  }

  function render(renWidth, renHeight) {
    if (!uplot) {
      let opts = getOpts(renWidth, renHeight);
      uplot = new uPlot(opts, plotData, plotWrapper);
    } else {
      uplot.setSize({ width: renWidth, height: renHeight });
    }
  }

  /* On Destroy */
  onDestroy(() => {
    if (timeoutId != null) clearTimeout(timeoutId);
    if (uplot) uplot.destroy();
  });

</script>

<!-- Define $width Wrapper and NoData Card -->
{#if metricData[0]?.data && metricData[0]?.data?.length > 0}
  <div bind:this={plotWrapper} bind:clientWidth={width}
        class={forNode ? 'py-2 rounded' : 'rounded'}
  ></div>
{:else if cluster}
  <Card body color="warning" class="mx-4"
    >Cannot render plot: No series data returned for <code>{cluster}</code>.</Card
  >
{:else}
  <Card body color="warning" class="mx-4"
    >Cannot render plot: No series data returned.</Card
  >
{/if}
