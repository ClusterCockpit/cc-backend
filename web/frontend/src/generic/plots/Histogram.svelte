<!--
    @component Histogram Plot based on uPlot Bars

    Properties:
    - `data [[],[]]`: uPlot data structure array ( [[],[]] == [X, Y] )
    - `usesBins Bool?`: If X-Axis labels are bins ("XX-YY") [Default: false]
    - `width Number?`: Plot width (reactively adaptive) [Default: 500]
    - `height Number?`: Plot height (reactively adaptive) [Default: 300]
    - `title String?`: Plot title [Default: ""]
    - `xlabel String?`: Plot X axis label [Default: ""]
    - `xunit String?`: Plot X axis unit [Default: ""]
    - `ylabel String?`: Plot Y axis label [Default: ""]
    - `yunit String?`: Plot Y axis unit [Default: ""]
 -->

<script>
  import uPlot from "uplot";
  import { getContext, onMount, onDestroy, createEventDispatcher } from "svelte";
  import { formatNumber } from "../units.js";
  import { Card } from "@sveltestrap/sveltestrap";

  export let data;
  export let usesBins = false;
  export let width = null;
  export let height = 250;
  export let title = "";
  export let xlabel = "";
  export let xunit = "";
  export let xtime = false;
  export let ylabel = "";
  export let yunit = "";
  export let zoomState = null;
  export let lastBinCount = null;
  export let zoomableHistogram = false;

  const { bars } = uPlot.paths;
  const dispatch = createEventDispatcher();

  const drawStyles = {
    bars: 1,
    points: 2,
  };

  const binCounts = xtime ? [24, 48, 96, 144, 288, 720, 1440] : [10, 20, 50, 100, 200]; // , 500, 1000

  function formatTime(t) {
    if (t !== null) {
      if (isNaN(t)) {
        return t;
      } else {
        const tAbs = Math.abs(t);
        const h = Math.floor(tAbs / 3600);
        const m = Math.floor((tAbs % 3600) / 60);
        // Re-Add "negativity" to time ticks only as string, so that if-cases work as intended
        if (h == 0) return `${m}m`;
        else if (m == 0) return `${h}h`;
        else return `${h}:${m}h`;
      }
    }
  }

  function paths(u, seriesIdx, idx0, idx1, extendGap, buildClip) {
    let s = u.series[seriesIdx];
    let style = s.drawStyle;

    let renderer = // If bars to wide, change here
      style == drawStyles.bars ? bars({ size: [0.75, 100] }) : () => null;

    return renderer(u, seriesIdx, idx0, idx1, extendGap, buildClip);
  }

  // converts the legend into a simple tooltip
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

      // hide series color markers
      const idents = legendEl.querySelectorAll(".u-marker");

      for (let i = 0; i < idents.length; i++) idents[i].style.display = "none";

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
      //	overEl.style.overflow = "visible";
    }

    function update(u) {
      const { left, top } = u.cursor;
      legendEl.style.transform =
        "translate(" + (left + 15) + "px, " + (top + 15) + "px)";
    }

    return {
      hooks: {
        init: init,
        setCursor: update,
      },
    };
  }

  let plotWrapper = null;
  let uplot = null;
  let timeoutId = null;

  function render() {
    let opts = {
      hooks: {
        init: [
          (u) => {
            if (zoomableHistogram) {
              u.over.addEventListener("dblclick", (e) => {
                console.log('Dispatch Reset')
                dispatch('zoom', {
                  lastZoomState: {
                    x: { time: false },
                    y: { auto: true }
                  }
                });
              });
            }
          },
        ],
        setScale: [
          (u, key) => {
            if (key === 'x') {
              if (zoomableHistogram) {
                const numX = (u.series[0].idxs[1] - u.series[0].idxs[0])
                if (xtime && numX <= 12 && lastBinCount !== 1440) {
                  // console.log("Dispatch for Duration: ", numX, lastBinCount, binCounts[binCounts.indexOf(lastBinCount) + 1])
                  dispatch('zoom', {
                    durationBinCount: binCounts[binCounts.indexOf(lastBinCount) + 1],
                    lastZoomState: u?.scales,
                  });
                } else if (!xtime && numX <= 6 && lastBinCount !== 200) {
                  // console.log("Dispatch for Metrics: ", numX, lastBinCount, binCounts[binCounts.indexOf(lastBinCount) + 1])
                  dispatch('zoom', {
                    metricBinCount: binCounts[binCounts.indexOf(lastBinCount) + 1],
                    lastZoomState: u?.scales,
                  });
                };
              }
            };
          },
        ]
      },
      width: width,
      height: height,
      title: title,
      plugins: [legendAsTooltipPlugin()],
      cursor: {
        points: {
          size: (u, seriesIdx) => u.series[seriesIdx].points.size * 2.5,
          width: (u, seriesIdx, size) => size / 4,
          stroke: (u, seriesIdx) =>
            u.series[seriesIdx].points.stroke(u, seriesIdx) + "90",
          fill: (u, seriesIdx) => "#fff",
        },
      },
      scales: {
        x: {
          time: false,
        },
      },
      axes: [
        {
          stroke: "#000000",
          // scale: 'x',
          label: xlabel,
          labelGap: 10,
          size: 25,
          incrs: xtime ? [60, 120, 300, 600, 900, 1800, 3600, 7200, 14400] : [1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000],
          border: {
            show: true,
            stroke: "#000000",
          },
          ticks: {
            width: 1 / devicePixelRatio,
            size: 5 / devicePixelRatio,
            stroke: "#000000",
          },
          values: (_, t) => t.map((v) => {
            if (xtime) {
              return formatTime(v);
            } else {
              return formatNumber(v)
            }
          }),
        },
        {
          stroke: "#000000",
          // scale: 'y',
          label: ylabel,
          labelGap: 10,
          size: 35,
          border: {
            show: true,
            stroke: "#000000",
          },
          ticks: {
            width: 1 / devicePixelRatio,
            size: 5 / devicePixelRatio,
            stroke: "#000000",
          },
          values: (_, t) => t.map((v) => {
            return formatNumber(v)
          }),
        },
      ],
      series: [
        {
          label: xunit !== "" ? xunit : null,
          value: (u, ts, sidx, didx) => {
            if (usesBins) {
              const min = u.data[sidx][didx - 1] ? u.data[sidx][didx - 1] : 0;
              const max = u.data[sidx][didx];
              ts = min + " - " + max; // narrow spaces
            } else if (xtime) {
              ts = formatTime(ts);
            }
            return ts;
          },
        },
        Object.assign(
          {
            label: yunit !== "" ? yunit : null,
            width: 1 / devicePixelRatio,
            drawStyle: drawStyles.points,
            lineInterpolation: null,
            paths,
          },
          {
            drawStyle: drawStyles.bars,
            width: 1, // 1 / lastBinCount,
            lineInterpolation: null,
            stroke: "#85abce",
            fill: "#85abce", //  + "1A", // Transparent Fill
          },
        ),
      ],
    };

    if (zoomableHistogram && zoomState) {
      console.log("Apply ZoomState ...", zoomState)
      opts.scales = {...zoomState}
    }

    uplot = new uPlot(opts, data, plotWrapper);
  }

  onMount(() => {
    render();
  });

  onDestroy(() => {
    if (uplot) uplot.destroy();

    if (timeoutId != null) clearTimeout(timeoutId);
  });

  function sizeChanged() {
    if (timeoutId != null) clearTimeout(timeoutId);

    timeoutId = setTimeout(() => {
      timeoutId = null;
      if (uplot) uplot.destroy();

      render();
    }, 200);
  }

  $: sizeChanged(width, height);
</script>

<!-- Define Wrapper and NoData Card within $width -->
<div bind:clientWidth={width}>
  {#if data.length > 0}
    <div bind:this={plotWrapper} />
  {:else}
    <Card class="mx-4" body color="warning"
      >Cannot render histogram: No data!</Card
    >
  {/if}
</div>
