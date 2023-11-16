<script>
    import { getContext } from 'svelte'
    // import { Button, Table, InputGroup, InputGroupText, Icon } from 'sveltestrap'
    import { mean, round } from 'mathjs'
    // import { findThresholds } from './plots/MetricPlot.svelte'
    // import { formatNumber } from './units.js'

    import { Pie } from 'svelte-chartjs';
    import {
        Chart as ChartJS,
        Title,
        Tooltip,
        Legend,
        Filler,
        ArcElement,
        CategoryScale
    } from 'chart.js';

    ChartJS.register(
        Title,
        Tooltip,
        Legend,
        Filler,
        ArcElement,
        CategoryScale
    );

    export let job
    export let jobMetrics

    export let size = 200
    export let displayLegend = true

    const footprintMetrics = ['mem_used', 'mem_bw','flops_any', 'cpu_load', 'acc_utilization'] // missing: energy , move to central config before deployment

    const footprintMetricConfigs = footprintMetrics.map((fm) => { 
        return getContext('metrics')(job.cluster, fm)
    }).filter( Boolean ) // Filter only "truthy" vals, see: https://stackoverflow.com/questions/28607451/removing-undefined-values-from-array

    console.log("FMCs", footprintMetricConfigs)

    // const footprintMetricThresholds = footprintMetricConfigs.map((fmc) => { // Only required if scopes smaller than node required
    //     return {name: fmc.name, ...findThresholds(fmc, 'node', job?.subCluster ? job.subCluster : '')} // Merge 2 objects
    // }).filter( Boolean )

    // console.log("FMTs", footprintMetricThresholds)

    const meanVals = footprintMetrics.map((fm) => {
        let jm = jobMetrics.find((jm) => jm.name === fm)
        if (jm?.metric?.statisticsSeries) {
            return {name: jm.name, scope: jm.scope, avg: round(mean(jm.metric.statisticsSeries.mean), 2)}
        } else if (jm?.metric?.series[0]) {
            return {name: jm.name, scope: jm.scope, avg: jm.metric.series[0].statistics.avg}
        }
    }).filter( Boolean )

    console.log("MVs", meanVals)

    const footprintLabels = meanVals.map((mv) => [mv.name, mv.name+' Threshold'])

    const footprintData = meanVals.map((mv) => {
        const metricConfig = footprintMetricConfigs.find((fmc) => fmc.name === mv.name)
        const levelPeak    = metricConfig.peak - mv.avg
        const levelNormal  = metricConfig.normal - mv.avg
        const levelCaution = metricConfig.caution - mv.avg
        const levelAlert   = metricConfig.alert - mv.avg

        if (levelAlert > 0) {
            return [mv.avg, levelAlert]
        } else if (levelCaution > 0) {
            return [mv.avg, levelCaution]
        } else if (levelNormal > 0) {
            return [mv.avg, levelNormal]
        } else {
            return [mv.avg, levelPeak]
        }
    })

    $: data = {
        labels: footprintLabels.flat(),
        datasets: [
            {
                backgroundColor: ['#AAA', '#777'],
                data: footprintData[0]
            },
            {
                backgroundColor: ['hsl(0, 100%, 60%)', 'hsl(0, 100%, 35%)'],
                data: footprintData[1]
            },
            {
                backgroundColor: ['hsl(100, 100%, 60%)', 'hsl(100, 100%, 35%)'],
                data: footprintData[2]
            },
            {
                backgroundColor: ['hsl(180, 100%, 60%)', 'hsl(180, 100%, 35%)'],
                data: footprintData[3]
            }
        ]
    }

    const options = { 
        maintainAspectRatio: false,
        animation: false,
        plugins: {
            legend: {
                display: displayLegend,
                labels: { // see: https://www.chartjs.org/docs/latest/samples/other-charts/multi-series-pie.html
                    generateLabels: function(chart) {
                        // Get the default label list
                        const original = ChartJS.overrides.pie.plugins.legend.labels.generateLabels;
                        const labelsOriginal = original.call(this, chart);

                        // Build an array of colors used in the datasets of the chart
                        let datasetColors = chart.data.datasets.map(function(e) {
                        return e.backgroundColor;
                        });
                        datasetColors = datasetColors.flat();

                        // Modify the color and hide state of each label
                        labelsOriginal.forEach(label => {
                        // There are twice as many labels as there are datasets. This converts the label index into the corresponding dataset index
                        label.datasetIndex = (label.index - label.index % 2) / 2;

                        // The hidden state must match the dataset's hidden state
                        label.hidden = !chart.isDatasetVisible(label.datasetIndex);

                        // Change the color to match the dataset
                        label.fillStyle = datasetColors[label.index];
                        });

                        return labelsOriginal;
                    }
                },
                onClick: function(mouseEvent, legendItem, legend) {
                    // toggle the visibility of the dataset from what it currently is
                    legend.chart.getDatasetMeta(
                        legendItem.datasetIndex
                    ).hidden = legend.chart.isDatasetVisible(legendItem.datasetIndex);
                    legend.chart.update();
                }
            },
            tooltip: {
                callbacks: {
                    label: function(context) {
                        const labelIndex = (context.datasetIndex * 2) + context.dataIndex;
                        return context.chart.data.labels[labelIndex] + ': ' + context.formattedValue;
                    },
                    title: function(context) {
                        const labelIndex = (context[0].datasetIndex * 2) + context[0].dataIndex;
                        return context[0].chart.data.labels[labelIndex];
                    }
                }
            }
        }
    }

</script>

 <div class="chart-container" style="--container-width: {size}; --container-height: {size}">
    <Pie {data} {options}/>
</div>

<style>
    .chart-container {
        position: relative;
        margin: auto;
        height: var(--container-height);
        width: var(--container-width);
    }
</style>


