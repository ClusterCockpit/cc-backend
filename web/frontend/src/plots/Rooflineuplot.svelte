<script>
    import uPlot from 'uplot'
    import { formatNumber } from '../units.js'
    import { onMount, onDestroy } from 'svelte'
    import { Card } from 'sveltestrap'

    export let flopsAny = null
    export let memBw = null
    export let cluster = null
    export let defaultMaxY = null
    export let width = 500
    export let height = 300
    export let tiles = null
    export let colorDots = true
    export let showTime = true
    export let data = null

    let plotWrapper = null
    let uplot = null
    let timeoutId = null

    const paddingLeft = 40,
          paddingRight = 10,
          paddingTop = 10,
          paddingBottom = 50

    // Three Render-Cases:
    // #1 Single-Job Roofline -> Has Time-Information: Use data, allow colorDots && showTime
    // #2 MultiNode Roofline - > Has No Time-Information: Transform from nodeData, only "IST"-state of nodes, no timeInfo
    // #3 Multi-Job Roofline -> No Time Information? -> Use Backend-Prepared "Tiles" with increasing occupancy for stronger values

    // Start Demo Data

    function randInt(min, max) {
        return Math.floor(Math.random() * (max - min + 1)) + min;
    }

    function randFloat(min, max) {
        return roundTwo(((Math.random() * (max - min + 1)) + min) / randInt(1, 500));
    }

    function roundTwo(num) {
       return  Math.round((num + Number.EPSILON) * 100) / 100
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

    let points = 1000;

    data       = [null, [], []] // Null-Axis required for scatter
    data[1][0] = filledArr(points, i => randFloat(1,5000)) // Intensity
    data[1][1] = filledArr(points, i => randFloat(1,5000)) // Performance
    data[2]    = filledArr(points, i => 0) // Time Information (Optional)

    console.log("Subcluster: ", cluster);
    console.log("Data: ", data);

    // End Demo Data

    // Helpers

    const [minX, maxX, minY, maxY] = [0.01, 1000, 1., cluster?.flopRateSimd?.value || defaultMaxY]

    const w = width - paddingLeft - paddingRight

    const h = height - paddingTop - paddingBottom

    const [log10minX, log10maxX, log10minY, log10maxY] =
            [Math.log10(minX), Math.log10(maxX), Math.log10(minY), Math.log10(maxY)]

    const getCanvasX = (x) => {
        x = Math.log10(x)
        x -= log10minX; x /= (log10maxX - log10minX)
        return Math.round((x * w) + paddingLeft)
    }

    const getCanvasY = (y) => {
        y = Math.log10(y)
        y -= log10minY
        y /= (log10maxY - log10minY)
        return Math.round((h - y * h) + paddingTop)
    }

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

    function lineIntersect(x1, y1, x2, y2, x3, y3, x4, y4) {
        let l = (y4 - y3) * (x2 - x1) - (x4 - x3) * (y2 - y1)
        let a = ((x4 - x3) * (y1 - y3) - (y4 - y3) * (x1 - x3)) / l
        return {
            x: x1 + a * (x2 - x1),
            y: y1 + a * (y2 - y1)
        }
    }

    function transformData(flopsAny, memBw, colorDots) { // Uses Metric Object
        const nodes = flopsAny.series.length
        const timesteps = flopsAny.series[0].data.length

        /* c will contain values from 0 to 1 representing the time */
        const x = [], y = [], c = []

        if (flopsAny && memBw) {
            for (let i = 0; i < nodes; i++) {
                const flopsData = flopsAny.series[i].data
                const memBwData = memBw.series[i].data
                for (let j = 0; j < timesteps; j++) {
                    const f = flopsData[j], m = memBwData[j]
                    const intensity = f / m
                    if (Number.isNaN(intensity) || !Number.isFinite(intensity))
                        continue

                    x.push(intensity)
                    y.push(f)
                    c.push(colorDots ? j / timesteps : 0)
                }
            }
        } else {
            console.warn("transformData: metrics for 'mem_bw' and/or 'flops_any' missing!")
        }

        return {
            x, y, c,
            xLabel: 'Intensity [FLOPS/byte]',
            yLabel: 'Performance [GFLOPS]'
        }
    }

    // Return something to be plotted. The argument shall be the result of the
    // `nodeMetrics` GraphQL query.
    export function transformPerNodeData(nodes) {
        const x = [], y = [], c = []
        for (let node of nodes) {
            let flopsAny = node.metrics.find(m => m.name == 'flops_any' && m.scope == 'node')?.metric
            let memBw    = node.metrics.find(m => m.name == 'mem_bw'    && m.scope == 'node')?.metric
            if (!flopsAny || !memBw) {
                console.warn("transformPerNodeData: metrics for 'mem_bw' and/or 'flops_any' missing!")
                continue
            }

            let flopsData = flopsAny.series[0].data, memBwData = memBw.series[0].data
            const f = flopsData[flopsData.length - 1], m = memBwData[flopsData.length - 1]
            const intensity = f / m
            if (Number.isNaN(intensity) || !Number.isFinite(intensity))
                continue

            x.push(intensity)
            y.push(f)
            c.push(0)
        }

        return {
            x, y, c,
            xLabel: 'Intensity [FLOPS/byte]',
            yLabel: 'Performance [GFLOPS]'
        }
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

    function render() {
        const opts = {
            title: "",
            mode: 2,
            width: width,
            height: height,
            legend: {
                show: false
            },
            axes: [
                {
                    label: 'Intensity [FLOPS/Byte]'
                },
                {
                    label: 'Performace [GFLOPS]'
                }
            ],
            scales: {
                x: {
                    time: false,
                    distr: 3,
					log: 10,
                },
                y: {
                    distr: 3,
					log: 10,
                },
            },
            series: [
                {},
                {
                    stroke: (u, seriesIdx) => {
                        for (let i = 0; i < points; ++i) { return getRGB(data[2][i]) }
					},
                    fill: (u, seriesIdx) => {
                        for (let i = 0; i < points; ++i) { return getRGB(data[2][i]) }
					},
                    paths: drawPoints,
                }
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
                        u => { // draw roofs
                        u.ctx.strokeStyle = 'black'
                        u.ctx.lineWidth = 2
                        u.ctx.beginPath()
                        if (cluster != null) {
                            const ycut = 0.01 * cluster.memoryBandwidth.value
                            const scalarKnee = (cluster.flopRateScalar.value - ycut) / cluster.memoryBandwidth.value
                            const simdKnee = (cluster.flopRateSimd.value - ycut) / cluster.memoryBandwidth.value
                            const scalarKneeX = getCanvasX(scalarKnee),
                                simdKneeX = getCanvasX(simdKnee),
                                flopRateScalarY = getCanvasY(cluster.flopRateScalar.value),
                                flopRateSimdY = getCanvasY(cluster.flopRateSimd.value)

                            if (scalarKneeX < width - paddingRight) {
                                u.ctx.moveTo(scalarKneeX, flopRateScalarY)
                                u.ctx.lineTo(width - paddingRight, flopRateScalarY)
                            }

                            if (simdKneeX < width - paddingRight) {
                                u.ctx.moveTo(simdKneeX, flopRateSimdY)
                                u.ctx.lineTo(width - paddingRight, flopRateSimdY)
                            }

                            let x1 = getCanvasX(0.01),
                                y1 = getCanvasY(ycut),
                                x2 = getCanvasX(simdKnee),
                                y2 = flopRateSimdY

                            let xAxisIntersect = lineIntersect(
                                x1, y1, x2, y2,
                                0, height - paddingBottom, width, height - paddingBottom)

                            if (xAxisIntersect.x > x1) {
                                x1 = xAxisIntersect.x
                                y1 = xAxisIntersect.y
                            }

                            u.ctx.moveTo(x1, y1)
                            u.ctx.lineTo(x2, y2)
                        }
                        u.ctx.stroke()
                    }
                ]
            },
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