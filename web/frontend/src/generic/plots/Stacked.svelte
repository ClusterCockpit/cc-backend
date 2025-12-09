<!--
  @component Node State/Health Data Stacked Plot Component, based on uPlot; states by timestamp

  Properties:
  - `width Number?`: The plot width [Default: 0]
  - `height Number?`: The plot height [Default: 300]
  - `data [Array]`: The data object [Default: null]
  - `xlabel String?`: Plot X axis label [Default: ""]
  - `ylabel String?`: Plot Y axis label [Default: ""]
  - `yunit String?`: Plot Y axis unit [Default: ""]
  - `title String?`: Plot title [Default: ""]
  - `stateType String?`: Which states to render, affects plot render config [Options: Health, Node; Default: ""]
-->

<script>
  import uPlot from "uplot";
  import { formatUnixTime } from "../units.js";
  import { getContext, onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    width = 0,
    height = 300,
    data = null,
    xlabel = "",
    ylabel = "",
    yunit = "",
    title = "",
    stateType = "" // Health, Node
  } = $props();

  /* Const Init */
  const clusterCockpitConfig = getContext("cc-config");
  const lineWidth = clusterCockpitConfig?.plotConfiguration_lineWidth / window.devicePixelRatio || 2;
  const cbmode = clusterCockpitConfig?.plotConfiguration_colorblindMode || false;
  const seriesConfig = {
    full: {
      label: "Full",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(0, 110, 0, 0.4)" : "rgba(0, 128, 0, 0.4)",
      stroke: cbmode ? "rgb(0, 110, 0)" : "green",
    },
    partial: {
      label: "Partial",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(235, 172, 35, 0.4)" : "rgba(255, 215, 0, 0.4)",
      stroke: cbmode ? "rgb(235, 172, 35)" : "gold",
    },
    failed: {
      label: "Failed",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgb(181, 29, 20, 0.4)" : "rgba(255, 0, 0, 0.4)",
      stroke: cbmode ? "rgb(181, 29, 20)" : "red",
    },
    idle: {
      label: "Idle",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(0, 140, 249, 0.4)" : "rgba(0, 0, 255, 0.4)",
      stroke: cbmode ? "rgb(0, 140, 249)" : "blue",
    },
    allocated: {
      label: "Allocated",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(0, 110, 0, 0.4)" : "rgba(0, 128, 0, 0.4)",
      stroke: cbmode ? "rgb(0, 110, 0)" : "green",
    },
    reserved: {
      label: "Reserved",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(209, 99, 230, 0.4)" : "rgba(255, 0, 255, 0.4)",
      stroke: cbmode ? "rgb(209, 99, 230)" : "magenta",
    },
    mixed: {
      label: "Mixed",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(235, 172, 35, 0.4)" : "rgba(255, 215, 0, 0.4)",
      stroke: cbmode ? "rgb(235, 172, 35)" : "gold",
    },
    down: {
      label: "Down",
      scale: "y",
      width: lineWidth,
      fill: cbmode ? "rgba(181, 29 ,20, 0.4)" : "rgba(255, 0, 0, 0.4)",
      stroke: cbmode ? "rgb(181, 29, 20)" : "red",
    },
    unknown: {
      label: "Unknown",
      scale: "y",
      width: lineWidth,
      fill: "rgba(0, 0, 0, 0.4)",
      stroke: "black",
    }
  };

  // Data Prep For uPlot
  const sortedData = data.sort((a, b) => a.state.localeCompare(b.state));
  const collectLabel = sortedData.map(d => d.state);
  // Align Data to Timesteps, Introduces 'undefied' as placeholder, reiterate and set those to 0
  const collectData  = uPlot.join(sortedData.map(d => [d.times, d.counts])).map(d => d.map(i => i ? i : 0));

  // STACKED CHART FUNCTIONS //
  function stack(data, omit) {
    let data2 = [];
    let bands = [];
    let d0Len = data[0].length;
    let accum = Array(d0Len);

    for (let i = 0; i < d0Len; i++)
      accum[i] = 0;

    for (let i = 1; i < data.length; i++)
      data2.push(omit(i) ? data[i] : data[i].map((v, i) => (accum[i] += +v)));

    for (let i = 1; i < data.length; i++)
      !omit(i) && bands.push({
        series: [
          data.findIndex((s, j) => j > i && !omit(j)),
          i,
        ],
      });

    bands = bands.filter(b => b.series[1] > -1);

    return {
      data: [data[0]].concat(data2),
      bands,
    };
  }

  function getStackedOpts(title, width, height, series, data) {
    let opts = {
      width,
      height,
      title,
      plugins: [legendAsTooltipPlugin()],
      series,
      axes: [
        {
          scale: "x",
          space: 25, // Tick Spacing
          rotate: 30,
          show: true,
          label: xlabel,
          values(self, splits) {
            return splits.map(s => formatUnixTime(s));
          }
        },
        {
          scale: "y",
          grid: { show: true },
          // labelFont: "sans-serif",
          label: ylabel + (yunit ? ` (${yunit})` : ''),
          // values: (u, vals) => vals.map((v) => formatNumber(v)),
        },
      ],
      padding: [5, 10, 0, 0],
      scales: {
        x: { time: false },
        y: { auto: true, distr: 1 },
      },
      legend: {
        show: true,
      },
      cursor: { 
        drag: { x: true, y: true },
      }
    };

    let stacked = stack(data, i => false);
    opts.bands = stacked.bands;

    opts.cursor = opts.cursor || {};
    opts.cursor.dataIdx = (u, seriesIdx, closestIdx, xValue) => {
      return data[seriesIdx][closestIdx] == null ? null : closestIdx;
    };

    opts.series.forEach(s => {
      // Format Time Info from Unix TS to LocalTimeString
      s.value = (u, v, si, i) => (si === 0) ? formatUnixTime(data[si][i]) : data[si][i];

      s.points = s.points || {};

      // scan raw unstacked data to return only real points
      s.points.filter = (u, seriesIdx, show, gaps) => {
        if (show) {
          let pts = [];
          data[seriesIdx].forEach((v, i) => {
            v != null && pts.push(i);
          });
          return pts;
        }
      }
    });

    // force 0 to be the sum minimum this instead of the bottom series
    opts.scales.y = {
      range: (u, min, max) => {
        let minMax = uPlot.rangeNum(0, max, 0.1, true);
        return [0, minMax[1]];
      }
    };

    // restack on toggle
    opts.hooks = {
      setSeries: [
        (u, i) => {
          let stacked = stack(data, i => !u.series[i].show);
          u.delBand(null);
          stacked.bands.forEach(b => u.addBand(b));
          u.setData(stacked.data);
        }
      ],
    };

    return {opts, data: stacked.data};
  }

  // UPLOT PLUGIN: Converts the legend into a simple tooltip
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
        minWidth: "175px",
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
      // const idents = legendEl.querySelectorAll(".u-marker");
      // for (let i = 0; i < idents.length; i++)
      //   idents[i].style.display = "none";

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
      const internalWidth = u?.over?.querySelector(".u-legend")?.offsetWidth ? u.over.querySelector(".u-legend").offsetWidth : 0;
      if (left < (width/2)) {
        legendEl.style.transform = "translate(" + (left + 15) + "px, " + (top + 15) + "px)";
      } else {
        legendEl.style.transform = "translate(" + (left - internalWidth - 15) + "px, " + (top + 15) + "px)";
      }
    }

    return {
      hooks: {
        init: init,
        setCursor: update,
      },
    };
  }

  // UPLOT SERIES INIT
  const plotSeries = [
    {
      label: "Time",
      scale: "x"
    },
    ...collectLabel.map(l => seriesConfig[l])
  ]

  /* Var Init */
  let timeoutId = null;
  let uplot = null;

  /* State Init */
  let plotWrapper = $state(null);

  /* Effects */
  $effect(() => {
    if (plotWrapper) {
      onSizeChange(width, height);
    }
  });

  /* Functions */
  function render(ren_width, ren_height) {
    if (!uplot) {
      let { opts, data } = getStackedOpts(title, ren_width, ren_height, plotSeries, collectData);
      uplot = new uPlot(opts, data, plotWrapper); // Data is uplot formatted [[X][Y1][Y2]...]
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

  /* On Mount */
  onMount(() => {
    if (plotWrapper) {
      render(width, height);
    }
  });

  /* On Destroy */
  onDestroy(() => {
    if (timeoutId != null) clearTimeout(timeoutId);
    if (uplot) uplot.destroy();
  });
</script>

<!-- Define $width Wrapper and NoData Card -->
{#if data && collectData[0].length > 0}
  <div bind:this={plotWrapper} bind:clientWidth={width}
        style="background-color: rgba(255, 255, 255, 1.0);" class="rounded"
  ></div>
{:else}
  <Card body color="warning" class="mx-4 my-2"
    >Cannot render plot: No series data returned for <code>{stateType} State Stacked Chart</code></Card
  >
{/if}
