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
  import { roundTwoDigits, formatTime, formatNumber } from "../units.js";
  import { getContext, onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  export let metric = "";
  export let width = 0;
  export let height = 300;
  export let data = null;
  export let xlabel = "";
  export let xticks = [];
  export let ylabel = "";
  export let yunit = "";
  export let title = "";
  export let forResources = false;
  export let plotSync;

  // NOTE: Metric Thresholds non-required, Cluster Mixing Allowed

  const clusterCockpitConfig = getContext("cc-config");
  const lineWidth = clusterCockpitConfig.plot_general_lineWidth / window.devicePixelRatio;
  const cbmode = clusterCockpitConfig?.plot_general_colorblindMode || false;

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
  ]

  if (forResources) {
    const resSeries = [
      {
        label: "Nodes",
        scale: "y",
        width: lineWidth,
        stroke: "black",
      },
      {
        label: "Threads",
        scale: "y",
        width: lineWidth,
        stroke: "rgb(0,0,255)",
      },
      {
        label: "Accelerators",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(0,255,0)" : "red",
      }
    ];
    plotSeries.push(...resSeries)
  } else {
    const statsSeries = [
      {
        label: "Min",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(0,255,0)" : "red",
        value: (u, ts, sidx, didx) => {
          return `${roundTwoDigits(ts)} ${yunit}`;
        },
      },
      {
        label: "Avg",
        scale: "y",
        width: lineWidth,
        stroke: "black",
        value: (u, ts, sidx, didx) => {
          return `${roundTwoDigits(ts)} ${yunit}`;
        },
      },
      {
        label: "Max",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(0,0,255)" : "green",
        value: (u, ts, sidx, didx) => {
          return `${roundTwoDigits(ts)} ${yunit}`;
        },
      }
    ];
    plotSeries.push(...statsSeries)
  };

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
        label: ylabel + (yunit ? ` (${yunit})` : ''),
        values: (u, vals) => vals.map((v) => formatNumber(v)),
      },
    ],
    bands: forResources ? [] : plotBands,
    padding: [5, 10, 0, 0],
    hooks: {
      draw: [
        (u) => {
          // Draw plot type label:
          let textl = forResources ? "Job Resources by Type" : "Metric Min/Avg/Max for Job Duration";
          let textr = "Earlier <- StartTime -> Later";
          u.ctx.save();
          u.ctx.textAlign = "start";
          u.ctx.fillStyle = "black";
          u.ctx.fillText(textl, u.bbox.left + 10, u.bbox.top + 10);
          u.ctx.textAlign = "end";
          u.ctx.fillStyle = "black";
          u.ctx.fillText(
            textr,
            u.bbox.left + u.bbox.width - 10,
            u.bbox.top + 10,
          );
          u.ctx.restore();
          return;
        },
      ]
    },
    scales: {
      x: { time: false },
      xst: { time: false },
      xrt: { time: false },
      y: {auto: true, distr: forResources ? 3 : 1},
    },
    legend: {
      // Display legend
      show: true,
      live: true,
    },
    cursor: { 
      drag: { x: true, y: true },
      sync: { 
        key: plotSync.key,
        scales: ["x", null],
      }
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
      plotSync.sub(uplot)
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
