<!--
    @component Main plot component, based on uPlot; metricdata values by time

    Only width/height should change reactively.

    Properties:
    - `metric String`: The metric name
    - `width Number?`: The plot width [Default: 0]
    - `height Number?`: The plot height [Default: 300]
    - `data [Array]`: The metric data object
    - `cluster String`: Cluster name of the parent job / data
    - `subCluster String`: Name of the subCluster of the parent job
 -->

<script>
  import uPlot from "uplot";
  import { formatNumber } from "../units.js";
  import { getContext, onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  export let metric;
  export let width = 0;
  export let height = 300;
  export let data;
  export let xlabel;
  export let xticks;
  export let ylabel;
  export let yunit;
  export let title;
  // export let cluster = "";
  // export let subCluster = "";

  $: console.log('LABEL:', metric, yunit)
  $: console.log('DATA:', data)
  $: console.log('XTICKS:', xticks)

  const metricConfig = null // DEBUG FILLER
  // const metricConfig = getContext("getMetricConfig")(cluster, subCluster, metric); // Args woher
  const clusterCockpitConfig = getContext("cc-config");
  const lineWidth = clusterCockpitConfig.plot_general_lineWidth / window.devicePixelRatio;
  const cbmode = clusterCockpitConfig?.plot_general_colorblindMode || false;

  // Format Seconds to hh:mm
  function formatTime(t) {
    if (t !== null) {
      if (isNaN(t)) {
        return t;
      } else {
        const tAbs = Math.abs(t);
        const h = Math.floor(tAbs / 3600);
        const m = Math.floor((tAbs % 3600) / 60);
        if (h == 0) return `${m}m`;
        else if (m == 0) return `${h}h`;
        else return `${h}:${m}h`;
      }
    }
  }

  // UPLOT PLUGIN // converts the legend into a simple tooltip
  function legendAsTooltipPlugin({
    className,
    style = { backgroundColor: "rgba(255, 249, 196, 0.92)", color: "black" },
  } = {}) {
    let legendEl;

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

      //  hide series color markers:
      const idents = legendEl.querySelectorAll(".u-marker");
      for (let i = 0; i < idents.length; i++)
        idents[i].style.display = "none";

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
      const width = u?.over?.querySelector(".u-legend")?.offsetWidth ? u.over.querySelector(".u-legend").offsetWidth : 0;
      legendEl.style.transform =
        "translate(" + (left - width - 15) + "px, " + (top + 15) + "px)";
    }

    return {
      hooks: {
        init: init,
        setCursor: update,
      },
    };
  }

  let maxY = null;
  // TODO: Hilfreich!
  // if (metricConfig !== null) {
  //   maxY = data[3].reduce( // Data[3] is JobMaxs
  //         (max, x) => Math.max(max, x),
  //         metricConfig.normal,
  //       ) || metricConfig.normal
  //   if (maxY >= 10 * metricConfig.peak) {
  //     // Hard y-range render limit if outliers in series data
  //     maxY = 10 * metricConfig.peak;
  //   }
  // }

  const plotSeries = [
    {
      label: "JobID",
      scale: "x",
      value: (u, ts, sidx, didx) => {
        return xticks[didx];
      },
    },
    {
      label: "Starttime",
      scale: "xst",
      value: (u, ts, sidx, didx) => {
        return new Date(ts * 1000).toLocaleString();
      },
    },
    {
      label: "Duration",
      scale: "xrt",
      value: (u, ts, sidx, didx) => {
        return formatTime(ts);
      },
    },
    {
      label: "Min",
      scale: "y",
      width: lineWidth,
      stroke: cbmode ? "rgb(0,255,0)" : "red",
    },
    {
      label: "Avg",
      scale: "y",
      width: lineWidth,
      stroke: "black",
    },
    {
      label: "Max",
      scale: "y",
      width: lineWidth,
      stroke: cbmode ? "rgb(0,0,255)" : "green",
    }
  ];

  const plotBands = [
    { series: [5, 4], fill: cbmode ? "rgba(0,0,255,0.1)" : "rgba(0,255,0,0.1)" },
    { series: [4, 3], fill: cbmode ? "rgba(0,255,0,0.1)" : "rgba(255,0,0,0.1)" },
  ];

  const opts = {
    width,
    height,
    title,
    plugins: [legendAsTooltipPlugin()],
    series: plotSeries,
    axes: [
      {
        scale: "x",
        // space: 35,
        rotate: 30,
        show: true,
        label: xlabel,
        values(self, splits) {
          return splits.map(s => xticks[s]);
        }
      },
      {
        scale: "xst",
        show: false,
      },
      {
        scale: "xrt",
        show: false,
      },
      {
        scale: "y",
        grid: { show: true },
        labelFont: "sans-serif",
        label: ylabel + ' (' + yunit + ')'
      },
    ],
    bands: plotBands,
    padding: [5, 10, 0, 0], // 5, 10, -20, 0
    hooks: {
      draw: [
        (u) => {
          // Draw plot type label:
          let textl = "Metric Min/Avg/Max in Duration";
          let textr = "Earlier <- StartTime -> Later";
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

          if (!metricConfig) {
            u.ctx.restore();
            return;
          }

          // TODO: Braucht MetricConf
          let y = u.valToPos(metricConfig?.normal, "y", true);
          u.ctx.save();
          u.ctx.lineWidth = lineWidth;
          u.ctx.strokeStyle = "#000000"; // Black
          u.ctx.setLineDash([5, 5]);
          u.ctx.beginPath();
          u.ctx.moveTo(u.bbox.left, y);
          u.ctx.lineTo(u.bbox.left + u.bbox.width, y);
          u.ctx.stroke();
          u.ctx.restore();
        },
      ]
    },
    scales: {
      x: { time: false },
      xst: { time: false },
      xrt: { time: false },
      y: maxY ? { min: 0, max: (maxY * 1.1) } : {auto: true}, // Add some space to upper render limit
    },
    legend: {
      // Display legend
      show: true,
      live: true,
    },
    cursor: { 
      drag: { x: true, y: true },
    }
  };

  // RENDER HANDLING
  let plotWrapper = null;
  let uplot = null;
  let timeoutId = null;

  function render(ren_width, ren_height) {
    if (!uplot) {
      opts.width = ren_width;
      opts.height = ren_height;
      uplot = new uPlot(opts, data, plotWrapper); // Data is uplot formatted [[X][Ymin][Yavg][Ymax]]
    } else {
      uplot.setSize({ width: ren_width, height: ren_height });
    }
  }

  function onSizeChange(chg_width, chg_height) {
    if (!uplot) return;
    if (timeoutId != null) clearTimeout(timeoutId);
    timeoutId = setTimeout(() => {
      timeoutId = null;
      render(chg_width, chg_height);
    }, 200);
  }

  onMount(() => {
    if (plotWrapper) {
      render(width, height);
    }
  });

  onDestroy(() => {
    if (timeoutId != null) clearTimeout(timeoutId);
    if (uplot) uplot.destroy();
  });

  // This updates plot on all size changes if wrapper (== data) exists
  $: if (plotWrapper) {
    onSizeChange(width, height);
  }

</script>

<!-- Define $width Wrapper and NoData Card -->
{#if data && data[0].length > 0}
  <div bind:this={plotWrapper} bind:clientWidth={width}
        style="background-color: rgba(255, 255, 255, 1.0);" class="rounded"
  />
{:else}
  <Card body color="warning" class="mx-4"
    >Cannot render plot: No series data returned for <code>{metric}</code></Card
  >
{/if}
