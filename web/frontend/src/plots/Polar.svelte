<script>
    import { getContext } from 'svelte'
    import { Radar } from 'svelte-chartjs';
    import {
        Chart as ChartJS,
        Title,
        Tooltip,
        Legend,
        Filler,
        PointElement,
        RadialLinearScale,
        LineElement
    } from 'chart.js';

    ChartJS.register(
        Title,
        Tooltip,
        Legend,
        Filler,
        PointElement,
        RadialLinearScale,
        LineElement
    );

    export let size
    export let metrics
    export let cluster
    export let jobMetrics

    const metricConfig = getContext('metrics')

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

    const data = {
        labels: labels,
        datasets: [
            {
                label: 'Max',
                data: getValuesForStat(getMax),
                fill: 1,
                backgroundColor: 'rgba(0, 102, 255, 0.25)',
                borderColor: 'rgb(0, 102, 255)',
                pointBackgroundColor: 'rgb(0, 102, 255)',
                pointBorderColor: '#fff',
                pointHoverBackgroundColor: '#fff',
                pointHoverBorderColor: 'rgb(0, 102, 255)'
            },
            {
                label: 'Avg',
                data: getValuesForStat(getAvg),
                fill: true,
                backgroundColor: 'rgba(255, 153, 0, 0.25)',
                borderColor: 'rgb(255, 153, 0)',
                pointBackgroundColor: 'rgb(255, 153, 0)',
                pointBorderColor: '#fff',
                pointHoverBackgroundColor: '#fff',
                pointHoverBorderColor: 'rgb(255, 153, 0)'
            }
        ]
    }

    // No custom defined options but keep for clarity 
    const options = {
        maintainAspectRatio: false,
        animation: false
    }

</script>

<div class="chart-container">
    <Radar {data} {options} width={size} height={size}/>
</div>

<style>
    .chart-container {
        margin: auto;
        position: relative;
    }
</style>