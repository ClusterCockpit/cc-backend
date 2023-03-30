<!--
    @component
    Properties:
    - width, height: Number
    - min, max: Number
    - label: (x-Value) => String
    - data: [{ value: Number, count: Number }]
 -->

<div
    on:mousemove={mousemove}
    on:mouseleave={() => (infoText = '')}>
    <span style="left: {paddingLeft + 5}px;">{infoText}</span>
    <canvas bind:this={canvasElement} width="{width}" height="{height}"></canvas>
</div>

<script>
    import { onMount } from 'svelte'

    export let data
    export let width = 500
    export let height = 300
    export let xlabel = ''
    export let ylabel = ''
    export let min = null
    export let max = null
    export let label = formatNumber

    const fontSize = 12
    const fontFamily = 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"'
    const paddingLeft = 35, paddingRight = 20, paddingTop = 20, paddingBottom = 20

    let ctx, canvasElement

    const maxCount = data.reduce((max, point) => Math.max(max, point.count), 0),
          maxValue = data.reduce((max, point) => Math.max(max, point.value), 0.1)

    function getStepSize(valueRange, pixelRange, minSpace) {
        const proposition = valueRange / (pixelRange / minSpace)
        const getStepSize = n => Math.pow(10, Math.floor(n / 3)) *
            (n < 0 ? [1., 5., 2.][-n % 3] : [1., 2., 5.][n % 3])

        let n = 0
        let stepsize = getStepSize(n)
        while (true) {
            let bigger = getStepSize(n + 1)
            if (proposition > bigger) {
                n += 1
                stepsize = bigger
            } else {
                return stepsize
            }
        }
    }

    let infoText = ''
    function mousemove(event) {
        let rect = event.target.getBoundingClientRect()
        let x = event.clientX - rect.left
        if (x < paddingLeft || x > width - paddingRight) {
            infoText = ''
            return
        }

        const w = width - paddingLeft - paddingRight
        const barWidth = Math.round(w / (maxValue + 1))
        x = Math.floor((x - paddingLeft) / (w - barWidth) * maxValue)
        let point = data.find(point => point.value == x)

        if (point)
            infoText = `count: ${point.count} (value: ${label(x)})`
        else
            infoText = ''
    }

    function render() {
        const labelOffset =  Math.floor(height * 0.1)
        const h = height - paddingTop - paddingBottom - labelOffset
        const w = width - paddingLeft - paddingRight
        const barGap = 5
        const barWidth = Math.ceil(w / (maxValue + 1)) - barGap

        if (Number.isNaN(barWidth))
            return

        const getCanvasX = (value) => (value / maxValue) * (w - barWidth) + paddingLeft + (barWidth / 2.)
        const getCanvasY = (count) => (h - (count / maxCount) * h) + paddingTop

        // X Axis
        ctx.font = `bold ${fontSize}px ${fontFamily}`
        ctx.fillStyle = 'black'
        if (xlabel != '') {
            let textWidth = ctx.measureText(xlabel).width
            ctx.fillText(xlabel, Math.floor((width / 2) - (textWidth / 2) + barGap), height - Math.floor(labelOffset / 2))
        }
        ctx.textAlign = 'center'
        ctx.font = `${fontSize}px ${fontFamily}`
        if (min != null && max != null) {
            const stepsizeX = getStepSize(max - min, w, 75)
            let startX = 0
            while (startX < min)
                startX += stepsizeX

            for (let x = startX; x < max; x += stepsizeX) {
                let px = ((x - min) / (max - min)) * (w - barWidth) + paddingLeft + (barWidth / 2.)
                ctx.fillText(`${formatNumber(x)}`, px, height - paddingBottom - Math.floor(labelOffset / 2))
            }
        } else {
            const stepsizeX = getStepSize(maxValue, w, 120)
            for (let x = 0; x <= maxValue; x += stepsizeX) {
                ctx.fillText(label(x), getCanvasX(x), height - paddingBottom - Math.floor(labelOffset / 2))
            }
        }

        // Y Axis
        ctx.fillStyle = 'black'
        ctx.strokeStyle = '#bbbbbb'
        ctx.font = `bold ${fontSize}px ${fontFamily}`
        if (ylabel != '') {
            ctx.save()
            ctx.translate(15, Math.floor(h / 2))
            ctx.rotate(-Math.PI / 2)
            ctx.fillText(ylabel, 0, 0)
            ctx.restore()
        }
        ctx.textAlign = 'right'
        ctx.font = `${fontSize}px ${fontFamily}`
        ctx.beginPath()
        const stepsizeY = getStepSize(maxCount, h, 50)
        for (let y = stepsizeY; y <= maxCount; y += stepsizeY) {
            const py = Math.floor(getCanvasY(y))
            ctx.fillText(`${formatNumber(y)}`, paddingLeft - 5, py)
            ctx.moveTo(paddingLeft, py)
            ctx.lineTo(width, py)
        }
        ctx.stroke()

        // Draw bars
        ctx.fillStyle = '#85abce'
        for (let p of data) {
            ctx.fillRect(
                getCanvasX(p.value) - (barWidth / 2.),
                getCanvasY(p.count),
                barWidth,
                (p.count / maxCount) * h)
        }

        // Fat lines left and below plotting area
        ctx.strokeStyle = 'black'
        ctx.beginPath()
        ctx.moveTo(0, height - paddingBottom - labelOffset)
        ctx.lineTo(width, height - paddingBottom - labelOffset)
        ctx.moveTo(paddingLeft, 0)
        ctx.lineTo(paddingLeft, height - Math.floor(labelOffset / 2))
        ctx.stroke()
    }

    let mounted = false
    onMount(() => {
        mounted = true
        canvasElement.width = width
        canvasElement.height = height
        ctx = canvasElement.getContext('2d')
        render()
    })

    let timeoutId = null;
    function sizeChanged() {
        if (timeoutId != null)
            clearTimeout(timeoutId)

        timeoutId = setTimeout(() => {
            timeoutId = null
            if (!canvasElement)
                return

            canvasElement.width = width
            canvasElement.height = height
            ctx = canvasElement.getContext('2d')
            render()
        }, 250)
    }

    $: sizeChanged(width, height)
</script>

<style>
    div {
        position: relative;
    }
    div > span {
        position: absolute;
        top: 0px;
    }
</style>

<script context="module">
    import { formatNumber } from '../utils.js'

    export function binsFromFootprint(weights, values, numBins) {
        let min = 0, max = 0
        if (values.length != 0) {
            for (let x of values) {
                min = Math.min(min, x)
                max = Math.max(max, x)
            }
            max += 1 // So that we have an exclusive range.
        }

        if (numBins == null || numBins < 3)
            numBins = 3

        const bins = new Array(numBins).fill(0)
        for (let i = 0; i < values.length; i++)
            bins[Math.floor(((values[i] - min) / (max - min)) * numBins)] += weights ? weights[i] : 1

        return {
            label: idx => {
                let start = min + (idx / numBins) * (max - min)
                let stop = min + ((idx + 1) / numBins) * (max - min)
                return `${formatNumber(start)} - ${formatNumber(stop)}`
            },
            bins: bins.map((count, idx) => ({ value: idx, count: count })),
            min: min,
            max: max
        }
    }
</script>
