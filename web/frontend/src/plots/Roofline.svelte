<div class="cc-plot">
    <canvas bind:this={canvasElement} width="{prevWidth}" height="{prevHeight}"></canvas>
</div>

<script context="module">
    const axesColor = '#aaaaaa'
    const fontSize = 12
    const fontFamily = 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"'
    const paddingLeft = 40,
        paddingRight = 10,
        paddingTop = 10,
        paddingBottom = 50

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

    const power = [1, 1e3, 1e6, 1e9, 1e12]
    const suffix = ['', 'k', 'm', 'g']
    function formatNumber(x) {
        for (let i = 0; i < suffix.length; i++)
            if (power[i] <= x && x < power[i+1])
                return `${x / power[i]}${suffix[i]}`

        return Math.abs(x) >= 1000 ? x.toExponential() : x.toString()
    }

    function axisStepFactor(i, size) {
        if (size && size < 500)
            return 10

        if (i % 3 == 0)
            return 2
        else if (i % 3 == 1)
            return 2.5
        else
            return 2
    }

    function render(ctx, data, cluster, width, height, colorDots, showTime, defaultMaxY) {
        if (width <= 0)
            return

        const [minX, maxX, minY, maxY] = [0.01, 1000, 1., cluster?.flopRateSimd || defaultMaxY]
        const w = width - paddingLeft - paddingRight
        const h = height - paddingTop - paddingBottom

        // Helpers:
        const [log10minX, log10maxX, log10minY, log10maxY] =
            [Math.log10(minX), Math.log10(maxX), Math.log10(minY), Math.log10(maxY)]

        /* Value -> Pixel-Coordinate */
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

        // Axes
        ctx.fillStyle = 'black'
        ctx.strokeStyle = axesColor
        ctx.font = `${fontSize}px ${fontFamily}`
        ctx.beginPath()
        for (let x = minX, i = 0; x <= maxX; i++) {
            let px = getCanvasX(x)
            let text = formatNumber(x)
            let textWidth = ctx.measureText(text).width
            ctx.fillText(text,
                Math.floor(px - (textWidth / 2)),
                height - paddingBottom + fontSize + 5)
            ctx.moveTo(px, paddingTop - 5)
            ctx.lineTo(px, height - paddingBottom + 5)

            x *= axisStepFactor(i, w)
        }
        if (data.xLabel) {
            let textWidth = ctx.measureText(data.xLabel).width
            ctx.fillText(data.xLabel, Math.floor((width / 2) - (textWidth / 2)), height - 20)
        }

        ctx.textAlign = 'center'
        for (let y = minY, i = 0; y <= maxY; i++) {
            let py = getCanvasY(y)
            ctx.moveTo(paddingLeft - 5, py)
            ctx.lineTo(width - paddingRight + 5, py)

            ctx.save()
            ctx.translate(paddingLeft - 10, py)
            ctx.rotate(-Math.PI / 2)
            ctx.fillText(formatNumber(y), 0, 0)
            ctx.restore()

            y *= axisStepFactor(i)
        }
        if (data.yLabel) {
            ctx.save()
            ctx.translate(15, Math.floor(height / 2))
            ctx.rotate(-Math.PI / 2)
            ctx.fillText(data.yLabel, 0, 0)
            ctx.restore()
        }
        ctx.stroke()

        // Draw Data
        if (data.x && data.y) {
            for (let i = 0; i < data.x.length; i++) {
                let x = data.x[i], y = data.y[i], c = data.c[i]
                if (x == null || y == null || Number.isNaN(x) || Number.isNaN(y))
                    continue

                const s = 3
                const px = getCanvasX(x)
                const py = getCanvasY(y)

                ctx.fillStyle = getRGB(c)
                ctx.beginPath()
                ctx.arc(px, py, s, 0, Math.PI * 2, false)
                ctx.fill()
            }
        } else if (data.tiles) {
            const rows = data.tiles.length
            const cols = data.tiles[0].length

            const tileWidth = Math.ceil(w / cols)
            const tileHeight = Math.ceil(h / rows)

            let max = data.tiles.reduce((max, row) =>
                Math.max(max, row.reduce((max, val) =>
                    Math.max(max, val)), 0), 0)

            if (max == 0)
                max = 1

            const tileColor = val => `rgba(255, 0, 0, ${(val / max)})`

            for (let i = 0; i < rows; i++) {
                for (let j = 0; j < cols; j++) {
                    let px = paddingLeft + (j / cols) * w
                    let py = paddingTop + (h - (i / rows) * h) - tileHeight

                    ctx.fillStyle = tileColor(data.tiles[i][j])
                    ctx.fillRect(px, py, tileWidth, tileHeight)
                }
            }
        }

        // Draw roofs
        ctx.strokeStyle = 'black'
        ctx.lineWidth = 2
        ctx.beginPath()
        if (cluster != null) {
            const ycut = 0.01 * cluster.memoryBandwidth
            const scalarKnee = (cluster.flopRateScalar - ycut) / cluster.memoryBandwidth
            const simdKnee = (cluster.flopRateSimd - ycut) / cluster.memoryBandwidth
            const scalarKneeX = getCanvasX(scalarKnee),
                  simdKneeX = getCanvasX(simdKnee),
                  flopRateScalarY = getCanvasY(cluster.flopRateScalar),
                  flopRateSimdY = getCanvasY(cluster.flopRateSimd)

            if (scalarKneeX < width - paddingRight) {
                ctx.moveTo(scalarKneeX, flopRateScalarY)
                ctx.lineTo(width - paddingRight, flopRateScalarY)
            }

            if (simdKneeX < width - paddingRight) {
                ctx.moveTo(simdKneeX, flopRateSimdY)
                ctx.lineTo(width - paddingRight, flopRateSimdY)
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

            ctx.moveTo(x1, y1)
            ctx.lineTo(x2, y2)
        }
        ctx.stroke()

        if (colorDots && showTime &&  data.x && data.y) {
            // The Color Scale For Time Information
            ctx.fillStyle = 'black'
            ctx.fillText('Time:', 17, height - 5)
            const start = paddingLeft + 5
            for (let x = start; x < width - paddingRight; x += 15) {
                let c = (x - start) / (width - start - paddingRight)
                ctx.fillStyle = getRGB(c)
                ctx.beginPath()
                ctx.arc(x, height - 10, 5, 0, Math.PI * 2, false)
                ctx.fill()
            }
        }
    }

    function transformData(flopsAny, memBw, colorDots) {
        const nodes = flopsAny.series.length
        const timesteps = flopsAny.series[0].data.length

        /* c will contain values from 0 to 1 representing the time */
        const x = [], y = [], c = []
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
            let flopsAny = node.metrics.find(m => m.name == 'flops_any' && m.metric.scope == 'node')?.metric
            let memBw    = node.metrics.find(m => m.name == 'mem_bw'    && m.metric.scope == 'node')?.metric
            if (!flopsAny || !memBw)
                continue

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
</script>

<script>
    import { onMount, tick } from 'svelte'

    export let flopsAny = null
    export let memBw = null
    export let cluster = null
    export let maxY = null
    export let width
    export let height
    export let tiles = null
    export let colorDots = true
    export let showTime = true
    export let data = null

    console.assert(data || tiles || (flopsAny && memBw), "you must provide flopsAny and memBw or tiles!")

    let ctx, canvasElement, prevWidth = width, prevHeight = height
    data = data != null ? data : (flopsAny && memBw
        ? transformData(flopsAny, memBw, colorDots)
        : {
            tiles: tiles,
            xLabel: 'Intensity [FLOPS/byte]',
            yLabel: 'Performance [GFLOPS]'
        })

    onMount(() => {
        ctx = canvasElement.getContext('2d')
        if (prevWidth != width || prevHeight != height) {
            sizeChanged()
            return
        }

        canvasElement.width = width
        canvasElement.height = height
        render(ctx, data, cluster, width, height, colorDots, showTime, maxY)
    })

    let timeoutId = null
    function sizeChanged() {
        if (!ctx)
            return

        if (timeoutId != null)
            clearTimeout(timeoutId)

        prevWidth = width
        prevHeight = height
        timeoutId = setTimeout(() => {
            if (!canvasElement)
                return

            timeoutId = null
            canvasElement.width = width
            canvasElement.height = height
            render(ctx, data, cluster, width, height, colorDots, showTime, maxY)
        }, 250)
    }

    $: sizeChanged(width, height)
</script>
