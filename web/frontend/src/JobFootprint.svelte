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

    export let job
    export let jobMetrics
    export let view = 'job'
    export let width = 'auto'

    const clusters         = getContext('clusters')
    const subclusterConfig = clusters.find((c) => c.name == job.cluster).subClusters.find((sc) => sc.name == job.subCluster)

    const footprintMetrics = (job.numAcc !== 0)
        ? (job.exclusive !== 1) 
            ? ['cpu_load', 'flops_any', 'acc_utilization']
            : ['cpu_load', 'flops_any', 'acc_utilization', 'mem_bw']
        : (job.exclusive !== 1) 
            ? ['cpu_load', 'flops_any', 'mem_used']
            : ['cpu_load', 'flops_any', 'mem_used', 'mem_bw']

    const footprintData = footprintMetrics.map((fm) => {
        // Mean: Primarily use backend sourced avgs from job.*, secondarily calculate/read from metricdata
        let mv = null
        if (fm === 'cpu_load' && job.loadAvg !== 0) {
            mv = round(job.loadAvg, 2)
        } else if (fm === 'flops_any' && job.flopsAnyAvg !== 0) {
            mv = round(job.flopsAnyAvg, 2)
        } else if (fm === 'mem_bw' && job.memBwAvg !== 0) {
            mv = round(job.memBwAvg, 2)
        } else { // Calculate from jobMetrics
            const jm  = jobMetrics.find((jm) => jm.name === fm && jm.scope === 'node')
            if (jm?.metric?.statisticsSeries) {
                mv = round(mean(jm.metric.statisticsSeries.mean), 2)
            } else if (jm?.metric?.series?.length > 1) {
                const avgs = jm.metric.series.map(jms => jms.statistics.avg)
                mv = round(mean(avgs), 2)
            } else {
                mv = jm.metric.series[0].statistics.avg
            }
        }
        
        // Unit
        const fmc = getContext('metrics')(job.cluster, fm)
        let unit = null
        if (fmc?.unit?.base) {
            unit = fmc.unit.prefix + fmc.unit.base
        } else {
            unit = ''
        }

        // Threshold / -Differences
        const fmt = findJobThresholds(job, fmc, subclusterConfig)
        const levelPeak    = fm === 'flops_any' ? round((fmt.peak * 0.85), 0) - mv : fmt.peak - mv // Scale flops_any down
        const levelNormal  = fmt.normal - mv
        const levelCaution = fmt.caution - mv
        const levelAlert   = fmt.alert - mv

        // Define basic data
        const fmBase = {
            name: fm,
            unit: unit,
            avg: mv,
            max: fm === 'flops_any' ? round((fmt.peak * 0.85), 0) : fmt.peak
        }

        // Collect
        if (fm !== 'mem_used') { // Alert if usage is low, peak as maxmimum possible (scaled down for flops_any)
            if (levelAlert > 0) {
                return {
                    ...fmBase,
                    color: 'danger',
                    message: 'Metric strongly below common levels!',
                    impact: 3
                }
            } else if (levelCaution > 0) {
                return {
                    ...fmBase,
                    color: 'warning',
                    message: 'Metric below common levels',
                    impact: 2
                }
            } else if (levelNormal > 0) {
                return {
                    ...fmBase,
                    color: 'success',
                    message: 'Metric within common levels',
                    impact: 1
                }
            } else if (levelPeak > 0) {
                return {
                    ...fmBase,
                    color: 'info',
                    message: 'Metric performs better than common levels',
                    impact: 0
                }
            } else { // Possible artifacts - <5% Margin OK, >5% warning, > 50% danger
                if (fmBase.avg >= (1.5 * fmBase.max)) {
                    return {
                        ...fmBase,
                        color: 'secondary',
                        message: 'Metric average at least 50% above common peak value: Check data for artifacts!',
                        impact: -2
                    }
                } else if (fmBase.avg >= (1.05 * fmBase.max)) {
                    return {
                        ...fmBase,
                        color: 'secondary',
                        message: 'Metric average at least 5% above common peak value: Check data for artifacts',
                        impact: -1
                    }
                } else {
                    return {
                        ...fmBase,
                        color: 'info',
                        message: 'Metric performs better than common levels',
                        impact: 0
                    }
                }
            }
        } else { // Inverse Logic: Alert if usage is high, Peak is bad and limits execution
            if (levelPeak <= 0 && levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0) {  // Possible artifacts - <5% Margin OK, >5% warning, > 50% danger
                if (fmBase.avg >= (1.5 * fmBase.max)) {
                    return {
                        ...fmBase,
                        color: 'secondary',
                        message: 'Memory usage at least 50% above possible maximum value: Check data for artifacts!',
                        impact: -2
                    }
                } else if (fmBase.avg >= (1.05 * fmBase.max)) {
                    return {
                        ...fmBase,
                        color: 'secondary',
                        message: 'Memory usage at least 5% above possible maximum value: Check data for artifacts!',
                        impact: -1
                    }
                } else {
                    return {
                        ...fmBase,
                        color: 'danger',
                        message: 'Memory usage extremely above common levels!',
                        impact: 4
                    }
                }
            } else if (levelAlert <= 0 && levelCaution <= 0 && levelNormal <= 0) {
                return {
                    ...fmBase,
                    color: 'danger',
                    message: 'Memory usage extremely above common levels!',
                    impact: 4
                }
            } else if (levelAlert > 0 && (levelCaution <= 0 && levelNormal <= 0)) {
                return {
                    ...fmBase,
                    color: 'danger',
                    message: 'Memory usage strongly above common levels!',
                    impact: 3
                }
            } else if (levelCaution > 0 && levelNormal <= 0) {
                return {
                    ...fmBase,
                    color: 'warning',
                    message: 'Memory usage above common levels',
                    impact: 2
                }
            } else {
                return {
                    ...fmBase,
                    color: 'success',
                    message: 'Memory usage within common levels',
                    impact: 1
                }
            }
        }
    })
</script>

<script context="module">
    export function findJobThresholds(job, metricConfig, subClusterConfig) {

    if (!job || !metricConfig || !subClusterConfig) {
        console.warn('Argument missing for findJobThresholds!')
        return null
    }

    const subclusterThresholds = metricConfig.subClusters.find(sc => sc.name == subClusterConfig.name)
    const defaultThresholds = { 
        peak:    subclusterThresholds ? subclusterThresholds.peak    : metricConfig.peak,
        normal:  subclusterThresholds ? subclusterThresholds.normal  : metricConfig.normal,
        caution: subclusterThresholds ? subclusterThresholds.caution : metricConfig.caution,
        alert:   subclusterThresholds ? subclusterThresholds.alert   : metricConfig.alert
    }

    if (job.exclusive === 1) { // Exclusive: Use as defined
        return defaultThresholds
    } else { // Shared: Handle specifically
        if (metricConfig.name === 'cpu_load') { // Special: Avg Aggregation BUT scaled based on #hwthreads
            return { 
                peak:    job.numHWThreads,
                normal:  job.numHWThreads,
                caution: defaultThresholds.caution,
                alert:   defaultThresholds.alert
            }   
        } else if (metricConfig.aggregation === 'avg' ){
            return defaultThresholds
        } else if (metricConfig.aggregation === 'sum' ){
            const jobFraction = job.numHWThreads / subClusterConfig.topology.node.length
            return {
                peak: round((defaultThresholds.peak * jobFraction), 0),
                normal: round((defaultThresholds.normal * jobFraction), 0),
                caution: round((defaultThresholds.caution * jobFraction), 0),
                alert: round((defaultThresholds.alert * jobFraction), 0)
            }
        } else {
            console.warn('Missing or unkown aggregation mode (sum/avg) for metric:', metricConfig)
            return null
        }
    } // Other job.exclusive cases?
}
</script>

<Card class="h-auto mt-1" style="width: {width}px;">
    {#if view === 'job'}
    <CardHeader>
        <CardTitle class="mb-0 d-flex justify-content-center">
            Core Metrics Footprint
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
