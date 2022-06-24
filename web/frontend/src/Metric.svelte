<script>
    import { getContext, createEventDispatcher } from 'svelte'
    import Timeseries from './plots/MetricPlot.svelte'
    import { InputGroup, InputGroupText, Spinner, Card } from 'sveltestrap'
    import { fetchMetrics, minScope } from './utils'

    export let job
    export let metric
    export let scopes
    export let width

    const dispatch = createEventDispatcher()
    const cluster = getContext('clusters').find(cluster => cluster.name == job.cluster)
    const subCluster = cluster.subClusters.find(subCluster => subCluster.name == job.subCluster)
    const metricConfig = cluster.metricConfig.find(metricConfig => metricConfig.name == metric)

    let selectedScope = minScope(scopes.map(s => s.scope)), selectedHost = null, plot, fetching = false, error = null

    $: avaliableScopes = scopes.map(metric => metric.scope)
    $: data = scopes.find(metric => metric.scope == selectedScope)
    $: series = data?.series.filter(series => selectedHost == null || series.hostname == selectedHost)

    let from = null, to = null
    export function setTimeRange(f, t) {
        from = f, to = t
    }

    $: if (plot != null) plot.setTimeRange(from, to)

    export async function loadMore() {
        fetching = true
        let response = await fetchMetrics(job, [metric], ["core"])
        fetching = false

        if (response.error) {
            error = response.error
            return
        }

        for (let jm of response.data.jobMetrics) {
            if (jm.metric.scope != "node") {
                scopes.push(jm.metric)
                selectedScope = jm.metric.scope
                dispatch('more-loaded', jm)
                if (!avaliableScopes.includes(selectedScope))
                    avaliableScopes = [...avaliableScopes, selectedScope]
            }
        }
    }

    $: if (selectedScope == "load-more") loadMore()
</script>
<InputGroup>
    <InputGroupText style="min-width: 150px;">
        {metric} ({metricConfig?.unit})
    </InputGroupText>
    <select class="form-select" bind:value={selectedScope}>
        {#each avaliableScopes as scope}
            <option value={scope}>{scope}</option>
        {/each}
        {#if avaliableScopes.length == 1 && metricConfig?.scope != "node"}
            <option value={"load-more"}>Load more...</option>
        {/if}
    </select>
    {#if job.resources.length > 1}
        <select class="form-select" bind:value={selectedHost}>
            <option value={null}>All Hosts</option>
            {#each job.resources as { hostname }}
                <option value={hostname}>{hostname}</option>
            {/each}
        </select>
    {/if}
</InputGroup>
{#key series}
    {#if fetching == true}
        <Spinner/>
    {:else if error != null}
        <Card body color="danger">{error.message}</Card>
    {:else if series != null}
        <Timeseries
            bind:this={plot}
            width={width} height={300}
            cluster={cluster} subCluster={subCluster}
            timestep={data.timestep}
            scope={selectedScope} metric={metric}
            series={series} />
    {/if}
{/key}
