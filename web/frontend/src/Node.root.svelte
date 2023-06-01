<script>
    import { init } from './utils.js'
    import { Row, Col, InputGroup, InputGroupText, Icon, Spinner, Card } from 'sveltestrap'
    import { queryStore, gql, getContextClient } from '@urql/svelte'
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

    const ccconfig = getContext('cc-config')
    const clusters = getContext('clusters')
    const client = getContextClient();
    const query = gql`query($cluster: String!, $nodes: [String!], $from: Time!, $to: Time!) {
        nodeMetrics(cluster: $cluster, nodes: $nodes, from: $from, to: $to) {
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
    }`;

    $: nodesQuery = queryStore({
        client: client,
        query: query,
        variables: {
            cluster: cluster,
            nodes: [hostname],
            from: from.toISOString(),
            to: to.toISOString(),
        }
    });

    let metricUnits = {}
    $: if ($nodesQuery.data) {
        for (let metric of clusters.find(c => c.name == cluster).metricConfig) {
            if (metric.unit.prefix || metric.unit.base) {
                metricUnits[metric.name] = '(' + (metric.unit.prefix ? metric.unit.prefix : '') + (metric.unit.base ? metric.unit.base : '') + ')'
            } else { // If no unit defined: Omit Unit Display
                metricUnits[metric.name] = ''
            }
        }
    }
    // $: console.log($nodesQuery?.data?.nodeMetrics[0].metrics)
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
                <h4 style="text-align: center;">{item.name} {metricUnits[item.name]}</h4>
                <MetricPlot
                    width={width} height={300} metric={item.name} timestep={item.metric.timestep}
                    cluster={clusters.find(c => c.name == cluster)} subCluster={$nodesQuery.data.nodeMetrics[0].subCluster}
                    series={item.metric.series} />
            </PlotTable>
        {/if}
    </Col>
</Row>
