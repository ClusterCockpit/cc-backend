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
    export let displayLegend = false

    const footprintMetrics = ['mem_used', 'mem_bw','flops_any', 'cpu_load'] // 'acc_utilization' / missing: energy , move to central config before deployment

    console.log('JMs', jobMetrics.filter((jm) => footprintMetrics.includes(jm.name)))

    const footprintMetricConfigs = footprintMetrics.map((fm) => { 
        return getContext('metrics')(job.cluster, fm)
    }).filter( Boolean ) // Filter only "truthy" vals, see: https://stackoverflow.com/questions/28607451/removing-undefined-values-from-array

    console.log("FMCs", footprintMetricConfigs)

    // const footprintMetricThresholds = footprintMetricConfigs.map((fmc) => { // Only required if scopes smaller than node required
    //     return {name: fmc.name, ...findThresholds(fmc, 'node', job?.subCluster ? job.subCluster : '')} // Merge 2 objects
    // }).filter( Boolean )

    // console.log("FMTs", footprintMetricThresholds)

    const meanVals = footprintMetrics.map((fm) => {
        let jm = jobMetrics.find((jm) => jm.name === fm && jm.scope === 'node') // Only Node Scope
        let mv = null
        if (jm?.metric?.statisticsSeries) {
            mv = {name: jm.name, avg: round(mean(jm.metric.statisticsSeries.mean), 2)}
        } else if (jm?.metric?.series[0]) {
            mv = {name: jm.name, avg: jm.metric.series[0].statistics.avg}
        }

        if (jm?.metric?.unit?.base) {
            return {...mv, unit: jm.metric.unit.prefix + jm.metric.unit.base}
        } else {
            return {...mv, unit: ''}
        }

    }).filter( Boolean )

    console.log("MVs", meanVals)

    const footprintData = meanVals.map((mv) => {
        const metricConfig = footprintMetricConfigs.find((fmc) => fmc.name === mv.name)
        const levelPeak    = metricConfig.peak - mv.avg
        const levelNormal  = metricConfig.normal - mv.avg
        const levelCaution = metricConfig.caution - mv.avg
        const levelAlert   = metricConfig.alert - mv.avg

        if (mv.name !== 'mem_used') { // Alert if usage is low, peak is high good usage
            if (levelAlert > 0) {
                return {
                    data: [mv.avg, levelAlert],
                    color: ['hsl(0, 100%, 60%)', '#AAA'],
                    messages: ['Metric strongly below recommended levels!', 'Difference towards acceptable performace'],
                    impact: 2
                } // 'hsl(0, 100%, 35%)'
            } else if (levelCaution > 0) {
                return {
                    data: [mv.avg, levelCaution],
                    color: ['hsl(56, 100%, 50%)', '#AAA'],
                    messages: ['Metric below recommended levels', 'Difference towards normal performance'],
                    impact: 1
                } // '#d5b60a'
            } else if (levelNormal > 0) {
                return {
                    data: [mv.avg, levelNormal],
                    color: ['hsl(100, 100%, 60%)', '#AAA'],
                    messages: ['Metric within recommended levels', 'Difference towards optimal performance'],
                    impact: 0
                } // 'hsl(100, 100%, 35%)'
            } else if (levelPeak > 0) {
                return {
                    data: [mv.avg, levelPeak],
                    color: ['hsl(180, 100%, 60%)', '#AAA'],
                    messages: ['Metric performs better than recommended levels', 'Difference towards maximum capacity'], // "Perfomrs optimal"?
                    impact: 0
                } // 'hsl(180, 100%, 35%)'
            } else { // If avg greater than configured peak: render negative diff as zero
                return {
                    data: [mv.avg, 0],
                    color: ['hsl(180, 100%, 60%)', '#AAA'],
                    messages: ['Metric performs at maximum capacity', 'Maximum reached'],
                    impact: 0
                } // 'hsl(180, 100%, 35%)'
            }
        } else { // Inverse Logic: Alert if usage is high, Peak is bad and limits execution
            if (levelPeak <= 0 && levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0) {  // If avg greater than configured peak: render negative diff as zero
                return {
                    data: [mv.avg, 0],
                    color: ['#7F00FF', '#AAA'],
                    messages: ['Memory usage at maximum capacity!', 'Maximum reached'],
                    impact: 4
                } // '#5D3FD3'
            } else if (levelPeak > 0 && (levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0)) {
                return {
                    data: [mv.avg, levelPeak],
                    color: ['#7F00FF', '#AAA'],
                    messages: ['Memory usage extremely above recommended levels!', 'Difference towards maximum memory capacity'],
                    impact: 2
                } // '#5D3FD3'
            } else if (levelAlert > 0 && (levelCaution <= 0 && levelNormal <= 0)) {
                return {
                    data: [mv.avg, levelAlert],
                    color: ['hsl(0, 100%, 60%)', '#AAA'],
                    messages: ['Memory usage strongly above recommended levels!', 'Difference towards highly alerting memory usage'],
                    impact: 2
                } // 'hsl(0, 100%, 35%)'
            } else if (levelCaution > 0 && levelNormal <= 0) {
                return {
                    data: [mv.avg, levelCaution],
                    color: ['hsl(56, 100%, 50%)', '#AAA'],
                    messages: ['Memory usage above recommended levels', 'Difference towards alerting memory usage'],
                    impact: 1
                } // '#d5b60a'
            } else {
                return {
                    data: [mv.avg, levelNormal],
                    color: ['hsl(100, 100%, 60%)', '#AAA'],
                    messages: ['Memory usage within recommended levels', 'Difference towards increased memory usage'],
                    impact: 0
                } // 'hsl(100, 100%, 35%)'
            }
        }
    })

    console.log("FPD", footprintData)

    // Collect data for chartjs
    const footprintLabels    = meanVals.map((mv) => [mv.name, 'Threshold']).flat()
    const footprintUnits     = meanVals.map((mv) => [mv.unit, mv.unit]).flat()
    const footprintMessages  = footprintData.map((fpd) => fpd.messages).flat()
    const footprintResultSum = footprintData.map((fpd) => fpd.impact).reduce((accumulator, currentValue) => { return accumulator + currentValue }, 0)
    let   footprintResult    = ''

    if (footprintResultSum <= 1) {
        footprintResult = 'good'
    } else if (footprintResultSum > 1 && footprintResultSum <= 3) {
        footprintResult = 'well'
    } else if (footprintResultSum > 3 && footprintResultSum <= 5) {
        footprintResult = 'acceptable'
    } else {
        footprintResult = 'badly'
    }

    $: data = {
        labels: footprintLabels,
        datasets: [
            {
                backgroundColor: footprintData[0].color,
                data: footprintData[0].data
            },
            {
                backgroundColor: footprintData[1].color,
                data: footprintData[1].data
            },
            {
                backgroundColor: footprintData[2].color,
                data: footprintData[2].data
            },
            {
                backgroundColor: footprintData[3].color,
                data: footprintData[3].data
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
                        if (context.chart.data.labels[labelIndex] === 'Threshold') {
                            return ' -' + context.formattedValue + ' ' + footprintUnits[labelIndex]
                        } else {
                            return '  ' + context.formattedValue + ' ' + footprintUnits[labelIndex]
                        }
                    },
                    title: function(context) {
                        const labelIndex = (context[0].datasetIndex * 2) + context[0].dataIndex;
                        if (context[0].chart.data.labels[labelIndex] === 'Threshold') {
                            return 'Until ' + context[0].chart.data.labels[labelIndex]
                        } else {
                            return 'Average ' + context[0].chart.data.labels[labelIndex]
                        }
                    },
                    footer: function(context) {
                        const labelIndex = (context[0].datasetIndex * 2) + context[0].dataIndex;
                        if (context[0].chart.data.labels[labelIndex] === 'Threshold') {
                            return  footprintMessages[labelIndex]
                        } else {
                            return  footprintMessages[labelIndex]
                        }
                    }
                }
            }
        }
    }

</script>

 <div class="chart-container" style="--container-width: {size}; --container-height: {size}">
    <Pie {data} {options}/>
</div>
<div class="mt-3 d-flex justify-content-center">
    <b>Overall Job Performance:&nbsp;</b> Your job {job.state === 'running' ? 'performs' : 'performed'} {footprintResult}.
</div>


<style>
    .chart-container {
        position: relative;
        margin: auto;
        height: var(--container-height);
        width: var(--container-width);
    }
</style>


