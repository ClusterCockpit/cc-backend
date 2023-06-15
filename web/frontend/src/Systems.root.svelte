<script>
    import { init } from './utils.js'
    import { Row, Col, Input, InputGroup, InputGroupText, Icon, Spinner, Card } from 'sveltestrap'
    import { queryStore, gql, getContextClient } from '@urql/svelte'
    import TimeSelection from './filters/TimeSelection.svelte'
    import PlotTable from './PlotTable.svelte'
    import MetricPlot from './plots/MetricPlot.svelte'
    import { getContext } from 'svelte'

    export let cluster
    export let from = null
    export let to = null

    const { query: initq } = init()

    if (from == null || to == null) {
        to = new Date(Date.now())
        from = new Date(to.getTime())
        from.setMinutes(from.getMinutes() - 30)
    }

    const clusters = getContext('clusters')
    const ccconfig = getContext('cc-config')
    const metricConfig = getContext('metrics')

    let plotHeight = 300
    let hostnameFilter = ''
    let selectedMetric = ccconfig.system_view_selectedMetric

    const client = getContextClient();
    $: nodesQuery = queryStore({
        client: client,
        query: gql`query($cluster: String!, $metrics: [String!], $from: Time!, $to: Time!) {
            nodeMetrics(cluster: $cluster, metrics: $metrics, from: $from, to: $to) {
                host
                subCluster
                metrics {
                    name
                    scope
                    metric {
                        timestep
                        unit { base, prefix }
                        series {
                            statistics { min, avg, max }
                            data
                        }
                    }
                }
            }
        }`,
        variables: {
            cluster: cluster,
            metrics: [selectedMetric],
            from: from.toISOString(),
            to: to.toISOString()
        }
    })

    let metricUnits = {}
    $: if ($nodesQuery.data) {
        let thisCluster = clusters.find(c => c.name == cluster)
        if (thisCluster) {
            for (let metric of thisCluster.metricConfig) {
                if (metric.unit.prefix || metric.unit.base) {
                    metricUnits[metric.name] = '(' + (metric.unit.prefix ? metric.unit.prefix : '') + (metric.unit.base ? metric.unit.base : '') + ')'
                } else { // If no unit defined: Omit Unit Display
                    metricUnits[metric.name] = ''
                }
            }
        }
    }

</script>

<Row>
    {#if $initq.error}
        <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq.fetching}
        <Spinner/>
    {:else}
        <Col>
            <TimeSelection
                bind:from={from}
                bind:to={to} />
        </Col>
        <Col>
            <InputGroup>
                <InputGroupText><Icon name="graph-up" /></InputGroupText>
                <InputGroupText>Metric</InputGroupText>
                <select class="form-select" bind:value={selectedMetric}>
                    {#each clusters.find(c => c.name == cluster).metricConfig as metric}
                        <option value={metric.name}>{metric.name} {metricUnits[metric.name]}</option>
                    {/each}
                </select>
            </InputGroup>
        </Col>
        <Col>
            <InputGroup>
                <InputGroupText><Icon name="hdd" /></InputGroupText>
                <InputGroupText>Find Node</InputGroupText>
                <Input placeholder="hostname..." type="text" bind:value={hostnameFilter} />
            </InputGroup>
        </Col>
    {/if}
</Row>
<br/>
<Row>
    <Col>
        {#if $nodesQuery.error}
            <Card body color="danger">{$nodesQuery.error.message}</Card>
        {:else if $nodesQuery.fetching || $initq.fetching}
            <Spinner/>
        {:else}
            <PlotTable
                let:item
                let:width
                itemsPerRow={ccconfig.plot_view_plotsPerRow}
                items={$nodesQuery.data.nodeMetrics
                    .filter(h => h.host.includes(hostnameFilter) && h.metrics.some(m => m.name == selectedMetric && m.scope == 'node'))
                    .map(function (h) {
                        let thisConfig = metricConfig(cluster, selectedMetric)
                        let thisSCIndex = thisConfig.subClusters.findIndex(sc => sc.name == h.subCluster)
                        // Metric remove == true
                        if (thisSCIndex >= 0) {
                            if (thisConfig.subClusters[thisSCIndex].remove == true) {
                                return { host: h.host, subCluster: h.subCluster, data: null, removed: true }
                            }
                        }
                        // Else
                        return { host: h.host, subCluster: h.subCluster, data: h.metrics.find(m => m.name == selectedMetric && m.scope == 'node'), removed: false }
                    })
                    .sort((a, b) => a.host.localeCompare(b.host))}>

                    <h4 style="width: 100%; text-align: center;"><a style="display: block;padding-top: 15px;" href="/monitoring/node/{cluster}/{item.host}">{item.host} ({item.subCluster})</a></h4>
                    {#if item.removed == false && item.data != null}
                        <MetricPlot
                            width={width}
                            height={plotHeight}
                            timestep={item.data.metric.timestep}
                            series={item.data.metric.series}
                            metric={item.data.name}
                            cluster={clusters.find(c => c.name == cluster)}
                            subCluster={item.subCluster} />
                    {:else if item.removed == true && item.data == null}
                        <Card body color="info">Metric '{ selectedMetric }' disabled for subcluster '{ item.subCluster }'</Card>
                    {:else}
                        <Card body color="warning">Missing Data</Card>
                    {/if}
            </PlotTable>
        {/if}
    </Col>
</Row>

