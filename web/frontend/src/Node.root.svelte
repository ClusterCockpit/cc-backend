<script>
    import { init } from './utils.js'
    import { Row, Col, InputGroup, InputGroupText, Icon, Spinner, Card } from 'sveltestrap'
    import { operationStore, query } from '@urql/svelte'
    import TimeSelection from './filters/TimeSelection.svelte'
    import PlotTable from './PlotTable.svelte'
    import MetricPlot from './plots/MetricPlot.svelte'
    import { getContext } from 'svelte'

    export let cluster
    export let hostname
    export let from = null
    export let to = null

    const { query: initq } = init()

    if (from == null || to == null) {
        to = new Date(Date.now())
        from = new Date(to.getTime())
        from.setMinutes(from.getMinutes() - 30)
    }

    const ccconfig = getContext('cc-config'), clusters = getContext('clusters')

    const nodesQuery = operationStore(`query($cluster: String!, $nodes: [String!], $from: Time!, $to: Time!) {
        nodeMetrics(cluster: $cluster, nodes: $nodes, from: $from, to: $to) {
            host, subCluster
            metrics {
                name,
                metric {
                    timestep
                    scope
                    series {
                        statistics { min, avg, max }
                        data
                    }
                }
            }
        }
    }`, {
        cluster: cluster,
        nodes: [hostname],
        from: from.toISOString(),
        to: to.toISOString()
    })

    $: $nodesQuery.variables = { cluster, nodes: [hostname], from: from.toISOString(), to: to.toISOString() }

    query(nodesQuery)

    $: console.log($nodesQuery?.data?.nodeMetrics[0].metrics)
</script>

<Row>
    {#if $initq.error}
        <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq.fetching}
        <Spinner/>
    {:else}
        <Col>
            <InputGroup>
                <InputGroupText><Icon name="hdd"/></InputGroupText>
                <InputGroupText>{hostname} ({cluster})</InputGroupText>
            </InputGroup>
        </Col>
        <Col>
            <TimeSelection
                bind:from={from}
                bind:to={to} />
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
                items={$nodesQuery.data.nodeMetrics[0].metrics.sort((a, b) => a.name.localeCompare(b.name))}>
                <h4 style="text-align: center;">{item.name}</h4>
                <MetricPlot
                    width={width} height={300} metric={item.name} timestep={item.metric.timestep}
                    cluster={clusters.find(c => c.name == cluster)} subCluster={$nodesQuery.data.nodeMetrics[0].subCluster}
                    series={item.metric.series} />
            </PlotTable>
        {/if}
    </Col>
</Row>
