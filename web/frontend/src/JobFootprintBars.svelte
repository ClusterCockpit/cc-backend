<script>
    import { getContext } from 'svelte'
    import {
        Row,
        Col,
        Spinner,
        Card,
        CardHeader,
        CardTitle,
        CardBody,
        Progress,
        Icon,
    } from "sveltestrap";
    import { mean, round } from 'mathjs'
    // import { findThresholds } from './plots/MetricPlot.svelte'
    // import { formatNumber, scaleNumbers } from './units.js'

    export let job
    export let jobMetrics
    export let view = 'job'
    export let width = 200

    const footprintMetrics = ['cpu_load', 'flops_any', 'mem_used', 'mem_bw'] // 'acc_utilization' / missing: energy , move to central config before deployment

    // console.log('JMs', jobMetrics.filter((jm) => footprintMetrics.includes(jm.name)))

    const footprintMetricConfigs = footprintMetrics.map((fm) => { 
        return getContext('metrics')(job.cluster, fm)
    }).filter( Boolean ) // Filter only "truthy" vals, see: https://stackoverflow.com/questions/28607451/removing-undefined-values-from-array

    // console.log("FMCs", footprintMetricConfigs)

    // const footprintMetricThresholds = footprintMetricConfigs.map((fmc) => { // Only required if scopes smaller than node required
    //     return {name: fmc.name, ...findThresholds(fmc, 'node', job?.subCluster ? job.subCluster : '')} // Merge 2 objects
    // }).filter( Boolean )

    // console.log("FMTs", footprintMetricThresholds)

    const footprintData = footprintMetrics.map((fm) => {
        // From JobMetrics: Only Node Scope
        const jm = jobMetrics.find((jm) => jm.name === fm && jm.scope === 'node')
        // ... get Mean
        let mv = null
        if (jm?.metric?.statisticsSeries) {
            mv = round(mean(jm.metric.statisticsSeries.mean), 2)
        } else if (jm?.metric?.series[0]) {
            mv = jm.metric.series[0].statistics.avg
        }
        // ... get Unit
        let unit = null
        if (jm?.metric?.unit?.base) {
            unit = jm.metric.unit.prefix + jm.metric.unit.base
        } else {
            unit = ''
        }

        // From MetricConfig: Scope only for scaling -> Not of interest here
        const metricConfig = footprintMetricConfigs.find((fmc) => fmc.name === fm)
        // ... get Thresholds
        const levelNormal  = metricConfig.normal - mv
        const levelCaution = metricConfig.caution - mv
        const levelAlert   = metricConfig.alert - mv

        // Collect
        if (fm !== 'mem_used') { // Alert if usage is low, peak as maxmimum possible (scaled down for flops_any)
            if (levelAlert > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((metricConfig.peak * 0.85), 0) : metricConfig.peak,
                    color: 'danger',
                    message: 'Metric strongly below common levels!',
                    impact: 3
                }
            } else if (levelCaution > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((metricConfig.peak * 0.85), 0) : metricConfig.peak,
                    color: 'warning',
                    message: 'Metric below common levels',
                    impact: 2
                }
            } else if (levelNormal > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((metricConfig.peak * 0.85), 0) : metricConfig.peak,
                    color: 'success',
                    message: 'Metric within common levels',
                    impact: 1
                }
            } else {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((metricConfig.peak * 0.85), 0) : metricConfig.peak,
                    color: 'info',
                    message: 'Metric performs better than common levels',
                    impact: 0
                }
            }
        } else { // Inverse Logic: Alert if usage is high, Peak is bad and limits execution
            if (levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: metricConfig.peak,
                    color: 'danger',
                    message: 'Memory usage extremely above common levels!',
                    impact: 4
                }
            } else if (levelAlert > 0 && (levelCaution <= 0 && levelNormal <= 0)) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: metricConfig.peak,
                    color: 'danger',
                    message: 'Memory usage strongly above common levels!',
                    impact: 3
                }
            } else if (levelCaution > 0 && levelNormal <= 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: metricConfig.peak,
                    color: 'warning',
                    message: 'Memory usage above common levels',
                    impact: 2
                }
            } else {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: metricConfig.peak,
                    color: 'success',
                    message: 'Memory usage within common levels',
                    impact: 1
                }
            }
        }
    }).filter( Boolean )

    // console.log("FPD", footprintData)

</script>

<Card class="h-auto mt-1" style="min-width: {width}px;">
    {#if view === 'job'}
    <CardHeader>
        <CardTitle class="mb-0 d-flex justify-content-center">
            Core Metrics Footprint
        </CardTitle>
    </CardHeader>
    {/if}
    <CardBody>
        {#each footprintData as fpd}
            <div class="mb-1 d-flex justify-content-between">
                <div><b>{fpd.name}</b></div>
                <div class="cursor-help d-inline-flex" title={fpd.message}>
                    <div class="mx-1">
                        <!-- Alerts Only -->
                        {#if fpd.impact === 3}
                            <Icon name="exclamation-triangle-fill" class="text-danger"/>
                        {:else if fpd.impact === 2}
                            <Icon name="exclamation-triangle" class="text-warning"/>
                        {/if}
                        <!-- Emoji for all states-->
                        {#if fpd.impact === 4}
                            <Icon name="emoji-angry" class="text-danger"/>
                        {:else if fpd.impact === 3}
                            <Icon name="emoji-frown" class="text-danger"/>
                        {:else if fpd.impact === 2}
                            <Icon name="emoji-neutral" class="text-warning"/>
                        {:else if fpd.impact === 1}
                            <Icon name="emoji-smile" class="text-success"/>
                        {:else if fpd.impact === 0}
                            <Icon name="emoji-laughing" class="text-info"/>
                        {/if}
                    </div>
                    <div>
                        <!-- Print Values -->
                        {fpd.avg} / {fpd.max} {fpd.unit}
                    </div>
                </div>
            </div>
            <div class="mb-2"> <!-- title={fpd.message} -->
                <Progress
                    value={fpd.avg}
                    max={fpd.max}
                    color={fpd.color}
                />
            </div>
        {/each}
    </CardBody>
</Card>

<style>
    .cursor-help {
        cursor: help;
    }
</style>

