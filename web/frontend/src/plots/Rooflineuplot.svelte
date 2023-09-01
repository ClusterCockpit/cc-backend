<script>
    import uPlot from 'uplot'
    import { formatNumber } from '../units.js'
    import { onMount, onDestroy } from 'svelte'
    import { Card } from 'sveltestrap'

    let plotWrapper = null
    let uplot = null
    let timeoutId = null

    function randInt(min, max) {
        return Math.floor(Math.random() * (max - min + 1)) + min;
    }

    function filledArr(len, val) {
        let arr = Array(len);

        if (typeof val == "function") {
            for (let i = 0; i < len; ++i)
                arr[i] = val(i);
        }
        else {
            for (let i = 0; i < len; ++i)
                arr[i] = val;
        }

        return arr;
    }

    let points = 10000;
    let series = 5;

    console.time("prep");

    let data = filledArr(series, v => [
        filledArr(points, i => randInt(0,500)),
        filledArr(points, i => randInt(0,500)),
    ]);

    data[0] = null;

    console.timeEnd("prep");

    console.log(data);

    const drawPoints = (u, seriesIdx, idx0, idx1) => {
        const size = 5 * devicePixelRatio;

        uPlot.orient(u, seriesIdx, (series, dataX, dataY, scaleX, scaleY, valToPosX, valToPosY, xOff, yOff, xDim, yDim, moveTo, lineTo, rect, arc) => {
            let d = u.data[seriesIdx];

            u.ctx.fillStyle = series.stroke();

            let deg360 = 2 * Math.PI;

            console.time("points");

            let p = new Path2D();

            for (let i = 0; i < d[0].length; i++) {
                let xVal = d[0][i];
                let yVal = d[1][i];

                if (xVal >= scaleX.min && xVal <= scaleX.max && yVal >= scaleY.min && yVal <= scaleY.max) {
                    let cx = valToPosX(xVal, scaleX, xDim, xOff);
                    let cy = valToPosY(yVal, scaleY, yDim, yOff);

                    p.moveTo(cx + size/2, cy);
                    arc(p, cx, cy, size/2, 0, deg360);
                }
            }

            console.timeEnd("points1");

            u.ctx.fill(p);
        });

        return null;
    };

    let pxRatio;

    function setPxRatio() {
        pxRatio = devicePixelRatio;
    }

    function guardedRange(u, min, max) {
        if (max == min) {
            if (min == null) {
                min = 0;
                max = 100;
            }
            else {
                let delta = Math.abs(max) || 100;
                max += delta;
                min -= delta;
            }
        }

        return [min, max];
    }

    setPxRatio();

    window.addEventListener('dppxchange', setPxRatio);

    const opts = {
        title: "Scatter Plot",
        mode: 2,
        width: 1920,
        height: 600,
        legend: {
            live: false,
        },
        hooks: {
            drawClear: [
                u => {
                    u.series.forEach((s, i) => {
                        if (i > 0)
                            s._paths = null;
                    });
                },
            ],
        },
        scales: {
            x: {
                time: false,
            //	auto: false,
            //	range: [0, 500],
                // remove any scale padding, use raw data limits
                range: guardedRange,
            },
            y: {
            //	auto: false,
            //	range: [0, 500],
                // remove any scale padding, use raw data limits
                range: guardedRange,
            },
        },
        series: [
            {
                /*
                stroke: "red",
                fill: "rgba(255,0,0,0.1)",
                paths: (u, seriesIdx, idx0, idx1) => {
                    uPlot.orient(u, seriesIdx, (series, dataX, dataY, scaleX, scaleY, valToPosX, valToPosY, xOff, yOff, xDim, yDim) => {
                        let d = u.data[seriesIdx];

                        console.log(d);
                    });

                    return null;
                },
                */
            },
            {
                stroke: "red",
                fill: "rgba(255,0,0,0.1)",
                paths: drawPoints,
            },
            {
                stroke: "green",
                fill: "rgba(0,255,0,0.1)",
                paths: drawPoints,
            },
            {
                stroke: "blue",
                fill: "rgba(0,0,255,0.1)",
                paths: drawPoints,
            },
            {
                stroke: "magenta",
                fill: "rgba(0,0,255,0.1)",
                paths: drawPoints,
            },
        ],
    };

    let u = new uPlot(opts, data, document.body);

</script>

{#if data != null}
    <div bind:this={plotWrapper}/>
{:else}
    <Card class="mx-4" body color="warning">Cannot render roofline: No data!</Card>
{/if}