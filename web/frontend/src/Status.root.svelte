<script>
    import Refresher from './joblist/Refresher.svelte'
    import Roofline, { transformPerNodeData } from './plots/Roofline.svelte'
    import Histogram from './plots/Histogram.svelte'
    import { Row, Col, Spinner, Card, Table, Progress } from 'sveltestrap'
    import { init } from './utils.js'
    import { operationStore, query } from '@urql/svelte'

    const { query: initq } = init()

    export let cluster

    let plotWidths = [], colWidth1 = 0, colWidth2

    let from = new Date(Date.now() - 5 * 60 * 1000), to = new Date(Date.now())
    const mainQuery = operationStore(`query($cluster: String!, $filter: [JobFilter!]!, $metrics: [String!], $from: Time!, $to: Time!) {
        nodeMetrics(cluster: $cluster, metrics: $metrics, from: $from, to: $to) {
            host,
            subCluster,
            metrics {
                name,
                metric {
                    scope
                    timestep,
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

    let allocatedNodes = {}, flopRate = {}, memBwRate = {}
    $: if ($initq.data && $mainQuery.data) {
        let subClusters = $initq.data.clusters.find(c => c.name == cluster).subClusters
        for (let subCluster of subClusters) {
            allocatedNodes[subCluster.name] = $mainQuery.data.allocatedNodes.find(({ name }) => name == subCluster.name)?.count || 0
            flopRate[subCluster.name] = Math.floor(sumUp($mainQuery.data.nodeMetrics, subCluster.name, 'flops_any') * 100) / 100
            memBwRate[subCluster.name] = Math.floor(sumUp($mainQuery.data.nodeMetrics, subCluster.name, 'mem_bw') * 100) / 100
        }
    }

    query(mainQuery)
</script>

<Row>
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
{#if $initq.data && $mainQuery.data}
    {#each $initq.data.clusters.find(c => c.name == cluster).subClusters as subCluster, i}
        <Row>
            <Col xs="3">
                <Table>
                    <tr>
                        <th scope="col">SubCluster</th>
                        <td colspan="2">{subCluster.name}</td>
                    </tr>
                    <tr>
                        <th scope="col">Allocated Nodes</th>
                        <td style="min-width: 75px;"><div class="col"><Progress value={allocatedNodes[subCluster.name]} max={subCluster.numberOfNodes}/></div></td>
                        <td>({allocatedNodes[subCluster.name]} / {subCluster.numberOfNodes})</td>
                    </tr>
                    <tr>
                        <th scope="col">Flop Rate</th>
                        <td style="min-width: 75px;"><div class="col"><Progress value={flopRate[subCluster.name]} max={subCluster.flopRateSimd * subCluster.numberOfNodes}/></div></td>
                        <td>({flopRate[subCluster.name]} / {subCluster.flopRateSimd * subCluster.numberOfNodes})</td>
                    </tr>
                    <tr>
                        <th scope="col">MemBw Rate</th>
                        <td style="min-width: 75px;"><div class="col"><Progress value={memBwRate[subCluster.name]} max={subCluster.memoryBandwidth * subCluster.numberOfNodes}/></div></td>
                        <td>({memBwRate[subCluster.name]} / {subCluster.memoryBandwidth * subCluster.numberOfNodes})</td>
                    </tr>
                </Table>
            </Col>
            <div class="col-9" bind:clientWidth={plotWidths[i]}>
                {#key $mainQuery.data.nodeMetrics}
                    <Roofline
                        width={plotWidths[i] - 10} height={300} colorDots={false} cluster={subCluster}
                        data={transformPerNodeData($mainQuery.data.nodeMetrics.filter(data => data.subCluster == subCluster.name))} />
                {/key}
            </div>
        </Row>
    {/each}
    <Row>
        <div class="col-4" bind:clientWidth={colWidth1}>
            <h4>Top Users</h4>
            {#key $mainQuery.data}
                <Histogram
                    width={colWidth1 - 25} height={300}
                    data={$mainQuery.data.topUsers.sort((a, b) => b.count - a.count).map(({ count }, idx) => ({ count, value: idx }))}
                    label={(x) => x < $mainQuery.data.topUsers.length ? $mainQuery.data.topUsers[Math.floor(x)].name : '0'} />
            {/key}
        </div>
        <div class="col-2">
            <Table>
                <tr><th>Name</th><th>Number of Nodes</th></tr>
                {#each $mainQuery.data.topUsers.sort((a, b) => b.count - a.count) as { name, count }}
                    <tr>
                        <th scope="col"><a href="/monitoring/user/{name}">{name}</a></th>
                        <td>{count}</td>
                    </tr>
                {/each}
            </Table>
        </div>
        <div class="col-4">
            <h4>Top Projects</h4>
            {#key $mainQuery.data}
                <Histogram
                    width={colWidth1 - 25} height={300}
                    data={$mainQuery.data.topProjects.sort((a, b) => b.count - a.count).map(({ count }, idx) => ({ count, value: idx }))}
                    label={(x) => x < $mainQuery.data.topProjects.length ? $mainQuery.data.topProjects[Math.floor(x)].name : '0'} />
            {/key}
        </div>
        <div class="col-2">
            <Table>
                <tr><th>Name</th><th>Number of Nodes</th></tr>
                {#each $mainQuery.data.topProjects.sort((a, b) => b.count - a.count) as { name, count }}
                    <tr><th scope="col">{name}</th><td>{count}</td></tr>
                {/each}
            </Table>
        </div>
    </Row>
    <Row>
        <div class="col" bind:clientWidth={colWidth2}>
            <h4>Duration Distribution</h4>
            {#key $mainQuery.data.stats}
                <Histogram
                    width={colWidth2 - 25} height={300}
                    data={$mainQuery.data.stats[0].histDuration} />
            {/key}
        </div>
        <div class="col">
            <h4>Number of Nodes Distribution</h4>
            {#key $mainQuery.data.stats}
                <Histogram
                    width={colWidth2 - 25} height={300}
                    data={$mainQuery.data.stats[0].histNumNodes} />
            {/key}
        </div>
    </Row>
{/if}
