<script>
    import uPlot from 'uplot'
    import { formatNumber } from '../units.js'
    import { onMount, onDestroy } from 'svelte'
    import { Card } from 'sveltestrap'

    export let flopsAny = null
    export let memBw = null
    export let cluster = null
    export let maxY = null
    export let width = 500
    export let height = 300
    export let tiles = null
    export let colorDots = true
    export let showTime = true
    export let data = null

    let plotWrapper = null
    let uplot = null
    let timeoutId = null

    // Three Render-Cases:
    // #1 Single-Job Roofline -> Has Time-Information: Use data, allow colorDots && showTime
    // #2 MultiNode Roofline - > Has No Time-Information: Transform from nodeData, only "IST"-state of nodes, no timeInfo
    // #3 Multi-Job Roofline -> No Time Information? -> Use Backend-Prepared "Tiles" with increasing occupancy for stronger values

    // Start Demo Data

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

    let points = 100;
    let series = 2;
    let time = []

    for (let i = 0; i < points; ++i)
                time[i] = i;

    data = filledArr(series, v => [
        filledArr(points, i => randInt(0,200)),
        filledArr(points, i => randInt(0,200)),
    ]);

    data[0] = null;

    console.log("Data: ", data);

    // End Demo Data

    // Helpers

    function getGradientR(x) {
        if (x < 0.5) return 0
        if (x > 0.75) return 255
        x = (x - 0.5) * 4.0
        return Math.floor(x * 255.0)
    }

    function getGradientG(x) {
        if (x > 0.25 && x < 0.75) return 255
        if (x < 0.25) x = x * 4.0
        else          x = 1.0 - (x - 0.75) * 4.0
        return Math.floor(x * 255.0)
    }

    function getGradientB(x) {
        if (x < 0.25) return 255
        if (x > 0.5) return 0
        x = 1.0 - (x - 0.25) * 4.0
        return Math.floor(x * 255.0)
    }

    function getRGB(c) {
        return `rgb(${getGradientR(c)}, ${getGradientG(c)}, ${getGradientB(c)})`
    }

    // End Helpers

    const drawPoints = (u, seriesIdx, idx0, idx1) => {
        const size = 5 * devicePixelRatio;

        uPlot.orient(u, seriesIdx, (series, dataX, dataY, scaleX, scaleY, valToPosX, valToPosY, xOff, yOff, xDim, yDim, moveTo, lineTo, rect, arc) => {
            let d = u.data[seriesIdx];

            u.ctx.fillStyle = series.stroke();

            let deg360 = 2 * Math.PI;

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

            u.ctx.fill(p);
        });

        return null;
    };

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

    function render() {
        const opts = {
            title: "",
            mode: 2,
            width: width,
            height: height,
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
                    stroke: (u, seriesIdx) => {
                        for (let i = 0; i < points; ++i) { return getRGB(time[i]) }
					},
                    fill: (u, seriesIdx) => {
                        for (let i = 0; i < points; ++i) { return getRGB(time[i]) }
					},
                    paths: drawPoints,
                }
            ],
        };

        uplot = new uPlot(opts, data, plotWrapper);
    }

    // Copied from Histogram
    
    onMount(() => {
        render()
    })

    onDestroy(() => {
        if (uplot)
            uplot.destroy()

        if (timeoutId != null)
            clearTimeout(timeoutId)
    })

    function sizeChanged() {
        if (timeoutId != null)
            clearTimeout(timeoutId)

        timeoutId = setTimeout(() => {
            timeoutId = null
            if (uplot)
                uplot.destroy()

            render()
        }, 200)
    }

    $: sizeChanged(width, height)

</script>

{#if data != null}
    <div bind:this={plotWrapper}/>
{:else}
    <Card class="mx-4" body color="warning">Cannot render roofline: No data!</Card>
{/if}