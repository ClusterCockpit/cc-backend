<!--
    @component

    Only width/height should change reactively.

    Properties:
    - width:            Number
    - height:           Number
    - timestep:         Number
    - series:           [GraphQL.Series]
    - statisticsSeries: [GraphQL.StatisticsSeries]
    - cluster:          GraphQL.Cluster
    - subCluster:       String
    - metric:           String
    - scope:            String
    - useStatsSeries:   Boolean

    Functions:
    - setTimeRange(from, to): Void

    // TODO: Move helper functions to module context?
 -->
<script>
    import uPlot from 'uplot'
    import { formatNumber } from '../units.js'
    import { getContext, onMount, onDestroy } from 'svelte'
    import { Card } from 'sveltestrap'

    export let metric
    export let scope = 'node'
    export let resources = []
    export let width
    export let height
    export let timestep
    export let series
    export let useStatsSeries = null
    export let statisticsSeries = null
    export let cluster
    export let subCluster
    export let isShared = false
    export let forNode = false

    if (useStatsSeries == null)
        useStatsSeries = statisticsSeries != null

    if (useStatsSeries == false && series == null)
        useStatsSeries = true

    const metricConfig = getContext('metrics')(cluster, metric)
    const clusterCockpitConfig = getContext('cc-config')
    const resizeSleepTime = 250
    const normalLineColor = '#000000'
    const lineWidth = clusterCockpitConfig.plot_general_lineWidth / window.devicePixelRatio
    const lineColors = clusterCockpitConfig.plot_general_colorscheme
    const backgroundColors = { normal:  'rgba(255, 255, 255, 1.0)', caution: 'rgba(255, 128, 0, 0.3)', alert: 'rgba(255, 0, 0, 0.3)' }
    const thresholds = findThresholds(metricConfig, scope, typeof subCluster == 'string' ? cluster.subClusters.find(sc => sc.name == subCluster) : subCluster)

    // converts the legend into a simple tooltip
    function legendAsTooltipPlugin({ className, style = { backgroundColor:"rgba(255, 249, 196, 0.92)", color: "black" } } = {}) {
        let legendEl;
        const dataSize = series.length

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
                ...style
            });

            // conditional hide series color markers:
            if (useStatsSeries === true || // Min/Max/Avg Self-Explanatory
                dataSize === 1          || // Only one Y-Dataseries
                dataSize > 6            ){ // More than 6 Y-Dataseries
                const idents = legendEl.querySelectorAll(".u-marker");
                for (let i = 0; i < idents.length; i++)
                    idents[i].style.display = "none";
            }

            const overEl = u.over;
            overEl.style.overflow = "visible";

            // move legend into plot bounds
            overEl.appendChild(legendEl);

            // show/hide tooltip on enter/exit
            overEl.addEventListener("mouseenter", () => {legendEl.style.display = null;});
            overEl.addEventListener("mouseleave", () => {legendEl.style.display = "none";});

            // let tooltip exit plot
            // overEl.style.overflow = "visible";
        }

        function update(u) {
            const { left, top } = u.cursor;
            const width = u.over.querySelector(".u-legend").offsetWidth;
            legendEl.style.transform = "translate(" + (left - width - 15) + "px, " + (top + 15) + "px)";
        }

        if (dataSize <= 12 || useStatsSeries === true) {
            return {
                hooks: {
                    init: init,
                    setCursor: update,
                }
            }
        } else { // Setting legend-opts show/live as object with false here will not work ...
            return {}
        }
    }

    function backgroundColor() {
        if (clusterCockpitConfig.plot_general_colorBackground == false
            || !thresholds
            || !(series && series.every(s => s.statistics != null)))
            return backgroundColors.normal

        let cond = thresholds.alert < thresholds.caution
            ? (a, b) => a <= b
            : (a, b) => a >= b

        let avg = series.reduce((sum, series) => sum + series.statistics.avg, 0) / series.length

        if (Number.isNaN(avg))
            return backgroundColors.normal

        if (cond(avg, thresholds.alert))
            return backgroundColors.alert

        if (cond(avg, thresholds.caution))
            return backgroundColors.caution

        return backgroundColors.normal
    }

    function lineColor(i, n) {
        if (n >= lineColors.length)
            return lineColors[i % lineColors.length];
        else
            return lineColors[Math.floor((i / n) * lineColors.length)];
    }

    const longestSeries = useStatsSeries
        ? statisticsSeries.mean.length
        : series.reduce((n, series) => Math.max(n, series.data.length), 0)
    const maxX = longestSeries * timestep
    let maxY = null
    
    if (thresholds !== null) {
        maxY = useStatsSeries
            ? (statisticsSeries.max.reduce((max, x) => Math.max(max, x), thresholds.normal) || thresholds.normal)
            : (series.reduce((max, series) => Math.max(max, series.statistics?.max), thresholds.normal) || thresholds.normal)

        if (maxY >= (10 * thresholds.normal)) { // Hard y-range render limit if outliers in series data
            maxY = (10 * thresholds.normal)
        } 
    }

    const plotSeries = [{label: 'Runtime', value: (u, ts, sidx, didx) => didx == null ? null : formatTime(ts, forNode)}]
    const plotData = [new Array(longestSeries)]

    if (forNode === true) {
        // Negative Timestamp Buildup
        for (let i = 0; i <= longestSeries; i++) {
            plotData[0][i] = (longestSeries - i) * timestep * -1
        }
    } else {
        // Positive Timestamp Buildup
        for (let j = 0; j < longestSeries; j++) // TODO: Cache/Reuse this array?
            plotData[0][j] = j * timestep
    }

    let plotBands = undefined
    if (useStatsSeries) {
        plotData.push(statisticsSeries.min)
        plotData.push(statisticsSeries.max)
        plotData.push(statisticsSeries.mean)

        if (forNode === true) { // timestamp 0 with null value for reversed time axis
            if (plotData[1].length != 0) plotData[1].push(null)
            if (plotData[2].length != 0) plotData[2].push(null)
            if (plotData[3].length != 0) plotData[3].push(null)
        }

        plotSeries.push({ label: 'min', scale: 'y', width: lineWidth, stroke: 'red' })
        plotSeries.push({ label: 'max', scale: 'y', width: lineWidth, stroke: 'green' })
        plotSeries.push({ label: 'mean', scale: 'y', width: lineWidth, stroke: 'black' })
      
        plotBands = [
            { series: [2,3], fill: 'rgba(0,255,0,0.1)' },
            { series: [3,1], fill: 'rgba(255,0,0,0.1)' }
        ];
    } else {
        for (let i = 0; i < series.length; i++) {
            plotData.push(series[i].data)
            if (forNode === true && plotData[1].length != 0) plotData[1].push(null) // timestamp 0 with null value for reversed time axis
            plotSeries.push({
                label: scope === 'node' ? resources[i].hostname : 
                       // scope === 'accelerator' ? resources[0].accelerators[i] : 
                       scope + ' #' + (i+1),
                scale: 'y',
                width: lineWidth,
                stroke: lineColor(i, series.length)
            })
        }
    }

    const opts = {
        width,
        height,
        plugins: [
				legendAsTooltipPlugin()
		],
        series: plotSeries,
        axes: [
            {
                scale: 'x',
                space: 35,
                incrs: timeIncrs(timestep, maxX, forNode),
                values: (_, vals) => vals.map(v => formatTime(v, forNode))
            },
            {
                scale: 'y',
                grid: { show: true },
                labelFont: 'sans-serif',
                values: (u, vals) => vals.map(v => formatNumber(v))
            }
        ],
        bands: plotBands,
        padding: [5, 10, -20, 0],
        hooks: {
            draw: [(u) => {
                // Draw plot type label:
                let textl = `${scope}${plotSeries.length > 2 ? 's' : ''}${
                    useStatsSeries ? ': min/avg/max' : (metricConfig != null && scope != metricConfig.scope ? ` (${metricConfig.aggregation})` : '')}`
                let textr = `${(isShared && (scope != 'core' && scope != 'accelerator')) ? '[Shared]' : '' }`
                u.ctx.save()
                u.ctx.textAlign = 'start' // 'end'
                u.ctx.fillStyle = 'black'
                u.ctx.fillText(textl, u.bbox.left + 10, u.bbox.top + 10)
                u.ctx.textAlign = 'end'
                u.ctx.fillStyle = 'black'
                u.ctx.fillText(textr, u.bbox.left + u.bbox.width - 10, u.bbox.top + 10)
                // u.ctx.fillText(text, u.bbox.left + u.bbox.width - 10, u.bbox.top + u.bbox.height - 10) // Recipe for bottom right

                if (!thresholds) {
                    u.ctx.restore()
                    return
                }

                let y = u.valToPos(thresholds.normal, 'y', true)
                u.ctx.save()
                u.ctx.lineWidth = lineWidth
                u.ctx.strokeStyle = normalLineColor
                u.ctx.setLineDash([5, 5])
                u.ctx.beginPath()
                u.ctx.moveTo(u.bbox.left, y)
                u.ctx.lineTo(u.bbox.left + u.bbox.width, y)
                u.ctx.stroke()
                u.ctx.restore()
            }]
        },
        scales: {
            x: { time: false },
            y: maxY ? { range: [0., maxY * 1.1] } : {}
        },
        legend : { // Display legend until max 12 Y-dataseries
            show: (series.length <= 12 || useStatsSeries === true) ? true : false,
            live: (series.length <= 12 || useStatsSeries === true) ? true : false
        },
        cursor: { drag: { x: true, y: true } }
    }

    // console.log(opts)

    let plotWrapper = null
    let uplot = null
    let timeoutId = null
    let prevWidth = null, prevHeight = null

    function render() {
        if (!width || Number.isNaN(width) || width < 0)
            return

        if (prevWidth != null && Math.abs(prevWidth - width) < 10)
            return

        prevWidth = width
        prevHeight = height

        if (!uplot) {
            opts.width = width
            opts.height = height
            uplot = new uPlot(opts, plotData, plotWrapper)
        } else {
            uplot.setSize({ width, height })
        }
    }

    function onSizeChange() {
        if (!uplot)
            return

        if (timeoutId != null)
            clearTimeout(timeoutId)

        timeoutId = setTimeout(() => {
            timeoutId = null
            render()
        }, resizeSleepTime)
    }

    $: if (series[0].data.length > 0) {
         onSizeChange(width, height)
    }

    onMount(() => {
        if (series[0].data.length > 0) {
            plotWrapper.style.backgroundColor = backgroundColor()
            render()
        }
    })

    onDestroy(() => {
        if (uplot)
            uplot.destroy()

        if (timeoutId != null)
            clearTimeout(timeoutId)
    })

    // `from` and `to` must be numbers between 0 and 1.
    export function setTimeRange(from, to) {
        if (!uplot || from > to)
            return false

        uplot.setScale('x', { min: from * maxX, max: to * maxX })
        return true
    }
</script>
<script context="module">
    export function formatTime(t, forNode = false) {
        if (t !== null) {
            if (isNaN(t)) {
                return t
            } else {
                const tAbs = Math.abs(t)
                const h = Math.floor(tAbs / 3600)
                const m = Math.floor((tAbs % 3600) / 60)
                // Re-Add "negativity" to time ticks only as string, so that if-cases work as intended
                if (h == 0)
                    return `${forNode && m != 0 ? '-' : ''}${m}m`
                else if (m == 0)
                    return `${forNode?'-':''}${h}h`
                else
                    return `${forNode?'-':''}${h}:${m}h`
            }
        }
    }

    export function timeIncrs(timestep, maxX, forNode) {
        if (forNode === true) {
            return [60, 300, 900, 1800, 3600, 7200, 14400, 21600] // forNode fixed increments
        } else {
            let incrs = []
            for (let t = timestep; t < maxX; t *= 10)
                incrs.push(t, t * 2, t * 3, t * 5)

            return incrs
        }
    }

    export function findThresholds(metricConfig, scope, subCluster) {
        // console.log('NAME ' + metricConfig.name + ' / SCOPE ' + scope + ' / SUBCLUSTER ' + subCluster.name)
        if (!metricConfig || !scope || !subCluster) {
            console.warn('Argument missing for findThresholds!')
            return null
        }

        if (scope == 'node' || metricConfig.aggregation == 'avg') {
            if (metricConfig.subClusters && metricConfig.subClusters.length === 0) {
                // console.log('subClusterConfigs array empty, use metricConfig defaults')
                return { normal: metricConfig.normal, caution: metricConfig.caution, alert: metricConfig.alert }
            } else if (metricConfig.subClusters && metricConfig.subClusters.length > 0) {
                // console.log('subClusterConfigs found, use subCluster Settings if matching jobs subcluster:')
                let forSubCluster = metricConfig.subClusters.find(sc => sc.name == subCluster.name)
                if (forSubCluster && forSubCluster.normal && forSubCluster.caution && forSubCluster.alert) return forSubCluster
                else return { normal: metricConfig.normal, caution: metricConfig.caution, alert: metricConfig.alert }
            } else {
                console.warn('metricConfig.subClusters not found!')
                return null
            }
        }

        if (metricConfig.aggregation != 'sum') {
            console.warn('Missing or unkown aggregation mode (sum/avg) for metric:', metricConfig)
            return null
        }

        let divisor = 1
        if (scope == 'socket')
            divisor = subCluster.topology.socket.length
        else if (scope == 'core')
            divisor = subCluster.topology.core.length
        else if (scope == 'accelerator')
            divisor = subCluster.topology.accelerators.length
        else if (scope == 'hwthread')
            divisor = subCluster.topology.node.length
        else {
            // console.log('TODO: how to calc thresholds for ', scope)
            return null
        }

        let mc = metricConfig?.subClusters?.find(sc => sc.name == subCluster.name) || metricConfig
        return {
            normal: mc.normal / divisor,
            caution: mc.caution / divisor,
            alert: mc.alert / divisor
        }
    }

</script>

{#if series[0].data.length > 0}
    <div bind:this={plotWrapper} class="cc-plot"></div>
{:else}
    <Card class="mx-4" body color="warning">Cannot render plot: No series data returned for <code>{metric}</code></Card>
{/if}

<style>
    .cc-plot {
        border-radius: 5px;
    }
</style>
