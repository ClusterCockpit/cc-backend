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

    const metricsQuery = operationStore(`query($id: ID!, $metrics: [String!]!, $scopes: [MetricScope!]!) {
        jobMetrics(id: $id, metrics: $metrics, scopes: $scopes) {
            name
            metric {
                unit, scope, timestep
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
        (a, b) => maxScope([a.metric.scope, b.metric.scope]) == a.metric.scope
            ? (job.numNodes > 1 ? a : b)
            : (job.numNodes > 1 ? b : a), jobMetrics[0])

    const sortAndSelectScope = (jobMetrics) => metrics
        .map(name => jobMetrics.filter(jobMetric => jobMetric.name == name))
        .map(jobMetrics => jobMetrics.length > 0 ? selectScope(jobMetrics) : null)

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
            {#if metric != null}
                <MetricPlot
                    width={plotWidth}
                    height={plotHeight}
                    timestep={metric.metric.timestep}
                    scope={metric.metric.scope}
                    series={metric.metric.series}
                    statisticsSeries={metric.metric.statisticsSeries}
                    metric={metric.name}
                    cluster={cluster}
                    subCluster={job.subCluster} />
            {:else}
                <Card body color="warning">Missing Data</Card>
            {/if}
            </td>
        {/each}
    {/if}
</tr>
