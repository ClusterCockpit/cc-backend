<div>
    <canvas bind:this={canvasElement} width="{width}" height="{height}"></canvas>
</div>

<script>
    import { onMount, getContext } from 'svelte'

    export let metrics
    export let width
    export let height
    export let cluster
    export let jobMetrics

    const fontSize = 12
    const fontFamily = 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"'
    const metricConfig = getContext('metrics')

    let ctx, canvasElement

    const labels = metrics.filter(name => {
        if (!jobMetrics.find(m => m.name == name && m.scope == "node")) {
            console.warn(`PolarPlot: No metric data for '${name}'`)
            return false
        }
        return true
    })

    const getValuesForStat = (getStat) => labels.map(name => {
        const peak = metricConfig(cluster, name).peak
        const metric = jobMetrics.find(m => m.name == name && m.scope == "node")
        const value = getStat(metric.metric) / peak
        return value <= 1. ? value : 1.
    })

    function getMax(metric) {
        let max = 0
        for (let series of metric.series)
            max = Math.max(max, series.statistics.max)
        return max
    }

    function getAvg(metric) {
        let avg = 0
        for (let series of metric.series)
            avg += series.statistics.avg
        return avg / metric.series.length
    }

    const data = [
        {
            name: 'Max',
            values: getValuesForStat(getMax),
            color: 'rgb(0, 102, 255)',
            areaColor: 'rgba(0, 102, 255, 0.25)'
        },
        {
            name: 'Avg',
            values: getValuesForStat(getAvg),
            color: 'rgb(255, 153, 0)',
            areaColor: 'rgba(255, 153, 0, 0.25)'
        }
    ]

    function render() {
        if (!width || Number.isNaN(width))
            return

        const centerX = width / 2
        const centerY = height / 2 - 15
        const radius = (Math.min(width, height) / 2) - 50

        // Draw circles
        ctx.lineWidth = 1
        ctx.strokeStyle = '#999999'
        ctx.beginPath()
        ctx.arc(centerX, centerY, radius * 1.0, 0, Math.PI * 2, false)
        ctx.stroke()
        ctx.beginPath()
        ctx.arc(centerX, centerY, radius * 0.666, 0, Math.PI * 2, false)
        ctx.stroke()
        ctx.beginPath()
        ctx.arc(centerX, centerY, radius * 0.333, 0, Math.PI * 2, false)
        ctx.stroke()

        // Axis
        ctx.font = `${fontSize}px ${fontFamily}`
        ctx.textAlign = 'center'
        ctx.fillText('1/3',
            Math.floor(centerX + radius * 0.333),
            Math.floor(centerY + 15))
        ctx.fillText('2/3',
            Math.floor(centerX + radius * 0.666),
            Math.floor(centerY + 15))
        ctx.fillText('1.0',
            Math.floor(centerX + radius * 1.0),
            Math.floor(centerY + 15))

        // Label text and straight lines from center
        for (let i = 0; i < labels.length; i++) {
            const angle = 2 * Math.PI * ((i + 1) / labels.length)
            const dx = Math.cos(angle) * radius
            const dy = Math.sin(angle) * radius
            ctx.fillText(labels[i],
                Math.floor(centerX + dx * 1.1),
                Math.floor(centerY + dy * 1.1))

            ctx.beginPath()
            ctx.moveTo(centerX, centerY)
            ctx.lineTo(centerX + dx, centerY + dy)
            ctx.stroke()
        }

        for (let dataset of data) {
            console.assert(dataset.values.length === labels.length, 'this will look confusing')
            ctx.fillStyle = dataset.color
            ctx.strokeStyle = dataset.color
            const points = []
            for (let i = 0; i < dataset.values.length; i++) {
                const value = dataset.values[i]
                const angle = 2 * Math.PI * ((i + 1) / labels.length)
                const x = centerX + Math.cos(angle) * radius * value
                const y = centerY + Math.sin(angle) * radius * value

                ctx.beginPath()
                ctx.arc(x, y, 3, 0, Math.PI * 2, false)
                ctx.fill()

                points.push({ x, y })
            }

            // "Fill" the shape this dataset has
            ctx.fillStyle = dataset.areaColor
            ctx.beginPath()
            ctx.moveTo(points[0].x, points[0].y)
            for (let p of points)
                ctx.lineTo(p.x, p.y)
            ctx.lineTo(points[0].x, points[0].y)
            ctx.stroke()
            ctx.fill()
        }

        // Legend at the bottom left corner
        ctx.textAlign = 'left'
        let paddingLeft = 0
        for (let dataset of data) {
            const text = `${dataset.name}: `
            const textWidth = ctx.measureText(text).width
            ctx.fillStyle = 'black'
            ctx.fillText(text, paddingLeft, height - 20)

            ctx.fillStyle = dataset.color
            ctx.beginPath()
            ctx.arc(paddingLeft + textWidth + 5, height - 25, 5, 0, Math.PI * 2, false)
            ctx.fill()

            paddingLeft += textWidth + 15
        }
        ctx.fillStyle = 'black'
        ctx.fillText(`Values relative to respective peak.`, 0, height - 7)
    }

    let mounted = false
    onMount(() => {
        canvasElement.width = width
        canvasElement.height = height
        ctx = canvasElement.getContext('2d')
        render(ctx, data, width, height)
        mounted = true
    })

    let timeoutId = null
    function sizeChanged() {
        if (!mounted)
            return;

        if (timeoutId != null)
            clearTimeout(timeoutId)

        timeoutId = setTimeout(() => {
            timeoutId = null

            canvasElement.width = width
            canvasElement.height = height
            ctx = canvasElement.getContext('2d')
            render(ctx, data, width, height)
        }, 250)
    }

    $: sizeChanged(width, height)
</script>
