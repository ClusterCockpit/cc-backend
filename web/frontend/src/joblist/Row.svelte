<!-- 
    @component

    Properties:
    - job:        GraphQL.Job (constant/key)
    - metrics:    [String]    (can change)
    - plotWidth:  Number
    - plotHeight: Number
 -->

<script>
    import { operationStore, query } from '@urql/svelte'
    import { getContext } from 'svelte'
    import { Card, Spinner } from 'sveltestrap'
    import MetricPlot from '../plots/MetricPlot.svelte'
    import JobInfo from './JobInfo.svelte'
    import { maxScope } from '../utils.js'

    export let job
    export let metrics
    export let plotWidth
    export let plotHeight = 275

    let scopes = [job.numNodes == 1 ? 'core' : 'node']

    const cluster = getContext('clusters').find(c => c.name == job.cluster)
    // Get all MetricConfs which include subCluster-specific settings for this job
    const metricConfig = getContext('metrics')
    const metricsQuery = operationStore(`query($id: ID!, $metrics: [String!]!, $scopes: [MetricScope!]!) {
        jobMetrics(id: $id, metrics: $metrics, scopes: $scopes) {
            name
            scope
            metric {
                unit { prefix, base }, timestep
                statisticsSeries { min, mean, max }
                series {
                    hostname, id, data
                    statistics { min, avg, max }
                }
            }
        }
    }`, {
        id: job.id,
        metrics,
        scopes
    })

    const selectScope = (jobMetrics) => jobMetrics.reduce(
        (a, b) => maxScope([a.scope, b.scope]) == a.scope
            ? (job.numNodes > 1 ? a : b)
            : (job.numNodes > 1 ? b : a), jobMetrics[0])

    const sortAndSelectScope = (jobMetrics) => metrics
        .map(function(name) {
            // Get MetricConf for this selected/requested metric
            let thisConfig = metricConfig(cluster, name)
            let thisSCIndex = thisConfig.subClusters.findIndex(sc => sc.name == job.subCluster)
            // Check if Subcluster has MetricConf: If not found (index == -1), no further remove flag check required
            if (thisSCIndex >= 0) {
                // SubCluster Config present: Check if remove flag is set
                if (thisConfig.subClusters[thisSCIndex].remove == true) {
                    // Return null data and informational flag
                    // console.log('Case 1.1 -> Returned')
                    // console.log({removed: true, data: null})
                    return {removed: true, data: null}
                } else {
                    // load and return metric, if data available
                    let thisMetric = jobMetrics.filter(jobMetric => jobMetric.name == name) // Returns Array
                    if (thisMetric.length > 0) {
                        // console.log('Case 1.2.1 -> Returned')
                        // console.log({removed: false, data: thisMetric})
                        return {removed: false, data: thisMetric}
                    } else {
                        // console.log('Case 1.2.2 -> Returned:')
                        // console.log({removed: false, data: null})
                        return {removed: false, data: null}
                    }
                }
            } else {
                // No specific subCluster config: 'remove' flag not set, deemed false -> load and return metric, if data available
                let thisMetric = jobMetrics.filter(jobMetric => jobMetric.name == name) // Returns Array
                if (thisMetric.length > 0) {
                    // console.log('Case 2.1 -> Returned')
                    // console.log({removed: false, data: thisMetric})
                    return {removed: false, data: thisMetric}
                } else {
                    // console.log('Case 2.2 -> Returned')
                    // console.log({removed: false, data: null})
                    return {removed: false, data: null}
                }
            }
        })
        .map(function(jobMetrics) {
            if (jobMetrics.data != null && jobMetrics.data.length > 0) {
                // console.log('Before')
                // console.log(jobMetrics.data)
                // console.log('After')
                // console.log(selectScope(jobMetrics.data))
                let res = {removed: jobMetrics.removed, data: selectScope(jobMetrics.data)}
                // console.log('Packed')
                // console.log(res)
                return res
            } else {
                return jobMetrics
            }
        })

    $: metricsQuery.variables = { id: job.id, metrics, scopes }

    if (job.monitoringStatus)
        query(metricsQuery)
</script>

<tr>
    <td>
        <JobInfo job={job}/>
    </td>
    {#if job.monitoringStatus == 0 || job.monitoringStatus == 2}
        <td colspan="{metrics.length}">
            <Card body color="warning">Not monitored or archiving failed</Card>
        </td>
    {:else if $metricsQuery.fetching}
        <td colspan="{metrics.length}" style="text-align: center;">
            <Spinner secondary />
        </td>
    {:else if $metricsQuery.error}
        <td colspan="{metrics.length}">
            <Card body color="danger" class="mb-3">
                {$metricsQuery.error.message.length > 500
                    ? $metricsQuery.error.message.substring(0, 499)+'...'
                    : $metricsQuery.error.message}
            </Card>
        </td>
    {:else}
        {#each sortAndSelectScope($metricsQuery.data.jobMetrics) as metric, i (metric || i)}
            <td>
            <!-- Subluster Metricconfig remove keyword for jobtables (joblist main, user joblist, project joblist) to be used here as toplevel case-->
            {#if metric.removed == false && metric.data != null}
                <MetricPlot
                    width={plotWidth}
                    height={plotHeight}
                    timestep={metric.data.metric.timestep}
                    scope={metric.data.scope}
                    series={metric.data.metric.series}
                    statisticsSeries={metric.data.metric.statisticsSeries}
                    metric={metric.data.name}
                    cluster={cluster}
                    subCluster={job.subCluster} />
            {:else if metric.removed == true && metric.data == null}
                <Card body color="info">Metric disabled for subcluster '{ job.subCluster }'</Card>
            {:else}
                <Card body color="warning">Missing Data</Card>
            {/if}
            </td>
        {/each}
    {/if}
</tr>
