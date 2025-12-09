<!--
  @component Roofline Model Plot based on uPlot

  Properties:
  - `data [null, [], []]`: Roofline Data Structure, see below for details [Default: null]
  - `allowSizeChange Bool?`: If dimensions of rendered plot can change [Default: false]
  - `subCluster GraphQL.SubCluster?`: SubCluster Object; contains required topology information [Default: null]
  - `width Number?`: Plot width (reactively adaptive) [Default: 600]
  - `height Number?`: Plot height (reactively adaptive) [Default: 380]

  Data Format:
  - `data = [null, [], []]` 
    - Index 0: null-axis required for scatter
    - Index 1: Array of XY-Arrays for Scatter
    - Index 2: Optional Time Info
  - `data[1][0] = [100, 200, 500, ...]`
    - X Axis: Intensity (Vals up to clusters' flopRateScalar value)
  - `data[1][1] = [1000, 2000, 1500, ...]`
    - Y Axis: Performance (Vals up to clusters' flopRateSimd value)
  - `data[2] = [0.1, 0.15, 0.2, ...]`
    - Color Code: Time Information (Floats from 0 to 1) (Optional)
-->
<script>
  import uPlot from "uplot";
  import { formatNumber } from "../units.js";
  import { onMount, onDestroy } from "svelte";
  import { Card } from "@sveltestrap/sveltestrap";
  import { roundTwoDigits } from "../units.js";

  /* Svelte 5 Props */
  let {
    roofData = null,
    jobsData = null,
    nodesData = null,
    cluster = null,
    subCluster = null,
    allowSizeChange = false,
    useColors = true,
    useLegend = true,
    width = 600,
    height = 380,
  } = $props();

  /* Const Init */
  const lineWidth = clusterCockpitConfig.plotConfiguration_lineWidth;
  const cbmode = clusterCockpitConfig?.plotConfiguration_colorblindMode || false;

  /* Var Init */
  let timeoutId = null;

  /* State Init */
  let plotWrapper = $state(null);
  let uplot = $state(null);

  /* Effect */
  $effect(() => {
    if (allowSizeChange) sizeChanged(width, height);
  });

  // Copied Example Vars for Uplot Bubble
  // https://developer.mozilla.org/en-US/docs/Web/API/CanvasRenderingContext2D/isPointInPath
  let qt;
  let hRect;
  let pxRatio;
  function setPxRatio() {
    pxRatio = uPlot.pxRatio;
  }
  setPxRatio();
  window.addEventListener('dppxchange', setPxRatio);
  // let minSize = 6;
  let maxSize = 60;
  // let maxArea = Math.PI * (maxSize / 2) ** 2;
  // let minArea = Math.PI * (minSize / 2) ** 2;

  /* Functions */
  // Helper
  function pointWithin(px, py, rlft, rtop, rrgt, rbtm) {
    return px >= rlft && px <= rrgt && py >= rtop && py <= rbtm;
  }

  function getGradientR(x) {
    if (x < 0.5) return 0;
    if (x > 0.75) return 255;
    x = (x - 0.5) * 4.0;
    return Math.floor(x * 255.0);
  }
  function getGradientG(x) {
    if (x > 0.25 && x < 0.75) return 255;
    if (x < 0.25) x = x * 4.0;
    else x = 1.0 - (x - 0.75) * 4.0;
    return Math.floor(x * 255.0);
  }
  function getGradientB(x) {
    if (x < 0.25) return 255;
    if (x > 0.5) return 0;
    x = 1.0 - (x - 0.25) * 4.0;
    return Math.floor(x * 255.0);
  }
  function getRGB(c, transparent = false) {
    if (transparent) return `rgba(${cbmode ? '0' : getGradientR(c)}, ${getGradientG(c)}, ${getGradientB(c)}, 0.5)`;
    else return `rgb(${cbmode ? '0' : getGradientR(c)}, ${getGradientG(c)}, ${getGradientB(c)})`;
  }
  function nearestThousand(num) {
    return Math.ceil(num / 1000) * 1000;
  }
  function lineIntersect(x1, y1, x2, y2, x3, y3, x4, y4) {
    let l = (y4 - y3) * (x2 - x1) - (x4 - x3) * (y2 - y1);
    let a = ((x4 - x3) * (y1 - y3) - (y4 - y3) * (x1 - x3)) / l;
    return {
      x: x1 + a * (x2 - x1),
      y: y1 + a * (y2 - y1),
    };
  }

  // quadratic scaling (px area)
  // function getSize(value, minValue, maxValue) {
  //   let pct = value / maxValue;
  //   // clamp to min area
  //   //let area = Math.max(maxArea * pct, minArea);
  //   let area = maxArea * pct;
  //   return Math.sqrt(area / Math.PI) * 2;
  // }

  // function getSizeMinMax(u) {
  //   let minValue = Infinity;
  //   let maxValue = -Infinity;
  //   for (let i = 1; i < u.series.length; i++) {
  //     let sizeData = u.data[i][2];
  //     for (let j = 0; j < sizeData.length; j++) {
  //       minValue = Math.min(minValue, sizeData[j]);
  //       maxValue = Math.max(maxValue, sizeData[j]);
  //     }
  //   }
  //   return [minValue, maxValue];
  // }

  // Quadtree Object (TODO: Split and Import)
  class Quadtree {
    constructor (x, y, w, h, l) {
      let t = this;
      t.x = x;
      t.y = y;
      t.w = w;
      t.h = h;
      t.l = l || 0;
      t.o = [];
      t.q = null;
      t.MAX_OBJECTS = 10;
      t.MAX_LEVELS  = 4;
    };

    get quadtree() {
      return "Implement me!";
    }

    split() {
      let t = this,
        x = t.x,
        y = t.y,
        w = t.w / 2,
        h = t.h / 2,
        l = t.l + 1;

      t.q = [
        // top right
        new Quadtree(x + w, y,     w, h, l),
        // top left
        new Quadtree(x,     y,     w, h, l),
        // bottom left
        new Quadtree(x,     y + h, w, h, l),
        // bottom right
        new Quadtree(x + w, y + h, w, h, l),
      ];
    };

    quads(x, y, w, h, cb) {
      let t        = this,
      q            = t.q,
      hzMid        = t.x + t.w / 2,
      vtMid        = t.y + t.h / 2,
      startIsNorth = y     < vtMid,
      startIsWest  = x     < hzMid,
      endIsEast    = x + w > hzMid,
      endIsSouth   = y + h > vtMid;

      // top-right quad
      startIsNorth && endIsEast && cb(q[0]);
      // top-left quad
      startIsWest && startIsNorth && cb(q[1]);
      // bottom-left quad
      startIsWest && endIsSouth && cb(q[2]);
      // bottom-right quad
      endIsEast && endIsSouth && cb(q[3]);
    };

    add(o) {
      let t = this;

      if (t.q != null) {
        t.quads(o.x, o.y, o.w, o.h, q => {
          q.add(o);
        });
      }
      else {
        let os = t.o;

        os.push(o);

        if (os.length > t.MAX_OBJECTS && t.l < t.MAX_LEVELS) {
          t.split();

          for (let i = 0; i < os.length; i++) {
            let oi = os[i];

            t.quads(oi.x, oi.y, oi.w, oi.h, q => {
              q.add(oi);
            });
          }

          t.o.length = 0;
        }
      }
    };

    get(x, y, w, h, cb) {
      let t = this;
      let os = t.o;

      for (let i = 0; i < os.length; i++)
        cb(os[i]);

      if (t.q != null) {
        t.quads(x, y, w, h, q => {
          q.get(x, y, w, h, cb);
        });
      }
    }

    clear() {
      this.o.length = 0;
      this.q = null;
    }
  }

  // Dot Renderer
  const makeDrawPoints = (opts) => {
    let {/*size, disp,*/ transparentFill, each = () => {}} = opts;
    const sizeBase = 6 * pxRatio;

    return (u, seriesIdx, idx0, idx1) => {
      uPlot.orient(u, seriesIdx, (series, dataX, dataY, scaleX, scaleY, valToPosX, valToPosY, xOff, yOff, xDim, yDim, moveTo, lineTo, rect, arc) => {
        let d = u.data[seriesIdx];
        let strokeWidth = 1;
        let deg360 = 2 * Math.PI;
        /* Alt.: Sizes based on other Data Rows */
        // let sizes = disp.size.values(u, seriesIdx, idx0, idx1);

        u.ctx.save();
        u.ctx.rect(u.bbox.left, u.bbox.top, u.bbox.width, u.bbox.height);
        u.ctx.clip();
        u.ctx.lineWidth = strokeWidth;
        
        // todo: this depends on direction & orientation
        // todo: calc once per redraw, not per path
        let filtLft = u.posToVal(-maxSize / 2, scaleX.key);
        let filtRgt = u.posToVal(u.bbox.width / pxRatio + maxSize / 2, scaleX.key);
        let filtBtm = u.posToVal(u.bbox.height / pxRatio + maxSize / 2, scaleY.key);
        let filtTop = u.posToVal(-maxSize / 2, scaleY.key);

        for (let i = 0; i < d[0].length; i++) {
          if (useColors) {
            u.ctx.strokeStyle = "rgb(0, 0, 0)";
            // Jobs: Color based on Duration
            if (jobsData) {
              //u.ctx.strokeStyle = getRGB(u.data[2][i]);
              u.ctx.fillStyle = getRGB(u.data[2][i], transparentFill);
            // Nodes: Color based on Idle vs. Allocated
            } else if (nodesData) {
              // console.log('In Plot Handler NodesData', nodesData)
              if (nodesData[i]?.schedulerState == "idle") {
                //u.ctx.strokeStyle = "rgb(0, 0, 255)";
                u.ctx.fillStyle = "rgba(0, 0, 255, 0.5)";
              } else if (nodesData[i]?.schedulerState == "allocated") {
                //u.ctx.strokeStyle = "rgb(0, 255, 0)";
                u.ctx.fillStyle = "rgba(0, 255, 0, 0.5)";
              } else if (nodesData[i]?.schedulerState == "notindb") {
                //u.ctx.strokeStyle = "rgb(0, 0, 0)";
                u.ctx.fillStyle = "rgba(0, 0, 0, 0.5)";
              } else { // Fallback: All other DEFINED states
                //u.ctx.strokeStyle = "rgb(255, 0, 0)";
                u.ctx.fillStyle = "rgba(255, 0, 0, 0.5)";
              }
            }
          } else {
            // No Colors: Use Black
            u.ctx.strokeStyle = "rgb(0, 0, 0)";
            u.ctx.fillStyle = "rgba(0, 0, 0, 0.5)";
          }

          // Get Values
          let xVal = d[0][i];
          let yVal = d[1][i];

          // Calc Size; Alt.: size = sizes[i] * pxRatio
          let size = 1;

          // Jobs: Size based on Resourcecount
          if (jobsData) {
            const scaling = jobsData[i].numNodes > 12
              ? 24 // Capped Dot Size 
              : jobsData[i].numNodes > 1
                ? jobsData[i].numNodes * 2 // MultiNode Scaling
                : jobsData[i]?.numAcc ? jobsData[i].numAcc : jobsData[i].numNodes * 2 // Single Node or Scale by Accs
            size = sizeBase + scaling
          // Nodes: Size based on Jobcount
          } else if (nodesData) {
            size = sizeBase + (nodesData[i]?.numJobs * 1.5) // Max Jobs Scale: 8 * 1.5 = 12
          };
          
          if (xVal >= filtLft && xVal <= filtRgt && yVal >= filtBtm && yVal <= filtTop) {
            let cx = valToPosX(xVal, scaleX, xDim, xOff);
            let cy = valToPosY(yVal, scaleY, yDim, yOff);

            u.ctx.moveTo(cx + size/2, cy);
            u.ctx.beginPath();
            u.ctx.arc(cx, cy, size/2, 0, deg360);
            u.ctx.fill();
            u.ctx.stroke();

            each(u, seriesIdx, i,
              cx - size/2 - strokeWidth/2,
              cy - size/2 - strokeWidth/2,
              size + strokeWidth,
              size + strokeWidth
            );
          }
        }
        u.ctx.restore();
      });
      return null;
    };
  };

  let drawPoints = makeDrawPoints({
    // disp: {
    //   size: {
    //     // unit: 3, // raw CSS pixels
    //     //	discr: true,
    //     values: (u, seriesIdx, idx0, idx1) => {
    //       /* Func to get sizes from additional subSeries [series][2...x] ([0,1] is [x,y]) */
    //       // TODO: only run once per setData() call
    //       let [minValue, maxValue] = getSizeMinMax(u);
    //       return u.data[seriesIdx][2].map(v => getSize(v, minValue, maxValue));
    //     },
    //   },
    // },
    transparentFill: true,
    each: (u, seriesIdx, dataIdx, lft, top, wid, hgt) => {
      // we get back raw canvas coords (included axes & padding). translate to the plotting area origin
      lft -= u.bbox.left;
      top -= u.bbox.top;
      qt.add({x: lft, y: top, w: wid, h: hgt, sidx: seriesIdx, didx: dataIdx});
    },
  });

  const legendValues = (u, seriesIdx, dataIdx) => {
    // when data null, it's initial schema probe (also u.status == 0)
    if (u.data == null || dataIdx == null || hRect == null || hRect.sidx != seriesIdx) {
      return {
        "Intensity [FLOPS/Byte]": '-',
        "":'',
        "Performace [GFLOPS]": '-'
      };
    }

    return {
      "Intensity [FLOPS/Byte]": roundTwoDigits(u.data[seriesIdx][0][dataIdx]),
      "":'',
      "Performace [GFLOPS]": roundTwoDigits(u.data[seriesIdx][1][dataIdx]),
    };
  };

  // Tooltip Plugin
  function tooltipPlugin({onclick, getLegendData, shiftX = 10, shiftY = 10}) {
    let tooltipLeftOffset = 0;
    let tooltipTopOffset = 0;

    const tooltip = document.createElement("div");

    // Build Manual Class By Styles
    tooltip.style.fontSize = "10pt";
    tooltip.style.position = "absolute";
    tooltip.style.background = "#fcfcfc";
    tooltip.style.display = "none";
    tooltip.style.border = "2px solid black";
    tooltip.style.padding = "4px";
    tooltip.style.pointerEvents = "none";
    tooltip.style.zIndex = "100";
    tooltip.style.whiteSpace = "pre";
    tooltip.style.fontFamily = "monospace";

    const tipSeriesIdx = 1; // Scatter: Series IDX is always 1
    let tipDataIdx = null;

    // const fmtDate = uPlot.fmtDate("{M}/{D}/{YY} {h}:{mm}:{ss} {AA}");
    let over;
    let tooltipVisible = false;

    function showTooltip() {
      if (!tooltipVisible) {
        tooltip.style.display = "block";
        over.style.cursor = "pointer";
        tooltipVisible = true;
      }
    }

    function hideTooltip() {
      if (tooltipVisible) {
        tooltip.style.display = "none";
        over.style.cursor = null;
        tooltipVisible = false;
      }
    }

    function setTooltip(u, i) {
      showTooltip();

      let top = u.valToPos(u.data[tipSeriesIdx][1][i], 'y');
      let lft = u.valToPos(u.data[tipSeriesIdx][0][i], 'x');

      tooltip.style.top  = (tooltipTopOffset  + top + shiftX) + "px";
      tooltip.style.left = (tooltipLeftOffset + lft + shiftY) + "px";

      if (useColors) {
        // Jobs: Color based on Duration
        if (jobsData) {
          tooltip.style.borderColor = getRGB(u.data[2][i]);
        // Nodes: Color based on Idle vs. Allocated
        } else if (nodesData) {
          if (nodesData[i]?.schedulerState == "idle") {
            tooltip.style.borderColor = "rgb(0, 0, 255)";
          } else if (nodesData[i]?.schedulerState == "allocated") {
            tooltip.style.borderColor = "rgb(0, 255, 0)";
          } else if (nodesData[i]?.schedulerState == "notindb") { // Missing from DB table
            tooltip.style.borderColor = "rgb(0, 0, 0)";
          } else { // Fallback: All other DEFINED states
            tooltip.style.borderColor = "rgb(255, 0, 0)";
          }
        }
      } else {
        // No Colors: Use Black
        tooltip.style.borderColor = "rgb(0, 0, 0)";
      }

      if (jobsData) {
        tooltip.textContent = (
          // Tooltip Content as String for Job
          `Job ID: ${getLegendData(u, i).jobId}\nRuntime: ${getLegendData(u, i).duration}\nNodes: ${getLegendData(u, i).numNodes}${getLegendData(u, i)?.numAcc?`\nAccelerators: ${getLegendData(u, i).numAcc}`:''}`
        );
      } else if (nodesData && useColors) {
        tooltip.textContent = (
          // Tooltip Content as String for Node
          `Host: ${getLegendData(u, i).nodeName}\nState: ${getLegendData(u, i).schedulerState}\nJobs: ${getLegendData(u, i).numJobs}`
        );
      } else if (nodesData && !useColors) {
        tooltip.textContent = (
          // Tooltip Content as String for Node
          `Host: ${getLegendData(u, i).nodeName}\nJobs: ${getLegendData(u, i).numJobs}`
        );
      }
    }

    return {
      hooks: {
        ready: [
          u => {
            over = u.over;
            tooltipLeftOffset = parseFloat(over.style.left);
            tooltipTopOffset = parseFloat(over.style.top);
            u.root.querySelector(".u-wrap").appendChild(tooltip);

            let clientX;
            let clientY;

            over.addEventListener("mousedown", e => {
              clientX = e.clientX;
              clientY = e.clientY;
            });

            over.addEventListener("mouseup", e => {
              // clicked in-place
              if (e.clientX == clientX && e.clientY == clientY) {
                if (tipDataIdx != null) {
                  onclick(u, tipDataIdx);
                }
              }
            });
          }
        ],
        setCursor: [
          u => {
            let i = u.legend.idxs[1];
            if (i != null) {
              tipDataIdx = i;
              setTooltip(u, i);
            } else {
              tipDataIdx = null;
              hideTooltip();
            }
          }
        ]
      }
    };
  }

  // Main Functions
  function sizeChanged() {
    if (timeoutId != null) clearTimeout(timeoutId);
    timeoutId = setTimeout(() => {
      timeoutId = null;
      if (uplot) uplot.destroy();
      render(roofData, jobsData, nodesData);
    }, 200);
  }

  function render(roofData, jobsData, nodesData) {
    let plotTitle = "CPU Roofline Diagram";
    if (jobsData) plotTitle = "Job Average Roofline Diagram";
    if (nodesData) plotTitle = "Node Average Roofline Diagram";

    if (roofData) {
      const opts = {
        title: plotTitle,
        mode: 2,
        width: width,
        height: height,
        legend: {
          show: useLegend,
        },
        cursor: { 
          dataIdx: (u, seriesIdx) => {
            if (seriesIdx == 1) {
              hRect = null;

              let dist = Infinity;
              let area = Infinity;
              let cx = u.cursor.left * pxRatio;
              let cy = u.cursor.top * pxRatio;

              qt.get(cx, cy, 1, 1, o => {
                if (pointWithin(cx, cy, o.x, o.y, o.x + o.w, o.y + o.h)) {
                  let ocx = o.x + o.w / 2;
                  let ocy = o.y + o.h / 2;

                  let dx = ocx - cx;
                  let dy = ocy - cy;

                  let d = Math.sqrt(dx ** 2 + dy ** 2);

                  // test against radius for actual hover
                  if (d <= o.w / 2) {
                    let a = o.w * o.h;

                    // prefer smallest
                    if (a < area) {
                      area = a;
                      dist = d;
                      hRect = o;
                    }
                    // only hover bbox with closest distance
                    else if (a == area && d <= dist) {
                      dist = d;
                      hRect = o;
                    }
                  }
                }
              });
            }
            return hRect && seriesIdx == hRect.sidx ? hRect.didx : null;
          },
          /* Render "Fill" on Data Point Hover: Works in Example Bubble, does not work here? Guess: Interference with tooltip */
          // points: {
          //   size: (u, seriesIdx) => {
          //     return hRect && seriesIdx == hRect.sidx ? hRect.w / pxRatio : 0;
          //   }
          // },
          /* Make all non-focused series semi-transparent: Useless unless more than one series rendered */
          // focus: {
          //   prox: 1e3,
          //   alpha: 0.3,
          //   dist: (u, seriesIdx) => {
          //     let prox = (hRect?.sidx === seriesIdx ? 0 : Infinity);
          //     return prox;
          //   },
          // },
          drag: { // Activates Zoom: Only one Dimension; YX Breaks Zoom Reset (Reason TBD)
            x: true,
            y: false
          },
        },
        axes: [
          {
            label: "Intensity [FLOPS/Byte]",
            values: (u, vals) => vals.map((v) => formatNumber(v)),
          },
          {
            label: "Performace [GFLOPS]",
            values: (u, vals) => vals.map((v) => formatNumber(v)),
          },
        ],
        scales: {
          x: {
            time: false,
            range: [0.01, 1000],
            distr: 3, // Render as log
            log: 10, // log exp
          },
          y: {
            range: [
              0.01,
              subCluster?.flopRateSimd?.value
                ? nearestThousand(subCluster.flopRateSimd.value)
                : 10000,
            ],
            distr: 3, // Render as log
            log: 10, // log exp
          },
        },
        series: [
          null,
          {
            /* Facets: Define Purpose of Sub-Arrays in Series-Array, e.g. x, y, size, label, color, ... */
            // facets: [
            //   {
            //     scale: 'x',
            //     auto: true,
            //   },
            //   {
            //     scale: 'y',
            //     auto: true,
            //   }
            // ],
            paths: drawPoints,
            values: legendValues
          }
        ],
        hooks: {
          // setSeries: [ (u, seriesIdx) => console.log('setSeries', seriesIdx) ],
          // setLegend: [ u => console.log('setLegend', u.legend.idxs) ],
          drawClear: [
            (u) => {
              qt = qt || new Quadtree(0, 0, u.bbox.width, u.bbox.height);
              qt.clear();

              // force-clear the path cache to cause drawBars() to rebuild new quadtree
              u.series.forEach((s, i) => {
                if (i > 0) 
                  s._paths = null;
              });
            },
          ],
          draw: [
            (u) => {
              // draw roofs when subCluster set
              if (subCluster != null) {
                const padding = u._padding; // [top, right, bottom, left]

                u.ctx.strokeStyle = "black";
                u.ctx.lineWidth = lineWidth;
                u.ctx.beginPath();

                const ycut = 0.01 * subCluster.memoryBandwidth.value;
                const scalarKnee =
                  (subCluster.flopRateScalar.value - ycut) /
                  subCluster.memoryBandwidth.value;
                const simdKnee =
                  (subCluster.flopRateSimd.value - ycut) /
                  subCluster.memoryBandwidth.value;
                const scalarKneeX = u.valToPos(scalarKnee, "x", true), // Value, axis, toCanvasPixels
                  simdKneeX = u.valToPos(simdKnee, "x", true),
                  flopRateScalarY = u.valToPos(
                    subCluster.flopRateScalar.value,
                    "y",
                    true,
                  ),
                  flopRateSimdY = u.valToPos(
                    subCluster.flopRateSimd.value,
                    "y",
                    true,
                  );

                if (
                  scalarKneeX <
                  width * window.devicePixelRatio -
                    padding[1] * window.devicePixelRatio
                ) {
                  // Lower horizontal roofline
                  u.ctx.moveTo(scalarKneeX, flopRateScalarY);
                  u.ctx.lineTo(
                    width * window.devicePixelRatio -
                      padding[1] * window.devicePixelRatio,
                    flopRateScalarY,
                  );
                }

                if (
                  simdKneeX <
                  width * window.devicePixelRatio -
                    padding[1] * window.devicePixelRatio
                ) {
                  // Top horitontal roofline
                  u.ctx.moveTo(simdKneeX, flopRateSimdY);
                  u.ctx.lineTo(
                    width * window.devicePixelRatio -
                      padding[1] * window.devicePixelRatio,
                    flopRateSimdY,
                  );
                }

                let x1 = u.valToPos(0.01, "x", true),
                  y1 = u.valToPos(ycut, "y", true);

                let x2 = u.valToPos(simdKnee, "x", true),
                  y2 = flopRateSimdY;

                let xAxisIntersect = lineIntersect(
                  x1,
                  y1,
                  x2,
                  y2,
                  u.valToPos(0.01, "x", true),
                  u.valToPos(1.0, "y", true), // X-Axis Start Coords
                  u.valToPos(1000, "x", true),
                  u.valToPos(1.0, "y", true), // X-Axis End Coords
                );

                if (xAxisIntersect.x > x1) {
                  x1 = xAxisIntersect.x;
                  y1 = xAxisIntersect.y;
                }

                // Diagonal
                u.ctx.moveTo(x1, y1);
                u.ctx.lineTo(x2, y2);

                u.ctx.stroke();
                // Reset grid lineWidth
                u.ctx.lineWidth = 0.15;
              }

              /* Render Scales */
              if (useColors) {
                // Jobs: The Color Scale For Time Information
                if (jobsData) {
                  const posX = u.valToPos(0.1, "x", true)
                  const posXLimit = u.valToPos(100, "x", true)
                  const posY = u.valToPos(17500.0, "y", true)
                  u.ctx.fillStyle = 'black'
                  u.ctx.fillText('0 Hours', posX, posY)
                  const start = posX + 10
                  for (let x = start; x < posXLimit; x += 10) {
                    let c = (x - start) / (posXLimit - start)
                    u.ctx.fillStyle = getRGB(c)
                    u.ctx.beginPath()
                    u.ctx.arc(x, posY, 3, 0, Math.PI * 2, false)
                    u.ctx.fill()
                  }
                  u.ctx.fillStyle = 'black'
                  u.ctx.fillText('24 Hours', posXLimit + 55, posY)
                }

                // Nodes: The Colors Of NodeStates
                if (nodesData) {
                  const posY = u.valToPos(17500.0, "y", true)

                  const posAllocDot = u.valToPos(0.03, "x", true)
                  const posAllocText = posAllocDot + 60
                  const posIdleDot = u.valToPos(0.3, "x", true)
                  const posIdleText = posIdleDot + 30
                  const posOtherDot = u.valToPos(3, "x", true)
                  const posOtherText = posOtherDot + 40
                  const posMissingDot = u.valToPos(30, "x", true)
                  const posMissingText = posMissingDot + 80

                  u.ctx.fillStyle = "rgb(0, 255, 0)"
                  u.ctx.beginPath()
                  u.ctx.arc(posAllocDot, posY, 3, 0, Math.PI * 2, false)
                  u.ctx.fill()
                  u.ctx.fillStyle = 'black'
                  u.ctx.fillText('Allocated', posAllocText, posY)

                  u.ctx.fillStyle = "rgb(0, 0, 255)"
                  u.ctx.beginPath()
                  u.ctx.arc(posIdleDot, posY, 3, 0, Math.PI * 2, false)
                  u.ctx.fill()
                  u.ctx.fillStyle = 'black'
                  u.ctx.fillText('Idle', posIdleText, posY)

                  u.ctx.fillStyle = "rgb(255, 0, 0)"
                  u.ctx.beginPath()
                  u.ctx.arc(posOtherDot, posY, 3, 0, Math.PI * 2, false)
                  u.ctx.fill()
                  u.ctx.fillStyle = 'black'
                  u.ctx.fillText('Other', posOtherText, posY)

                  u.ctx.fillStyle = 'black'
                  u.ctx.beginPath()
                  u.ctx.arc(posMissingDot, posY, 3, 0, Math.PI * 2, false)
                  u.ctx.fill()
                  u.ctx.fillText('Missing in DB', posMissingText, posY)
                }
              }
            },
          ],
        },
        plugins: [
          tooltipPlugin({
            onclick(u, dataIdx) {
              if (jobsData) {
                window.open(`/monitoring/job/${jobsData[dataIdx].id}`)
              } else if (nodesData) {
                window.open(`/monitoring/node/${cluster}/${nodesData[dataIdx].nodeName}`)
              }
            },
            getLegendData: (u, dataIdx) => {
              if (jobsData) {
                return jobsData[dataIdx]
              } else if (nodesData) {
                return nodesData[dataIdx]
              }
            }
          }),
        ],
      };
      uplot = new uPlot(opts, roofData, plotWrapper);
    } else {
      // console.log("No data for roofline!");
    }
  }

  /* On Mount */
  onMount(() => {
    render(roofData, jobsData, nodesData);
  });

  /* On Destroy */
  onDestroy(() => {
    if (uplot) uplot.destroy();
    if (timeoutId != null) clearTimeout(timeoutId);
  });
</script>

{#if roofData != null}
  <div bind:this={plotWrapper} class="p-2"></div>
{:else}
  <Card class="mx-4" body color="warning">Cannot render roofline: No data!</Card>
{/if}
