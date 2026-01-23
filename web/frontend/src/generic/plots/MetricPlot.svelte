<!--
  @component Main plot component, based on uPlot; metricdata values by time

  Only width/height should change reactively.

  Properties:
  - `metric String`: The metric name
  - `scope String?`: Scope of the displayed data [Default: node]
  - `height Number?`: The plot height [Default: 300]
  - `timestep Number`: The timestep used for X-axis rendering
  - `series [GraphQL.Series]`: The metric data object
  - `useStatsSeries Bool?`: If this plot uses the statistics Min/Max/Median representation; automatically set to according bool [Default: false]
  - `statisticsSeries [GraphQL.StatisticsSeries]?`: Min/Max/Median representation of metric data [Default: null]
  - `cluster String?`: Cluster name of the parent job / data [Default: ""]
  - `subCluster String`: Name of the subCluster of the parent job
  - `isShared Bool?`: If this job used shared resources; for additional legend display [Default: false]
  - `forNode Bool?`: If this plot is used for node data display; will render x-axis as negative time with $now as maximum [Default: false]
  - `numhwthreads Number?`: Number of job HWThreads [Default: 0]
  - `numaccs Number?`: Number of job Accelerators [Default: 0]
  - `zoomState Object?`: The last zoom state to preserve on user zoom [Default: null]
  - `thersholdState Object?`: The last threshold state to preserve on user zoom [Default: null]
  - `extendedLegendData Object?`: Additional information to be rendered in an extended legend [Default: null]
  - `onZoom Func`: Callback function to handle zoom-in event
-->

<script>
  import uPlot from "uplot";
  import { formatNumber, formatDurationTime } from "../units.js";
  import { getContext, onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    metric,
    scope = "node",
    width = 0,
    height = 300,
    timestep,
    series,
    useStatsSeries = false,
    statisticsSeries = null,
    cluster = "",
    subCluster,
    isShared = false,
    forNode = false,
    numhwthreads = 0,
    numaccs = 0,
    zoomState = null,
    thresholdState = null,
    extendedLegendData = null,
    plotSync = null,
    enableFlip = false,
    onZoom
  } = $props();

  /* Const Init */
  const clusterCockpitConfig = getContext("cc-config");
  const resampleConfig = getContext("resampling");
  const lineWidth = clusterCockpitConfig.plotConfiguration_lineWidth / window.devicePixelRatio;
  const cbmode = clusterCockpitConfig?.plotConfiguration_colorblindMode || false;
  const renderSleepTime = 200;
  const normalLineColor = "#000000";
  const backgroundColors = {
    normal: "rgba(255, 255, 255, 1.0)",
    caution: cbmode ? "rgba(239, 230, 69, 0.3)" : "rgba(255, 128, 0, 0.3)",
    alert: cbmode ? "rgba(225, 86, 44, 0.3)" : "rgba(255, 0, 0, 0.3)",
  };

  /* Var Init */
  let timeoutId = null;

  /* State Init */
  let plotWrapper = $state(null);
  let uplot = $state(null);

  /* Derived */
  const subClusterTopology = $derived(getContext("getHardwareTopology")(cluster, subCluster));
  const metricConfig = $derived(getContext("getMetricConfig")(cluster, subCluster, metric));
  const usesMeanStatsSeries = $derived((statisticsSeries?.mean && statisticsSeries.mean.length != 0));
  const resampleTrigger = $derived(resampleConfig?.trigger ? Number(resampleConfig.trigger) : null);
  const resampleResolutions = $derived(resampleConfig?.resolutions ? [...resampleConfig.resolutions] : null);
  const resampleMinimum = $derived(resampleConfig?.resolutions ? Math.min(...resampleConfig.resolutions) : null);
  const thresholds = $derived(findJobAggregationThresholds(
    subClusterTopology,
    metricConfig,
    scope,
    numhwthreads,
    numaccs
  ));
  const longestSeries = $derived.by(() => {
    if (useStatsSeries) {
      return usesMeanStatsSeries ? statisticsSeries?.mean?.length : statisticsSeries?.median?.length;
    } else {
      return series.reduce((n, series) => Math.max(n, series.data.length), 0);
    }
  });
  const maxX = $derived(longestSeries * timestep);
  const maxY = $derived.by(() => {
    let pendingY = 0;
    if (useStatsSeries) {
      pendingY = statisticsSeries.max.reduce(
        (max, x) => Math.max(max, x),
        thresholds?.normal,
      ) || thresholds?.normal
    } else { 
      pendingY = series.reduce(
          (max, series) => Math.max(max, series?.statistics?.max),
          thresholds?.normal,
        ) || thresholds?.normal;
    }

    if (pendingY >= 10 * thresholds.peak) {
      // Hard y-range render limit if outliers in series data
      return (10 * thresholds.peak);
    } else {
      return pendingY;
    }
  });
  const plotBands = $derived.by(() => {
    if (useStatsSeries) {
      return [
        { series: [2, 3], fill: cbmode ? "rgba(0,0,255,0.1)" : "rgba(0,255,0,0.1)" },
        { series: [3, 1], fill: cbmode ? "rgba(0,255,0,0.1)" : "rgba(255,0,0,0.1)" },
      ];
    };
    return null;
  })
  const plotData = $derived.by(() => {
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
    if (useStatsSeries) {
      pendingData.push(statisticsSeries.min);
      pendingData.push(statisticsSeries.max);
      if (usesMeanStatsSeries) {
        pendingData.push(statisticsSeries.mean);
      } else {
        pendingData.push(statisticsSeries.median);
      }

    } else {
      for (let i = 0; i < series.length; i++) {
        pendingData.push(series[i].data);
      };
    };
    return pendingData;
  })
  const plotSeries = $derived.by(() => {
    let pendingSeries = [
      // Note: X-Legend Will not be shown as soon as Y-Axis are in extendedMode
      {
        label: "Runtime",
        value: (u, ts, sidx, didx) =>
        (didx == null) ? null : formatDurationTime(ts, forNode),
      }
    ];
    // Y
    if (useStatsSeries) {
      pendingSeries.push({
        label: "min",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(0,255,0)" : "red",
      });
      pendingSeries.push({
        label: "max",
        scale: "y",
        width: lineWidth,
        stroke: cbmode ? "rgb(0,0,255)" : "green",
      });
      pendingSeries.push({
        label: usesMeanStatsSeries ? "mean" : "median",
        scale: "y",
        width: lineWidth,
        stroke: "black",
      });

    } else {
      for (let i = 0; i < series?.length; i++) {
        // Default
        if (!extendedLegendData) {
          pendingSeries.push({
            label: 
              scope === "node"
              ? series[i].hostname
              : scope === "accelerator"
                ? 'Acc #' + (i + 1) // series[i].id.slice(9, 14) | Too Hardware Specific
                : scope + " #" + (i + 1),
            scale: "y",
            width: lineWidth,
            stroke: lineColor(i, clusterCockpitConfig.plotConfiguration_colorScheme),
          });
        }
        // Extended Legend For NodeList
        else {
          pendingSeries.push({
            label: 
              scope === "node"
                ? series[i].hostname
                : scope === "accelerator"
                  ? 'Acc #' + (i + 1) // series[i].id.slice(9, 14) | Too Hardware Specific
                  : scope + " #" + (i + 1),
            scale: "y",
            width: lineWidth,
            stroke: lineColor(i, clusterCockpitConfig.plotConfiguration_colorScheme),
            values: (u, sidx, idx) => {
              // "i" = "sidx - 1" : sidx contains x-axis-data
              if (idx == null)
                return {
                  time: '-',
                  value: '-',
                  user: '-',
                  job: '-'
                };

              if (series[i].id in extendedLegendData) {
                return {
                  time: formatDurationTime(plotData[0][idx], forNode),
                  value: plotData[sidx][idx],
                  user: extendedLegendData[series[i].id].user,
                  job: extendedLegendData[series[i].id].job,
                };
              } else {
                return {
                  time: formatDurationTime(plotData[0][idx], forNode),
                  value: plotData[sidx][idx],
                  user: '-',
                  job: '-',
                };
              }
            }
          });
        }
      };
    };
    return pendingSeries;
  })

  /* Effects */
  $effect(() => {
    if (!useStatsSeries && statisticsSeries != null) useStatsSeries = true;
  })

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

  // removed arg "subcluster": input metricconfig and topology now directly derived from subcluster
  function findJobAggregationThresholds(
    subClusterTopology,
    metricConfig,
    scope,
    numhwthreads,
    numaccs
  ) {

    if (!subClusterTopology || !metricConfig || !scope) {
      console.warn("Argument missing for findJobAggregationThresholds!");
      return null;
    }

    // handle special *-stat scopes
    if (scope.match(/(.*)-stat$/)) {
      const statParts = scope.split('-');
      scope = statParts[0]
    }

    if (metricConfig?.aggregation == "avg") {
      // Return as Configured
      return {
        normal: metricConfig.normal,
        caution: metricConfig.caution,
        alert: metricConfig.alert,
        peak: metricConfig.peak,
      };
    }

    if (metricConfig?.aggregation == "sum") {
      // Scale Thresholds
      let fraction;
      if (numaccs > 0) fraction = subClusterTopology.accelerators.length / numaccs;
      else if (numhwthreads > 0) fraction = subClusterTopology.core.length / numhwthreads;
      else fraction = 1; // Fallback

      let divisor;
      // Exclusive: Fraction = 1; Shared: Fraction > 1
      if (scope == 'node')              divisor = fraction;
      // Cap divisor at number of available sockets or domains
      else if (scope == 'socket')       divisor = (fraction < subClusterTopology.socket.length) ? subClusterTopology.socket.length : fraction;
      else if (scope == "memoryDomain") divisor = (fraction < subClusterTopology.memoryDomain.length) ? subClusterTopology.socket.length : fraction;
      // Use Maximum Division for Smallest Scopes
      else if (scope == "core")         divisor = subClusterTopology.core.length;
      else if (scope == "hwthread")     divisor = subClusterTopology.core.length; // alt. name for core
      else if (scope == "accelerator")  divisor = subClusterTopology.accelerators.length;
      else {
        console.log('Unknown scope, return default aggregation thresholds for sum', scope)
        divisor = 1;
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

  // UPLOT PLUGIN // converts the legend into a simple tooltip
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
        minWidth: extendedLegendData ? "300px" : "100px",
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
        useStatsSeries || // Min/Max/Median Self-Explanatory
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

    if (dataSize <= 12 || useStatsSeries) {
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

  // RETURN BG COLOR FROM THRESHOLD
  function backgroundColor() {
    if (
      clusterCockpitConfig.plotConfiguration_colorBackground == false ||
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

  function lineColor(index, colors) {
    return colors[index % colors.length];
  }

  function render(ren_width, ren_height) {
    // Set Options
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
          values: (_, vals) => vals.map((v) => formatDurationTime(v, forNode)),
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
        init: [
          (u) => {
            /* IF Zoom Enabled */
            if (resampleConfig && !forNode) {
              u.over.addEventListener("dblclick", (e) => {
                // console.log('Dispatch: Zoom Reset')
                onZoom({
                  lastZoomState: {
                    x: { time: false },
                    y: { auto: true }
                  }
                });
              });
            };
          },
        ],
        draw: [
          (u) => {
            // Draw plot type label:
            let textl = `${scope}${plotSeries.length > 2 ? "s" : ""}${
              useStatsSeries
                ? (usesMeanStatsSeries ? ": min/mean/max" : ": min/median/max")
                : metricConfig != null && scope != metricConfig.scope
                  ? ` (${metricConfig.aggregation})`
                  : ""
            }`;
            let textr = `${isShared && scope != "core" && scope != "accelerator" ? "[Shared]" : ""}`;
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
        setScale: [
          (u, key) => { // If ZoomResample is Configured && Not System/Node View
            if (resampleConfig && !forNode && key === 'x') {
              const numX = (u.series[0].idxs[1] - u.series[0].idxs[0])
              if (numX <= resampleTrigger && timestep !== resampleMinimum) {
                /* Get closest zoom level; prevents multiple iterative zoom requests for big zoom-steps (e.g. 600 -> 300 -> 120 -> 60) */
                // Which resolution to theoretically request to achieve 30 or more visible data points:
                const target = (numX * timestep) / resampleTrigger
                // Which configured resolution actually matches the closest to theoretical target:
                const closest = resampleResolutions.reduce(function(prev, curr) {
                  return (Math.abs(curr - target) < Math.abs(prev - target) ? curr : prev);
                });
                // Prevents non-required dispatches
                if (timestep !== closest) {
                  // console.log('Dispatch: Zoom with Res from / to', timestep, closest)
                  onZoom({
                    newRes: closest,
                    lastZoomState: u?.scales,
                    lastThreshold: thresholds?.normal
                  });
                }
              } else {
                // console.log('Dispatch: Zoom Update States')
                onZoom({
                  lastZoomState: u?.scales,
                  lastThreshold: thresholds?.normal
                });
              };
            };
          },
        ]
      },
      scales: {
        x: { time: false },
        y: maxY ? { min: 0, max: (maxY * 1.1) } : {auto: true}, // Add some space to upper render limit
      },
      legend: {
        // Display legend until max 12 Y-dataseries
        show: series.length <= 12 || useStatsSeries,
        live: series.length <= 12 || useStatsSeries,
      },
      cursor: { 
        drag: { x: true, y: true },
      }
    };

    // Handle Render
    if (!uplot) {
      opts.width = ren_width;
      opts.height = ren_height;
      
      if (plotSync) {
        opts.cursor.sync = { 
          key: plotSync.key,
          scales: ["x", null],
        }
      }

      if (zoomState && metricConfig?.aggregation == "avg") {
        opts.scales = {...zoomState}
      } else if (zoomState && metricConfig?.aggregation == "sum") {
        // Allow Zoom In === Ymin changed
        if (zoomState.y.min !== 0) { // scope change?: only use zoomState if thresholds match
          if ((thresholdState === thresholds?.normal)) { opts.scales = {...zoomState} };
        } // else: reset scaling to default
      }

      uplot = new uPlot(opts, plotData, plotWrapper);
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
    }, renderSleepTime);
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
{#if series[0]?.data && series[0].data.length > 0}
  <div bind:this={plotWrapper} bind:clientWidth={width}
        style="background-color: {backgroundColor()};" class={forNode ? 'py-2 rounded' : 'rounded'}
  ></div>
{:else}
  <Card body color="warning" class="mx-4"
    >Cannot render plot: No series data returned for <code>{metric}</code></Card
  >
{/if}
