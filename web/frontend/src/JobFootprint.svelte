<script>
    import { getContext } from 'svelte'
    import {
        Card,
        CardHeader,
        CardTitle,
        CardBody,
        Progress,
        Icon,
        Tooltip
    } from "sveltestrap";
    import { mean, round } from 'mathjs'
    // import { formatNumber, scaleNumbers } from './units.js'

    export let job
    export let jobMetrics
    export let view = 'job'
    export let width = 'auto'

    const isAcceleratedJob = (job.numAcc    !== 0)
    const isSharedJob      = (job.exclusive !== 1)

    // console.log('JOB', job)
    console.log('ACCELERATED?', isAcceleratedJob)
    console.log('SHARED?', isSharedJob)

    const clusters = getContext('clusters')
    const subclusterConfig = clusters.find((c) => c.name == job.cluster).subClusters.find((sc) => sc.name == job.subCluster)

    console.log('SCC', subclusterConfig)

    /* NOTES:
        - 'mem_allocated' für shared jobs (noch todo / nicht in den jobdaten enthalten bisher)
        > For now: 'acc_util' gegen 'mem_used' für alex
        - Energy Metric Missiing, muss eingebaut werden
        - Diese Config in config.json?
        - Erste 5 / letzte 5 pts für avg auslassen? (Wenn minimallänge erreicht?) // Peak limited => Hier eigentlich nicht mein Proble, Ich zeige nur daten an die geliefert werden
    */

    const footprintMetrics = isAcceleratedJob ?
        ['cpu_load', 'flops_any', 'acc_utilization', 'mem_bw'] :
        ['cpu_load', 'flops_any', 'mem_used',        'mem_bw']

    console.log('JMs', jobMetrics.filter((jm) => footprintMetrics.includes(jm.name)))

    const footprintMetricConfigs = footprintMetrics.map((fm) => { 
        return getContext('metrics')(job.cluster, fm)
    }).filter( Boolean ) // Filter only "truthy" vals, see: https://stackoverflow.com/questions/28607451/removing-undefined-values-from-array

    console.log("FMCs", footprintMetricConfigs)

    const footprintMetricThresholds = footprintMetricConfigs.map((fmc) => {
        return {name: fmc.name, ...findJobThresholds(fmc, job, subclusterConfig)}
    }).filter( Boolean )

    console.log("FMTs", footprintMetricThresholds)

    const footprintData = footprintMetrics.map((fm) => {
        const jm = jobMetrics.find((jm) => jm.name === fm && jm.scope === 'node')
        // ... get Mean
        let mv = null
        if (jm?.metric?.statisticsSeries) {
            mv = round(mean(jm.metric.statisticsSeries.mean), 2) // see above
        } else if (jm?.metric?.series[0]) {
            mv = jm.metric.series[0].statistics.avg // see above
        }
        // ... get Unit
        let unit = null
        if (jm?.metric?.unit?.base) {
            unit = jm.metric.unit.prefix + jm.metric.unit.base
        } else {
            unit = ''
        }
        // Get Threshold Limits from scaled Thresholds per Metric
        const scaledThresholds = footprintMetricThresholds.find((fmc) => fmc.name === fm)
        const levelPeak    = fm === 'flops_any' ? round((scaledThresholds.peak * 0.85), 0) - mv : scaledThresholds.peak - mv // Scale flops_any down
        const levelNormal  = scaledThresholds.normal - mv
        const levelCaution = scaledThresholds.caution - mv
        const levelAlert   = scaledThresholds.alert - mv
        // Collect
        if (fm !== 'mem_used') { // Alert if usage is low, peak as maxmimum possible (scaled down for flops_any)
            if (levelAlert > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((scaledThresholds.peak * 0.85), 0) : scaledThresholds.peak,
                    color: 'danger',
                    message: 'Metric strongly below common levels!',
                    impact: 3
                }
            } else if (levelCaution > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((scaledThresholds.peak * 0.85), 0) : scaledThresholds.peak,
                    color: 'warning',
                    message: 'Metric below common levels',
                    impact: 2
                }
            } else if (levelNormal > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((scaledThresholds.peak * 0.85), 0) : scaledThresholds.peak,
                    color: 'success',
                    message: 'Metric within common levels',
                    impact: 1
                }
            } else if (levelPeak > 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((scaledThresholds.peak * 0.85), 0) : scaledThresholds.peak,
                    color: 'info',
                    message: 'Metric performs better than common levels',
                    impact: 0
                }
            } else { // Possible artifacts - <5% Margin OK, >5% warning, > 50% danger
                const checkData = {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: fm === 'flops_any' ? round((scaledThresholds.peak * 0.85), 0) : scaledThresholds.peak
                }

                if (checkData.avg >= (1.5 * checkData.max)) {
                    return {
                        ...checkData,
                        color: 'secondary',
                        message: 'Metric average at least 50% above common peak value: Check data for artifacts!',
                        impact: -2
                    }
                } else if (checkData.avg >= (1.05 * checkData.max)) {
                    return {
                        ...checkData,
                        color: 'secondary',
                        message: 'Metric average at least 5% above common peak value: Check data for artifacts',
                        impact: -1
                    }
                } else {
                    return {
                        ...checkData,
                        color: 'info',
                        message: 'Metric performs better than common levels',
                        impact: 0
                    }
                }
            }
        } else { // Inverse Logic: Alert if usage is high, Peak is bad and limits execution
            if (levelPeak <= 0 && levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0) {  // Possible artifacts - <5% Margin OK, >5% warning, > 50% danger
                const checkData = {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: scaledThresholds.peak
                }
                if (checkData.avg >= (1.5 * checkData.max)) {
                    return {
                        ...checkData,
                        color: 'secondary',
                        message: 'Memory usage at least 50% above possible maximum value: Check data for artifacts!',
                        impact: -2
                    }
                } else if (checkData.avg >= (1.05 * checkData.max)) {
                    return {
                        ...checkData,
                        color: 'secondary',
                        message: 'Memory usage at least 5% above possible maximum value: Check data for artifacts!',
                        impact: -1
                    }
                } else {
                    return {
                        ...checkData,
                        color: 'danger',
                        message: 'Memory usage extremely above common levels!',
                        impact: 4
                    }
                }
            } else if (levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: scaledThresholds.peak,
                    color: 'danger',
                    message: 'Memory usage extremely above common levels!',
                    impact: 4
                }
            } else if (levelAlert > 0 && (levelCaution <= 0 && levelNormal <= 0)) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: scaledThresholds.peak,
                    color: 'danger',
                    message: 'Memory usage strongly above common levels!',
                    impact: 3
                }
            } else if (levelCaution > 0 && levelNormal <= 0) {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: scaledThresholds.peak,
                    color: 'warning',
                    message: 'Memory usage above common levels',
                    impact: 2
                }
            } else {
                return {
                    name: fm,
                    unit: unit,
                    avg: mv,
                    max: scaledThresholds.peak,
                    color: 'success',
                    message: 'Memory usage within common levels',
                    impact: 1
                }
            }
        }
    }).filter( Boolean )

    console.log("FPD", footprintData)

</script>

<script context="module">
    export function findJobThresholds(metricConfig, job, subClusterConfig) {

    console.log('Hello', metricConfig.name, '@', subClusterConfig.name)

    if (!metricConfig || !job || !subClusterConfig) {
        console.warn('Argument missing for findJobThresholds!')
        return null
    }

    if (job.numHWThreads == subClusterConfig.topology.node.length   || // Job uses all available HWTs of one node
        job.numAcc == subClusterConfig.topology.accelerators.length || // Job uses all available GPUs of one node
        metricConfig.aggregation == 'avg'                           ){ // Metric uses "average" aggregation method

        console.log('Job uses all available Resources of one node OR uses "average" aggregation method, use unscaled thresholds')

        let subclusterThresholds = metricConfig.subClusters.find(sc => sc.name == subClusterConfig.name)
        if (subclusterThresholds) {
            console.log('subClusterThresholds found, use subCluster specific thresholds:', subclusterThresholds)
            return { 
                peak: subclusterThresholds.peak,
                normal: subclusterThresholds.normal,
                caution: subclusterThresholds.caution,
                alert: subclusterThresholds.alert
            }
        }

        return { 
            peak: metricConfig.peak,
            normal: metricConfig.normal,
            caution: metricConfig.caution,
            alert: metricConfig.alert
        }
    }

    if (metricConfig.aggregation != 'sum') {
        console.warn('Missing or unkown aggregation mode (sum/avg) for metric:', metricConfig)
        return null
    }

    /* Adapt based on numAccs? */
    const jobFraction = job.numHWThreads / subClusterConfig.topology.node.length
    //const fractionAcc = job.numAcc / subClusterConfig.topology.accelerators.length

    console.log('Fraction', jobFraction)

    return {
        peak: round((metricConfig.peak * jobFraction), 0),
        normal: round((metricConfig.normal * jobFraction), 0),
        caution: round((metricConfig.caution * jobFraction), 0),
        alert: round((metricConfig.alert * jobFraction), 0)
    }
}
</script>

<Card class="h-auto mt-1" style="width: {width}px;">
    {#if view === 'job'}
    <CardHeader>
        <CardTitle class="mb-0 d-flex justify-content-center">
            Core Metrics Footprint {isSharedJob ? '(Scaled)' : ''}
        </CardTitle>
    </CardHeader>
    {/if}
    <CardBody>
        {#each footprintData as fpd, index}
            <div class="mb-1 d-flex justify-content-between">
                <div>&nbsp;<b>{fpd.name}</b></div> <!-- For symmetry, see below ...-->
                <div class="cursor-help d-inline-flex" id={`footprint-${job.jobId}-${index}`}>
                    <div class="mx-1">
                        <!-- Alerts Only -->
                        {#if fpd.impact === 3}
                            <Icon name="exclamation-triangle-fill" class="text-danger"/>
                        {:else if fpd.impact === 2}
                            <Icon name="exclamation-triangle" class="text-warning"/>
                        {:else if fpd.impact === -1}
                            <Icon name="exclamation-triangle" class="text-warning"/>
                        {:else if fpd.impact === -2}
                            <Icon name="exclamation-triangle-fill" class="text-danger"/>
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
                        {:else if fpd.impact === -1}
                            <Icon name="emoji-dizzy" class="text-warning"/>
                        {:else if fpd.impact === -2}
                            <Icon name="emoji-dizzy" class="text-danger"/>
                        {/if}
                    </div>
                    <div>
                        <!-- Print Values -->
                        {fpd.avg} / {fpd.max} {fpd.unit} &nbsp; <!-- To increase margin to tooltip: No other way manageable ...-->
                    </div>
                </div>
                <Tooltip target={`footprint-${job.jobId}-${index}`} placement="right" offset={[0, 20]}>{fpd.message}</Tooltip>
            </div>
            <div class="mb-2">
                <Progress
                    value={fpd.avg}
                    max={fpd.max}
                    color={fpd.color}
                />
            </div>
        {/each}
        {#if job?.metaData?.message}
            <hr class="mt-1 mb-2"/>
            {@html job.metaData.message}
        {/if}
    </CardBody>
</Card>

<style>
    .cursor-help {
        cursor: help;
    }
</style>
