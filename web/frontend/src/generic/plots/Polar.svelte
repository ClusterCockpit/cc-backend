<!--
    @component Polar Plot based on chartJS Radar

    Properties:
    - `polarMetrics [Object]?`: Metric names and scaled peak values for rendering polar plot [Default: [] ]
    - `jobMetrics [GraphQL.JobMetricWithName]?`: Metric data [Default: null]
    - `height Number?`: Plot height [Default: 365]
 -->

<script>
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

    export let polarMetrics = [];
    export let jobMetrics = null;
    export let height = 350;

    const labels = polarMetrics
        .filter((m) => (m.peak != null))
        .map(pm => pm.name)
        .sort(function (a, b) {return ((a > b) ? 1 : ((b > a) ? -1 : 0))});

    function loadData(type) {
        if (!labels) {
            console.warn("Empty 'metrics' array prop! Cannot render Polar.")
            return []
        } else {
            if (type === 'avg') {
                return getValues(getAvg)
            } else if (type === 'max') {
                return getValues(getMax)
            } else if (type === 'min') {
                return getValues(getMin)
            }
            console.log('Unknown Type For Polar Data')
            return []
        }
    }

    // Helpers

    const getValues = (getStat) => labels.map(name => {
        // Peak is adapted and scaled for job shared state
        const peak = polarMetrics.find(m => m.name == name).peak
        const metric = jobMetrics.find(m => m.name == name && m.scope == "node")
        const value = getStat(metric.metric) / peak
        return value <= 1. ? value : 1.
    })

    function getMax(metric) {
        let max = metric.series[0].statistics.max;
        for (let series of metric.series)
            max = Math.max(max, series.statistics.max)
        return max
    }

    function getMin(metric) {
        let min = metric.series[0].statistics.min;
        for (let series of metric.series)
            min = Math.min(min, series.statistics.min)
        return min
    }

    function getAvg(metric) {
        let avg = 0;
        for (let series of metric.series)
            avg += series.statistics.avg
        return avg / metric.series.length
    }

    // Chart JS Objects

    const data = {
        labels: labels,
        datasets: [
            {
                label: 'Max',
                data: loadData('max'), // Node Scope Only
                fill: 1,
                backgroundColor: 'rgba(0, 0, 255, 0.25)',
                borderColor: 'rgb(0, 0, 255)',
                pointBackgroundColor: 'rgb(0, 0, 255)',
                pointBorderColor: '#fff',
                pointHoverBackgroundColor: '#fff',
                pointHoverBorderColor: 'rgb(0, 0, 255)'
            },
            {
                label: 'Avg',
                data: loadData('avg'), // Node Scope Only
                fill: 2,
                backgroundColor: 'rgba(255, 210, 0, 0.25)',
                borderColor: 'rgb(255, 210, 0)',
                pointBackgroundColor: 'rgb(255, 210, 0)',
                pointBorderColor: '#fff',
                pointHoverBackgroundColor: '#fff',
                pointHoverBorderColor: 'rgb(255, 210, 0)'
            },
            {
                label: 'Min',
                data: loadData('min'), // Node Scope Only
                fill: true,
                backgroundColor: 'rgba(255, 0, 0, 0.25)',
                borderColor: 'rgb(255, 0, 0)',
                pointBackgroundColor: 'rgb(255, 0, 0)',
                pointBorderColor: '#fff',
                pointHoverBackgroundColor: '#fff',
                pointHoverBorderColor: 'rgb(255, 0, 0)'
            }
        ]
    }

    // No custom defined options but keep for clarity 
    const options = {
        maintainAspectRatio: true,
        animation: false,
        scales: { // fix scale
            r: {
                suggestedMin: 0.0,
                suggestedMax: 1.0
            }
        }
    }

</script>

<div class="chart-container">
    <Radar {data} {options} {height}/>
</div>

<style>
    .chart-container {
        margin: auto;
        position: relative;
    }
</style>