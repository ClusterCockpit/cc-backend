<!--
    @component Polar Plot based on chart.js Radar

    Properties:
    - `footprintData [Object]?`: job.footprint content, evaluated in regards to peak config in jobSummary.svelte [Default: null]
    - `metrics [String]?`: Metric names to display as polar plot [Default: null]
    - `cluster GraphQL.Cluster?`: Cluster Object of the parent job [Default: null]
    - `subCluster GraphQL.SubCluster?`: SubCluster Object of the parent job [Default: null]
    - `jobMetrics [GraphQL.JobMetricWithName]?`: Metric data [Default: null]
    - `height Number?`: Plot height [Default: 365]
 -->

<script>
    import { getContext, onMount } from 'svelte'
    import Chart from 'chart.js/auto'

    export let canvasId;
    export let footprintData = null;
    export let metrics = null;
    export let cluster = null;
    export let subCluster = null;
    export let jobMetrics = null;
    export let height = 350;

    function getLabels() {
        if (footprintData) {
            return footprintData.filter(fpd => {
                if (!jobMetrics.find(m => m.name == fpd.name && m.scope == "node" || fpd.impact == 4)) {
                    console.warn(`PolarPlot: No metric data for '${fpd.name}'`)
                    return false
                }
                return true
            })
            .map(filtered => filtered.name)
            .sort(function (a, b) {
                return ((a > b) ? 1 : ((b > a) ? -1 : 0));
            });
        } else {
            return metrics.filter(name => {
                if (!jobMetrics.find(m => m.name == name && m.scope == "node")) {
                    console.warn(`PolarPlot: No metric data for '${name}'`)
                    return false
                }
                return true
            })
            .sort(function (a, b) {
                return ((a > b) ? 1 : ((b > a) ? -1 : 0));
            });
        }
    }

    const labels = getLabels();
    const getMetricConfig = getContext("getMetricConfig");

    const getValuesForStatGeneric = (getStat) => labels.map(name => {
        // TODO: Requires Scaling if Shared Job
        const peak = getMetricConfig(cluster, subCluster, name).peak
        const metric = jobMetrics.find(m => m.name == name && m.scope == "node")
        const value = getStat(metric.metric) / peak
        return value <= 1. ? value : 1.
    })

    const getValuesForStatFootprint = (getStat) => labels.map(name => {
        // FootprintData 'Peak' is pre-scaled for Shared Jobs in JobSummary Component
        const peak = footprintData.find(fpd => fpd.name === name).peak
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

    function loadDataGeneric(type) {
        if (type === 'avg') {
            return getValuesForStatGeneric(getAvg)
        } else if (type === 'max') {
            return getValuesForStatGeneric(getMax)
        } else if (type === 'min') {
            return getValuesForStatGeneric(getMin)
        }
        console.log('Unknown Type For Polar Data')
        return []
    }

    function loadDataForFootprint(type) {
        if (type === 'avg') {
            return getValuesForStatFootprint(getAvg)
        } else if (type === 'max') {
            return getValuesForStatFootprint(getMax)
        } else if (type === 'min') {
            return getValuesForStatFootprint(getMin)
        }
        console.log('Unknown Type For Polar Data')
        return []
    }

    const data = {
        labels: labels,
        datasets: [
            {
                label: 'Max',
                data: footprintData ? loadDataForFootprint('max') : loadDataGeneric('max'), // Node Scope Only
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
                data: footprintData ? loadDataForFootprint('avg') : loadDataGeneric('avg'), // Node Scope Only
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
                data: footprintData ? loadDataForFootprint('min') : loadDataGeneric('min'), // Node Scope Only
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

    onMount(() => {
        new Chart(
            document.getElementById(canvasId),
            {
                type: 'radar',
                data: data,
                options: options,
                height: height
            }
        );
    });

</script>

<!-- <div style="width: 500px;"><canvas id="dimensions"></canvas></div><br/> -->
<div class="chart-container">
    <canvas id={canvasId}></canvas>
</div>

<style>
    .chart-container {
        margin: auto;
        position: relative;
    }
</style>