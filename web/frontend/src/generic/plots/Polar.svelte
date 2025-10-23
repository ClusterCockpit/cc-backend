<!--
    @component Polar Plot based on chartJS Radar

    Properties:
    - `polarMetrics [Object]?`: Metric names and scaled peak values for rendering polar plot [Default: [] ]
    - `polarData [GraphQL.JobMetricStatWithName]?`: Metric data [Default: null]
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
    export let polarData = [];
    export let height = 350;

    const labels = polarMetrics
        .filter((m) => (m.peak != null))
        .map(pm => pm.name)
        .sort(function (a, b) {return ((a > b) ? 1 : ((b > a) ? -1 : 0))});

    function loadData(type) {
        if (labels && (type == 'avg' || type == 'min' ||type == 'max')) {
            return getValues(type)
        } else if (!labels) {
            console.warn("Empty 'polarMetrics' array prop! Cannot render Polar representation.")
        } else {
            console.warn('Unknown Type For Polar Data (must be one of [min, max, avg])')
        }
        return []
    }

    // Helper

    const getValues = (type) => labels.map(name => {
        // Peak is adapted and scaled for job shared state
        const peak = polarMetrics.find(m => m?.name == name)?.peak
        const metric = polarData.find(m => m?.name == name)?.stats
        const value = (peak && metric) ? (metric[type] / peak) : 0
        return value <= 1. ? value : 1.
    })

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