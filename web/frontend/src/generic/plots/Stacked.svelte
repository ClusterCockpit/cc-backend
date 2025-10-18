<!--
  @component Node State/Health Data Stacked Plot Component, based on uPlot; states by timestamp

  Only width/height should change reactively.

  Properties:
  - `metric String?`: The metric name [Default: ""]
  - `width Number?`: The plot width [Default: 0]
  - `height Number?`: The plot height [Default: 300]
  - `data [Array]`: The data object [Default: null]
  - `title String?`: Plot title [Default: ""]
  - `xlabel String?`: Plot X axis label [Default: ""]
  - `ylabel String?`: Plot Y axis label [Default: ""]
  - `yunit String?`: Plot Y axis unit [Default: ""]
  - `xticks Array`: Array containing jobIDs [Default: []]
  - `xinfo Array`: Array containing job information [Default: []]
  - `forResources Bool?`: Render this plot for allocated jobResources [Default: false]
  - `plot Sync Object!`: uPlot cursor synchronization key
-->

<script>
  import uPlot from "uplot";
  import { roundTwoDigits, formatDurationTime, formatUnixTime, formatNumber } from "../units.js";
  import { getContext, onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  // NOTE: Metric Thresholds non-required, Cluster Mixing Allowed

  /* Svelte 5 Props */
  let {
    cluster = "",
    width = 0,
    height = 300,
    data = null,
    xlabel = "",
    ylabel = "",
    yunit = "",
    title = "",
    stateType = "" // Health, Slurm, Both
  } = $props();

  /* Const Init */
  const clusterCockpitConfig = getContext("cc-config");
  const lineWidth = clusterCockpitConfig?.plotConfiguration_lineWidth / window.devicePixelRatio || 2;
  const cbmode = clusterCockpitConfig?.plotConfiguration_colorblindMode || false;

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

  function getOpts(title, series) {
    return {
      scales: {
        x: {
          time: false,
        },
      },
      series
    };
  }

  function getStackedOpts(title, series, data, interp) {
    let opts = getOpts(title, series);

    let interped = interp ? interp(data) : data;

    let stacked = stack(interped, i => false);
    opts.bands = stacked.bands;

    opts.cursor = opts.cursor || {};
    opts.cursor.dataIdx = (u, seriesIdx, closestIdx, xValue) => {
      return data[seriesIdx][closestIdx] == null ? null : closestIdx;
    };

    opts.series.forEach(s => {
      s.value = (u, v, si, i) => data[si][i];

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


  function stack2(series) {
    // for uplot data
    let data = Array(series.length);
    let bands = [];

    let dataLen = series[0].values.length;

    let zeroArr = Array(dataLen).fill(0);

    let stackGroups = new Map();
    let seriesStackKeys = Array(series.length);

    series.forEach((s, si) => {
      let vals = s.values.slice();

      // apply negY
      if (s.negY) {
        for (let i = 0; i < vals.length; i++) {
          if (vals[i] != null)
            vals[i] *= -1;
        }
      }

      if (s.stacking.mode != 'none') {
        let hasPos = vals.some(v => v > 0);
        // derive stacking key
        let stackKey = seriesStackKeys[si] = s.stacking.mode + s.scaleKey + s.stacking.group + (hasPos ? '+' : '-');
        let group = stackGroups.get(stackKey);

        // initialize stacking group
        if (group == null) {
          group = {
            series: [],
            acc: zeroArr.slice(),
            dir: hasPos ? -1 : 1,
          };
          stackGroups.set(stackKey, group);
        }

        // push for bands gen
        group.series.unshift(si);

        let stacked = data[si] = Array(dataLen);
        let { acc } = group;

        for (let i = 0; i < dataLen; i++) {
          let v = vals[i];

          if (v != null)
            stacked[i] = (acc[i] += v);
          else
            stacked[i] = v; // we may want to coerce to 0 here
        }
      }
      else
        data[si] = vals;
    });

    // re-compute by percent
    series.forEach((s, si) => {
      if (s.stacking.mode == 'percent') {
        let group = stackGroups.get(seriesStackKeys[si]);
        let { acc } = group;

        // re-negatify percent
        let sign = group.dir * -1;

        let stacked = data[si];

        for (let i = 0; i < dataLen; i++) {
          let v = stacked[i];

          if (v != null)
            stacked[i] = sign * (v / acc[i]);
        }
      }
    });

    // generate bands between adjacent group series
    stackGroups.forEach(group => {
      let { series, dir } = group;
      let lastIdx = series.length - 1;

      series.forEach((si, i) => {
        if (i != lastIdx) {
          let nextIdx = series[i + 1];
          bands.push({
            // since we're not passing x series[0] for stacking, real idxs are actually +1
            series: [si + 1, nextIdx + 1],
            dir,
          });
        }
      });
    });

    return {
      data,
      bands,
    };
  }

  // UPLOT SERIES INIT //

  const plotSeries = [
      {
        label: "Time",
        scale: "x",
        value: (u, ts, sidx, didx) =>
        (didx == null) ? null : formatUnixTime(ts),
      }
  ]

  if (stateType === "slurm") {
    const resSeries = [
      {
        label: "Idle",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(136, 204, 238)" : "lightblue",
      },
      {
        label: "Allocated",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(30, 136, 229)" : "green",
      },
      {
        label: "Reserved",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(211, 95, 183)" : "magenta",
      },
      {
        label: "Mixed",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(239, 230, 69)" : "yellow",
      },
      {
        label: "Down",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(225, 86, 44)" : "red",
      },
      {
        label: "Unknown",
        scale: "y",
        width: lineWidth,
        stroke: "black",
      }
    ];
    plotSeries.push(...resSeries)
  } else if (stateType === "health") {
    const resSeries = [
      {
        label: "Full",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(30, 136, 229)" : "green",
      },
      {
        label: "Partial",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(239, 230, 69)" : "yellow",
      },
      {
        label: "Failed",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(225, 86, 44)" : "red",
      }
    ];
    plotSeries.push(...resSeries)
  } else {
    const resSeries = [
      {
        label: "Full",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(30, 136, 229)" : "green",
      },
      {
        label: "Partial",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(239, 230, 69)" : "yellow",
      },
      {
        label: "Failed",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(225, 86, 44)" : "red",
      },
      {
        label: "Idle",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(136, 204, 238)" : "lightblue",
      },
      {
        label: "Allocated",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(30, 136, 229)" : "green",
      },
      {
        label: "Reserved",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(211, 95, 183)" : "magenta",
      },
      {
        label: "Mixed",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(239, 230, 69)" : "yellow",
      },
      {
        label: "Down",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(225, 86, 44)" : "red",
      },
      {
        label: "Unknown",
        scale: "y",
        width: lineWidth,
        stroke: "black",
      }
    ];
    plotSeries.push(...resSeries)
  }

  // UPLOT BAND COLORS //
  // const plotBands = [
  //   { series: [5, 4], fill: cbmode ? "rgba(0,0,255,0.1)" : "rgba(0,255,0,0.1)" },
  //   { series: [4, 3], fill: cbmode ? "rgba(0,255,0,0.1)" : "rgba(255,0,0,0.1)" },
  // ];

  // UPLOT OPTIONS //
  const opts = {
    width,
    height,
    title,
    plugins: [legendAsTooltipPlugin()],
    series: plotSeries,
    axes: [
      {
        scale: "x",
        space: 25, // Tick Spacing
        rotate: 30,
        show: true,
        label: xlabel,
        // values(self, splits) {
        //   return splits.map(s => xticks[s]);
        // }
      },
      {
        scale: "y",
        grid: { show: true },
        labelFont: "sans-serif",
        label: ylabel + (yunit ? ` (${yunit})` : ''),
        // values: (u, vals) => vals.map((v) => formatNumber(v)),
      },
    ],
    // bands: forResources ? [] : plotBands,
    padding: [5, 10, 0, 0],
    // hooks: {
    //   draw: [
    //     (u) => {
    //       // Draw plot type label:
    //       let textl = forResources ? "Job Resources by Type" : "Metric Min/Avg/Max for Job Duration";
    //       let textr = "Earlier <- StartTime -> Later";
    //       u.ctx.save();
    //       u.ctx.textAlign = "start";
    //       u.ctx.fillStyle = "black";
    //       u.ctx.fillText(textl, u.bbox.left + 10, u.bbox.top + 10);
    //       u.ctx.textAlign = "end";
    //       u.ctx.fillStyle = "black";
    //       u.ctx.fillText(
    //         textr,
    //         u.bbox.left + u.bbox.width - 10,
    //         u.bbox.top + 10,
    //       );
    //       u.ctx.restore();
    //       return;
    //     },
    //   ]
    // },
    scales: {
      x: { time: false },
      y: {auto: true, distr: 1},
    },
    legend: {
      // Display legend
      show: true,
      live: true,
    },
    cursor: { 
      drag: { x: true, y: true },
      // sync: { 
      //   key: plotSync.key,
      //   scales: ["x", null],
      // }
    }
  };

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

  // RENDER HANDLING
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
{#if data && data[0].length > 0}
  <div bind:this={plotWrapper} bind:clientWidth={width}
        style="background-color: rgba(255, 255, 255, 1.0);" class="rounded"
  ></div>
{:else}
  <Card body color="warning" class="mx-4 my-2"
    >Cannot render plot: No series data returned for <code>{metric?metric:'job resources'}</code></Card
  >
{/if}
