<div class="cc-plot">
    <canvas bind:this={canvasElement} width="{prevWidth}" height="{prevHeight}"></canvas>
</div>

<script context="module">
    const axesColor = '#aaaaaa'
    const tickFontSize = 10
    const labelFontSize = 12
    const fontFamily = 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"'
    const paddingLeft = 40,
        paddingRight = 10,
        paddingTop = 10,
        paddingBottom = 50

    function lineIntersect(x1, y1, x2, y2, x3, y3, x4, y4) {
        let l = (y4 - y3) * (x2 - x1) - (x4 - x3) * (y2 - y1)
        let a = ((x4 - x3) * (y1 - y3) - (y4 - y3) * (x1 - x3)) / l
        return {
            x: x1 + a * (x2 - x1),
            y: y1 + a * (y2 - y1)
        }
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

    function render(ctx, data, cluster, width, height, defaultMaxY) {
        if (width <= 0)
            return

        const [minX, maxX, minY, maxY] = [0.01, 1000, 1., cluster?.flopRateSimd?.value || defaultMaxY]
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
        ctx.font = `${tickFontSize}px ${fontFamily}`
        ctx.beginPath()
        for (let x = minX, i = 0; x <= maxX; i++) {
            let px = getCanvasX(x)
            let text = formatNumber(x)
            let textWidth = ctx.measureText(text).width
            ctx.fillText(text,
                Math.floor(px - (textWidth / 2)),
                height - paddingBottom + tickFontSize + 5)
            ctx.moveTo(px, paddingTop - 5)
            ctx.lineTo(px, height - paddingBottom + 5)

            x *= axisStepFactor(i, w)
        }
        if (data.xLabel) {
            ctx.font = `${labelFontSize}px ${fontFamily}`
            let textWidth = ctx.measureText(data.xLabel).width
            ctx.fillText(data.xLabel, Math.floor((width / 2) - (textWidth / 2)), height - 20)
        }

        ctx.textAlign = 'center'
        ctx.font = `${tickFontSize}px ${fontFamily}`
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
            ctx.font = `${labelFontSize}px ${fontFamily}`
            ctx.save()
            ctx.translate(15, Math.floor(height / 2))
            ctx.rotate(-Math.PI / 2)
            ctx.fillText(data.yLabel, 0, 0)
            ctx.restore()
        }
        ctx.stroke()

        // Draw Data
        if (data.tiles) {
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
            const ycut = 0.01 * cluster.memoryBandwidth.value
            const scalarKnee = (cluster.flopRateScalar.value - ycut) / cluster.memoryBandwidth.value
            const simdKnee = (cluster.flopRateSimd.value - ycut) / cluster.memoryBandwidth.value
            const scalarKneeX = getCanvasX(scalarKnee),
                  simdKneeX = getCanvasX(simdKnee),
                  flopRateScalarY = getCanvasY(cluster.flopRateScalar.value),
                  flopRateSimdY = getCanvasY(cluster.flopRateSimd.value)

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
    }
</script>

<script>
    import { onMount } from 'svelte'
    import { formatNumber } from '../units.js'

    export let cluster  = null
    export let tiles    = null
    export let maxY     = null
    export let width    = 500
    export let height   = 300

    console.assert(tiles, "you must provide tiles!")

    let ctx, canvasElement, prevWidth = width, prevHeight = height
    const data = {
        tiles: tiles,
        xLabel: 'Intensity [FLOPS/byte]',
        yLabel: 'Performance [GFLOPS]'
    }

    onMount(() => {
        ctx = canvasElement.getContext('2d')
        if (prevWidth != width || prevHeight != height) {
            sizeChanged()
            return
        }

        canvasElement.width = width
        canvasElement.height = height
        render(ctx, data, cluster, width, height, maxY)
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
            render(ctx, data, cluster, width, height, maxY)
        }, 250)
    }

    $: sizeChanged(width, height)
</script>
