<script>
    import Refresher from './joblist/Refresher.svelte'
    import Roofline, { transformPerNodeData } from './plots/Roofline.svelte'
    import Pie, { colors } from './plots/Pie.svelte'
    import Histogramuplot from './plots/Histogramuplot.svelte'
    import { Row, Col, Spinner, Card, CardHeader, CardTitle, CardBody, Table, Progress, Icon } from 'sveltestrap'
    import { init, convert2uplot } from './utils.js'
    import { scaleNumbers } from './units.js'
    import { queryStore, gql, getContextClient  } from '@urql/svelte'

    const { query: initq } = init()

    export let cluster

    let plotWidths = [], colWidth1 = 0, colWidth2
    let from = new Date(Date.now() - 5 * 60 * 1000), to = new Date(Date.now())

    const client = getContextClient();
    $: mainQuery = queryStore({
        client: client,
        query: gql`query($cluster: String!, $filter: [JobFilter!]!, $metrics: [String!], $from: Time!, $to: Time!) {
        nodeMetrics(cluster: $cluster, metrics: $metrics, from: $from, to: $to) {
            host
            subCluster
            metrics {
                name
                scope
                metric {
                    timestep
                    unit { base, prefix }
                    series { data }
                }
            }
        }

        stats: jobsStatistics(filter: $filter) {
            histDuration { count, value }
            histNumNodes { count, value }
        }

        allocatedNodes(cluster: $cluster)                                                        { name, count }
        topUsers:    jobsCount(filter: $filter, groupBy: USER,    weight: NODE_COUNT, limit: 10) { name, count }
        topProjects: jobsCount(filter: $filter, groupBy: PROJECT, weight: NODE_COUNT, limit: 10) { name, count }
    }`,
    variables: {
         cluster: cluster, metrics: ['flops_any', 'mem_bw'], from: from.toISOString(), to: to.toISOString(),
        filter: [{ state: ['running'] }, { cluster: { eq: cluster } }]
    }
    })

    const sumUp = (data, subcluster, metric) => data.reduce((sum, node) => node.subCluster == subcluster
        ? sum + (node.metrics.find(m => m.name == metric)?.metric.series.reduce((sum, series) => sum + series.data[series.data.length - 1], 0) || 0)
        : sum, 0)

    let allocatedNodes = {}, flopRate = {}, flopRateUnitPrefix = {}, flopRateUnitBase = {}, memBwRate = {}, memBwRateUnitPrefix = {}, memBwRateUnitBase = {}
    $: if ($initq.data && $mainQuery.data) {
        let subClusters = $initq.data.clusters.find(c => c.name == cluster).subClusters
        for (let subCluster of subClusters) {
            allocatedNodes[subCluster.name]     = $mainQuery.data.allocatedNodes.find(({ name }) => name == subCluster.name)?.count || 0
            flopRate[subCluster.name]           = Math.floor(sumUp($mainQuery.data.nodeMetrics, subCluster.name, 'flops_any') * 100) / 100
            flopRateUnitPrefix[subCluster.name] = subCluster.flopRateSimd.unit.prefix
            flopRateUnitBase[subCluster.name]   = subCluster.flopRateSimd.unit.base
            memBwRate[subCluster.name]          = Math.floor(sumUp($mainQuery.data.nodeMetrics, subCluster.name, 'mem_bw') * 100) / 100
            memBwRateUnitPrefix[subCluster.name] = subCluster.memoryBandwidth.unit.prefix
            memBwRateUnitBase[subCluster.name]  = subCluster.memoryBandwidth.unit.base
        }
    }

</script>

<!-- Loading indicator & Refresh -->

<Row>
    <Col xs="auto" style="align-self: flex-end;">
        <h4 class="mb-0" >Current utilization of cluster "{cluster}"</h4>
    </Col>
    <Col xs="auto">
        {#if $initq.fetching || $mainQuery.fetching}
            <Spinner/>
        {:else if $initq.error}
            <Card body color="danger">{$initq.error.message}</Card>
        {:else}
            <!-- ... -->
        {/if}
    </Col>
    <Col xs="auto" style="margin-left: auto;">
        <Refresher initially={120} on:reload={() => {
            from = new Date(Date.now() - 5 * 60 * 1000)
            to = new Date(Date.now())
        }} />
    </Col>
</Row>
{#if $mainQuery.error}
    <Row>
        <Col>
            <Card body color="danger">{$mainQuery.error.message}</Card>
        </Col>
    </Row>
{/if}

<hr>

<!-- Gauges & Roofline per Subcluster-->

{#if $initq.data && $mainQuery.data}
    {#each $initq.data.clusters.find(c => c.name == cluster).subClusters as subCluster, i}
        <Row cols={2} class="mb-3 justify-content-center">
            <Col xs="4" class="px-3">
                <Card class="h-auto mt-1">
                    <CardHeader>
                        <CardTitle class="mb-0">SubCluster "{subCluster.name}"</CardTitle>
                    </CardHeader>
                    <CardBody>
                        <Table borderless>
                            <tr class="py-2">
                                <th scope="col">Allocated Nodes</th>
                                <td style="min-width: 100px;"><div class="col"><Progress value={allocatedNodes[subCluster.name]} max={subCluster.numberOfNodes}/></div></td>
                                <td>{allocatedNodes[subCluster.name]} / {subCluster.numberOfNodes} Nodes</td>
                            </tr>
                            <tr class="py-2">
                                <th scope="col">Flop Rate (Any) <Icon name="info-circle" class="p-1" style="cursor: help;" title="Flops[Any] = (Flops[Double] x 2) + Flops[Single]"/></th>
                                <td style="min-width: 100px;"><div class="col"><Progress value={flopRate[subCluster.name]} max={subCluster.flopRateSimd.value * subCluster.numberOfNodes}/></div></td>
                                <td>
                                    {scaleNumbers(flopRate[subCluster.name], 
                                                  (subCluster.flopRateSimd.value * subCluster.numberOfNodes),
                                                  flopRateUnitPrefix[subCluster.name])
                                    }{flopRateUnitBase[subCluster.name]} [Max]
                                </td>
                            </tr>
                            <tr class="py-2">
                                <th scope="col">MemBw Rate</th>
                                <td style="min-width: 100px;"><div class="col"><Progress value={memBwRate[subCluster.name]} max={subCluster.memoryBandwidth.value * subCluster.numberOfNodes}/></div></td>
                                <td>
                                    {scaleNumbers(memBwRate[subCluster.name],
                                                  (subCluster.memoryBandwidth.value * subCluster.numberOfNodes), 
                                                  memBwRateUnitPrefix[subCluster.name])
                                    }{memBwRateUnitBase[subCluster.name]} [Max]
                                </td>
                            </tr>
                        </Table>
                    </CardBody>
                </Card>
            </Col>
            <Col class="px-3">
                <div bind:clientWidth={plotWidths[i]}>
                    {#key $mainQuery.data.nodeMetrics}
                        <Roofline
                            width={plotWidths[i] - 10} height={300} colorDots={true} showTime={false} cluster={subCluster}
                            data={transformPerNodeData($mainQuery.data.nodeMetrics.filter(data => data.subCluster == subCluster.name))} />
                    {/key}
                </div>
            </Col>
        </Row>
    {/each}

    <hr style="margin-top: -1em;">

    <!-- Usage Stats as Histograms -->

    <Row cols={4}>
        <Col class="p-2">
            <div bind:clientWidth={colWidth1}>
                <h4 class="text-center">Top Users</h4>
                {#key $mainQuery.data}
                    <Pie
                        size={colWidth1}
                        sliceLabel='Jobs'
                        quantities={$mainQuery.data.topUsers.sort((a, b) => b.count - a.count).map((tu) => tu.count)}
                        entities={$mainQuery.data.topUsers.sort((a, b) => b.count - a.count).map((tu) => tu.name)}
                        
                    />
                {/key}
            </div>
        </Col>
        <Col class="px-4 py-2">
            <Table>
                <tr class="mb-2"><th>Legend</th><th>User Name</th><th>Number of Nodes</th></tr>
                {#each $mainQuery.data.topUsers.sort((a, b) => b.count - a.count) as { name, count }, i}
                    <tr>
                        <td><Icon name="circle-fill" style="color: {colors[i]};"/></td>
                        <th scope="col"><a href="/monitoring/user/{name}?cluster={cluster}&state=running">{name}</a></th>
                        <td>{count}</td>
                    </tr>
                {/each}
            </Table>
        </Col>
        <Col class="p-2">
            <h4 class="text-center">Top Projects</h4>
            {#key $mainQuery.data}
                <Pie
                    size={colWidth1}
                    sliceLabel='Jobs'
                    quantities={$mainQuery.data.topProjects.sort((a, b) => b.count - a.count).map((tp) => tp.count)}
                    entities={$mainQuery.data.topProjects.sort((a, b) => b.count - a.count).map((tp) => tp.name)}
                />
            {/key}
        </Col>
        <Col class="px-4 py-2">
            <Table>
                <tr class="mb-2"><th>Legend</th><th>Project Code</th><th>Number of Nodes</th></tr>
                {#each $mainQuery.data.topProjects.sort((a, b) => b.count - a.count) as { name, count }, i}
                    <tr>
                        <td><Icon name="circle-fill" style="color: {colors[i]};"/></td>
                        <th scope="col"><a href="/monitoring/jobs/?cluster={cluster}&state=running&project={name}&projectMatch=eq">{name}</a></th>
                        <td>{count}</td>
                    </tr>
                {/each}
            </Table>
        </Col>
    </Row>
    <hr class="my-2"/>
    <Row cols={2}>
        <Col class="p-2">
            <div bind:clientWidth={colWidth2}>
                {#key $mainQuery.data.stats}
                    <Histogramuplot
                        data={convert2uplot($mainQuery.data.stats[0].histDuration)}
                        width={colWidth2 - 25}
                        title="Duration Distribution"
                        xlabel="Current Runtimes"
                        xunit="Hours" 
                        ylabel="Number of Jobs"
                        yunit="Jobs"/>
                {/key}
            </div>
        </Col>
        <Col class="p-2">
            {#key $mainQuery.data.stats}
                <Histogramuplot
                    data={convert2uplot($mainQuery.data.stats[0].histNumNodes)}
                    width={colWidth2 - 25}
                    title="Number of Nodes Distribution"
                    xlabel="Allocated Nodes"
                    xunit="Nodes" 
                    ylabel="Number of Jobs"
                    yunit="Jobs"/>
            {/key}
        </Col>
    </Row>
{/if}

<style>
    .colorBoxWrapper {
        display: flex;
    }

    .colorBox {
        width: 20px;
        height: 20px;
        border: 1px solid rgba(0, 0, 0, .2);
    }
</style>