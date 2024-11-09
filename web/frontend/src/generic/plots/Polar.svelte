<!--
    @component Polar Plot based on chartJS Radar

    Properties:
    - `footprintData [Object]?`: job.footprint content, evaluated in regards to peak config in jobSummary.svelte [Default: null]
    - `metrics [String]?`: Metric names to display as polar plot [Default: null]
    - `cluster GraphQL.Cluster?`: Cluster Object of the parent job [Default: null]
    - `subCluster GraphQL.SubCluster?`: SubCluster Object of the parent job [Default: null]
    - `jobMetrics [GraphQL.JobMetricWithName]?`: Metric data [Default: null]
    - `height Number?`: Plot height [Default: 365]
 -->

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
                    console.warn(`PolarPlot: No metric data (or config) for '${fpd.name}'`)
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
        const peak = getMetricConfig(cluster, subCluster, name).peak
        const metric = jobMetrics.find(m => m.name == name && m.scope == "node")
        const value = getStat(metric.metric) / peak
        return value <= 1. ? value : 1.
    })

    const getValuesForStatFootprint = (getStat) => labels.map(name => {
        const peak = footprintData.find(fpd => fpd.name === name).peak
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

    function loadDataGeneric(type) {
        if (type === 'avg') {
            return getValuesForStatGeneric(getAvg)
        } else if (type === 'max') {
            return getValuesForStatGeneric(getMax)
        }
        console.log('Unknown Type For Polar Data')
        return []
    }

    function loadDataForFootprint(type) {
        if (type === 'avg') {
            return getValuesForStatFootprint(getAvg)
        } else if (type === 'max') {
            return getValuesForStatFootprint(getMax)
        }
        console.log('Unknown Type For Polar Data')
        return []
    }

    const data = {
        labels: labels,
        datasets: [
            {
                label: 'Max',
                data: footprintData ? loadDataForFootprint('max') : loadDataGeneric('max'), // 
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
                data: footprintData ? loadDataForFootprint('avg') : loadDataGeneric('avg'), // getValuesForStat(getAvg)
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