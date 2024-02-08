<script>
    import uPlot from 'uplot'
    import { formatNumber } from '../units.js'
    import { onMount, onDestroy } from 'svelte'
    import { Card } from 'sveltestrap'

    export let data = null
    export let renderTime = false
    export let allowSizeChange = false
    export let cluster = null
    export let width = 600
    export let height = 350

    let plotWrapper = null
    let uplot = null
    let timeoutId = null

    const lineWidth = clusterCockpitConfig.plot_general_lineWidth

    /* Data Format
     * data       = [null, [], []] // 0: null-axis required for scatter, 1: Array of XY-Array for Scatter, 2: Optional Time Info
     * data[1][0] = [100, 200, 500, ...] // X Axis -> Intensity (Vals up to clusters' flopRateScalar value)
     * data[1][1] = [1000, 2000, 1500, ...] // Y Axis -> Performance (Vals up to clusters' flopRateSimd value)
     * data[2]    = [0.1, 0.15, 0.2, ...] // Color Code -> Time Information (Floats from 0 to 1) (Optional)
    */

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
    function nearestThousand (num) {
        return Math.ceil(num/1000) * 1000
    }
    function lineIntersect(x1, y1, x2, y2, x3, y3, x4, y4) {
        let l = (y4 - y3) * (x2 - x1) - (x4 - x3) * (y2 - y1)
        let a = ((x4 - x3) * (y1 - y3) - (y4 - y3) * (x1 - x3)) / l
        return {
            x: x1 + a * (x2 - x1),
            y: y1 + a * (y2 - y1)
        }
    }
    // End Helpers

    // Dot Renderers
    const drawColorPoints = (u, seriesIdx, idx0, idx1) => {
        const size = 5 * devicePixelRatio;
        uPlot.orient(u, seriesIdx, (series, dataX, dataY, scaleX, scaleY, valToPosX, valToPosY, xOff, yOff, xDim, yDim, moveTo, lineTo, rect, arc) => {
            let d = u.data[seriesIdx];
            let deg360 = 2 * Math.PI;
            for (let i = 0; i < d[0].length; i++) {
                let p    = new Path2D();
                let xVal = d[0][i];
                let yVal = d[1][i];
                u.ctx.strokeStyle = getRGB(u.data[2][i])
                u.ctx.fillStyle   = getRGB(u.data[2][i])
                if (xVal >= scaleX.min && xVal <= scaleX.max && yVal >= scaleY.min && yVal <= scaleY.max) {
                    let cx = valToPosX(xVal, scaleX, xDim, xOff);
                    let cy = valToPosY(yVal, scaleY, yDim, yOff);

                    p.moveTo(cx + size/2, cy);
                    arc(p, cx, cy, size/2, 0, deg360);
                }
                u.ctx.fill(p);
            }
        });
        return null;
    };

    const drawPoints = (u, seriesIdx, idx0, idx1) => {
        const size = 5 * devicePixelRatio;
        uPlot.orient(u, seriesIdx, (series, dataX, dataY, scaleX, scaleY, valToPosX, valToPosY, xOff, yOff, xDim, yDim, moveTo, lineTo, rect, arc) => {
            let d = u.data[seriesIdx];
            u.ctx.strokeStyle = getRGB(0);
            u.ctx.fillStyle   = getRGB(0);
            let deg360 = 2 * Math.PI;
            let p      = new Path2D();
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

    // Main Function
    function render(plotData) {
        if (plotData) {
            const opts = {
                title: "",
                mode: 2,
                width: width,
                height: height,
                legend: {
                    show: false
                },
                cursor: { drag: { x: false, y: false } },
                axes: [
                    { 
                        label: 'Intensity [FLOPS/Byte]',
                        values: (u, vals) => vals.map(v => formatNumber(v))
                    },
                    { 
                        label: 'Performace [GFLOPS]',
                        values: (u, vals) => vals.map(v => formatNumber(v))
                    }
                ],
                scales: {
                    x: {
                        time: false,
                        range: [0.01, 1000],
                        distr: 3, // Render as log
                        log: 10, // log exp
                    },
                    y: {
                        range: [1.0, cluster?.flopRateSimd?.value ? nearestThousand(cluster.flopRateSimd.value) : 10000],
                        distr: 3, // Render as log
                        log: 10, // log exp
                    },
                },
                series: [
                    {},
                    { paths: renderTime ? drawColorPoints : drawPoints }
                ],
                hooks: {
                    drawClear: [
                        u => {
                            u.series.forEach((s, i) => {
                                if (i > 0)
                                    s._paths = null;
                            });
                        },
                    ],
                    draw: [
                        u => { // draw roofs when cluster set
                            // console.log(u)
                            if (cluster != null) {
                                const padding = u._padding // [top, right, bottom, left]

                                u.ctx.strokeStyle = 'black'
                                u.ctx.lineWidth = lineWidth
                                u.ctx.beginPath()

                                const ycut = 0.01 * cluster.memoryBandwidth.value
                                const scalarKnee = (cluster.flopRateScalar.value - ycut) / cluster.memoryBandwidth.value
                                const simdKnee = (cluster.flopRateSimd.value - ycut) / cluster.memoryBandwidth.value
                                const scalarKneeX = u.valToPos(scalarKnee, 'x', true), // Value, axis, toCanvasPixels
                                    simdKneeX = u.valToPos(simdKnee, 'x', true),
                                    flopRateScalarY = u.valToPos(cluster.flopRateScalar.value, 'y', true),
                                    flopRateSimdY = u.valToPos(cluster.flopRateSimd.value, 'y', true)

                                // Debug get zoomLevel from browser
                                // console.log("Zoom", Math.round(window.devicePixelRatio * 100))

                                if (scalarKneeX < (width * window.devicePixelRatio) - (padding[1] * window.devicePixelRatio)) { // Lower horizontal roofline
                                    u.ctx.moveTo(scalarKneeX, flopRateScalarY)
                                    u.ctx.lineTo((width * window.devicePixelRatio) - (padding[1] * window.devicePixelRatio), flopRateScalarY)
                                }

                                if (simdKneeX < (width * window.devicePixelRatio) - (padding[1] * window.devicePixelRatio)) { // Top horitontal roofline
                                    u.ctx.moveTo(simdKneeX, flopRateSimdY)
                                    u.ctx.lineTo((width * window.devicePixelRatio) - (padding[1] * window.devicePixelRatio), flopRateSimdY)
                                }

                                let x1 = u.valToPos(0.01, 'x', true),
                                    y1 = u.valToPos(ycut, 'y', true)
                                    
                                let x2 = u.valToPos(simdKnee, 'x', true),
                                    y2 = flopRateSimdY

                                let xAxisIntersect = lineIntersect(
                                    x1, y1, x2, y2,
                                    u.valToPos(0.01, 'x', true), u.valToPos(1.0, 'y', true), // X-Axis Start Coords
                                    u.valToPos(1000, 'x', true), u.valToPos(1.0, 'y', true)  // X-Axis End Coords
                                )

                                if (xAxisIntersect.x > x1) {
                                    x1 = xAxisIntersect.x
                                    y1 = xAxisIntersect.y
                                }

                                // Diagonal
                                u.ctx.moveTo(x1, y1)
                                u.ctx.lineTo(x2, y2)

                                u.ctx.stroke()
                                // Reset grid lineWidth
                                u.ctx.lineWidth = 0.15
                            }
                        }
                    ]
                },
                // cursor: { drag: { x: true, y: true } } // Activate zoom
            };
            uplot = new uPlot(opts, plotData, plotWrapper);
        } else {
            console.log('No data for roofline!')
        }
    }

    // Svelte and Sizechange
    onMount(() => {
        render(data)
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
            render(data)
        }, 200)
    }
    $: if (allowSizeChange) sizeChanged(width, height)
</script>

{#if data != null}
    <div bind:this={plotWrapper}/>
{:else}
    <Card class="mx-4" body color="warning">Cannot render roofline: No data!</Card>
{/if}