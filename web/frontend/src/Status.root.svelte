<script>
    import Refresher from './joblist/Refresher.svelte'
    import Roofline, { transformPerNodeData } from './plots/Roofline.svelte'
    import Histogram from './plots/Histogram.svelte'
    import { Row, Col, Spinner, Card, CardHeader, CardTitle, CardBody, Table, Progress, Icon } from 'sveltestrap'
    import { init, formatNumber } from './utils.js'
    import { operationStore, query } from '@urql/svelte'

    const { query: initq } = init()

    export let cluster

    let plotWidths = [], colWidth1 = 0, colWidth2

    let from = new Date(Date.now() - 5 * 60 * 1000), to = new Date(Date.now())
    const mainQuery = operationStore(`query($cluster: String!, $filter: [JobFilter!]!, $metrics: [String!], $from: Time!, $to: Time!) {
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
    }`, {
        cluster: cluster,
        metrics: ['flops_any', 'mem_bw'],
        from: from.toISOString(),
        to: to.toISOString(),
        filter: [{ state: ['running'] }, { cluster: { eq: cluster } }]
    })

    const sumUp = (data, subcluster, metric) => data.reduce((sum, node) => node.subCluster == subcluster
        ? sum + (node.metrics.find(m => m.name == metric)?.metric.series.reduce((sum, series) => sum + series.data[series.data.length - 1], 0) || 0)
        : sum, 0)

    let allocatedNodes = {}, flopRate = {}, flopRateUnit = {}, memBwRate = {}, memBwRateUnit = {}
    $: if ($initq.data && $mainQuery.data) {
        let subClusters = $initq.data.clusters.find(c => c.name == cluster).subClusters
        for (let subCluster of subClusters) {
            allocatedNodes[subCluster.name] = $mainQuery.data.allocatedNodes.find(({ name }) => name == subCluster.name)?.count || 0
            flopRate[subCluster.name] = Math.floor(sumUp($mainQuery.data.nodeMetrics, subCluster.name, 'flops_any') * 100) / 100
            flopRateUnit[subCluster.name] = subCluster.flopRateSimd.unit.prefix + subCluster.flopRateSimd.unit.base
            memBwRate[subCluster.name] = Math.floor(sumUp($mainQuery.data.nodeMetrics, subCluster.name, 'mem_bw') * 100) / 100
            memBwRateUnit[subCluster.name] = subCluster.memoryBandwidth.unit.prefix + subCluster.memoryBandwidth.unit.base
        }
    }

    query(mainQuery)
</script>

<!-- Loading indicator & Refresh -->

<Row>
    <Col xs="auto" style="align-self: flex-end;">
        <h4 class="mb-0" >Current usage of cluster "{cluster}"</h4>
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
            console.log('reload...')

            from = new Date(Date.now() - 5 * 60 * 1000)
            to = new Date(Date.now())

            $mainQuery.variables = { ...$mainQuery.variables, from: from, to: to }
            $mainQuery.reexecute({ requestPolicy: 'network-only' })
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
                        <Table>
                            <tr>
                                <th scope="col">Allocated Nodes</th>
                                <td style="min-width: 100px;"><div class="col"><Progress value={allocatedNodes[subCluster.name]} max={subCluster.numberOfNodes}/></div></td>
                                <td>({allocatedNodes[subCluster.name]} Nodes / {subCluster.numberOfNodes} Total Nodes)</td>
                            </tr>
                            <tr>
                                <th scope="col">Flop Rate (Any) <Icon name="info-circle" class="p-1" style="cursor: help;" title="Flops[Any] = (Flops[Double] x 2) + Flops[Single]"/></th>
                                <td style="min-width: 100px;"><div class="col"><Progress value={flopRate[subCluster.name]} max={subCluster.flopRateSimd.value * subCluster.numberOfNodes}/></div></td>
                                <td>({flopRate[subCluster.name]} {flopRateUnit[subCluster.name]} / {(subCluster.flopRateSimd.value * subCluster.numberOfNodes)} {flopRateUnit[subCluster.name]} [Max])</td>
                            </tr>
                            <tr>
                                <th scope="col">MemBw Rate</th>
                                <td style="min-width: 100px;"><div class="col"><Progress value={memBwRate[subCluster.name]} max={subCluster.memoryBandwidth.value * subCluster.numberOfNodes}/></div></td>
                                <td>({memBwRate[subCluster.name]} {memBwRateUnit[subCluster.name]} / {(subCluster.memoryBandwidth.value * subCluster.numberOfNodes)} {memBwRateUnit[subCluster.name]} [Max])</td>
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
                <h4 class="mb-3 text-center">Top Users</h4>
                {#key $mainQuery.data}
                    <Histogram
                        width={colWidth1 - 25}
                        data={$mainQuery.data.topUsers.sort((a, b) => b.count - a.count).map(({ count }, idx) => ({ count, value: idx }))}
                        label={(x) => x < $mainQuery.data.topUsers.length ? $mainQuery.data.topUsers[Math.floor(x)].name : '0'}
                        xlabel="User Name" ylabel="Number of Jobs" />
                {/key}
            </div>
        </Col>
        <Col class="px-4 py-2">
            <Table>
                <tr class="mb-2"><th>User Name</th><th>Number of Nodes</th></tr>
                {#each $mainQuery.data.topUsers.sort((a, b) => b.count - a.count) as { name, count }}
                    <tr>
                        <th scope="col"><a href="/monitoring/user/{name}">{name}</a></th>
                        <td>{count}</td>
                    </tr>
                {/each}
            </Table>
        </Col>
        <Col class="p-2">
            <h4 class="mb-3 text-center">Top Projects</h4>
            {#key $mainQuery.data}
                <Histogram
                    width={colWidth1 - 25}
                    data={$mainQuery.data.topProjects.sort((a, b) => b.count - a.count).map(({ count }, idx) => ({ count, value: idx }))}
                    label={(x) => x < $mainQuery.data.topProjects.length ? $mainQuery.data.topProjects[Math.floor(x)].name : '0'}
                    xlabel="Project Code" ylabel="Number of Jobs" />
            {/key}
        </Col>
        <Col class="px-4 py-2">
            <Table>
                <tr class="mb-2"><th>Project Code</th><th>Number of Nodes</th></tr>
                {#each $mainQuery.data.topProjects.sort((a, b) => b.count - a.count) as { name, count }}
                    <tr><th scope="col">{name}</th><td>{count}</td></tr>
                {/each}
            </Table>
        </Col>
    </Row>
    <Row cols={2} class="mt-3">
        <Col class="p-2">
            <div bind:clientWidth={colWidth2}>
                <h4 class="mb-3 text-center">Duration Distribution</h4>
                {#key $mainQuery.data.stats}
                    <Histogram
                        width={colWidth2 - 25}
                        data={$mainQuery.data.stats[0].histDuration}
                        xlabel="Current Runtimes [h]" 
                        ylabel="Number of Jobs" />
                {/key}
            </div>
        </Col>
        <Col class="p-2">
            <h4 class="mb-3 text-center">Number of Nodes Distribution</h4>
            {#key $mainQuery.data.stats}
                <Histogram
                    width={colWidth2 - 25}
                    data={$mainQuery.data.stats[0].histNumNodes}
                    xlabel="Allocated Nodes [#]"
                    ylabel="Number of Jobs" />
            {/key}
        </Col>
    </Row>
{/if}
