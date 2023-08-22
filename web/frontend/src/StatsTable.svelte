<script>
    import { getContext } from 'svelte'
    import { Button, Table, InputGroup, InputGroupText, Icon } from 'sveltestrap'
    import MetricSelection from './MetricSelection.svelte'
    import StatsTableEntry from './StatsTableEntry.svelte'
    import { maxScope } from './utils.js'

    export let job
    export let jobMetrics
    export let accMetrics
    export let accNodeOnly

    const allMetrics = [...new Set(jobMetrics.map(m => m.name))].sort(),
          scopesForMetric = (metric) => jobMetrics
            .filter(jm => jm.name == metric)
            .map(jm => jm.scope)

    let hosts = job.resources.map(r => r.hostname).sort(),
        selectedScopes = {},
        sorting = {},
        isMetricSelectionOpen = false,
        selectedMetrics = getContext('cc-config')[`job_view_nodestats_selectedMetrics:${job.cluster}`]
            || getContext('cc-config')['job_view_nodestats_selectedMetrics']
            
    for (let metric of allMetrics) {
        // Not Exclusive or Single Node: Get maxScope()
        // No Accelerators in Job and not Acc-Metric: Use 'core'
        // Accelerator Metric available on accelerator scope: Use 'accelerator'
        // Accelerator Metric only on node scope: Fallback to 'node'
        selectedScopes[metric] = (job.exclusive != 1 || job.numNodes == 1) ?
                                   (job.numAccs != 0 && accMetrics.includes(metric)) ?
                                     accNodeOnly ?
                                       'node'
                                     : 'accelerator' 
                                   : 'core'
                                 : maxScope(scopesForMetric(metric))
        sorting[metric] = {
            min: { dir: 'up', active: false },
            avg: { dir: 'up', active: false },
            max: { dir: 'up', active: false },
        }
    }

    export function sortBy(metric, stat) {
        let s = sorting[metric][stat]
        if (s.active) {
            s.dir = s.dir == 'up' ? 'down' : 'up'
        } else {
            for (let metric in sorting)
                for (let stat in sorting[metric])
                    sorting[metric][stat].active = false
            s.active = true
        }

        let series = jobMetrics.find(jm => jm.name == metric && jm.scope == 'node')?.metric.series
        sorting = {...sorting}
        hosts = hosts.sort((h1, h2) => {
            let s1 = series.find(s => s.hostname == h1)?.statistics
            let s2 = series.find(s => s.hostname == h2)?.statistics
            if (s1 == null || s2 == null)
                return -1

            return s.dir != 'up' ? s1[stat] - s2[stat] : s2[stat] - s1[stat]
        })
    }

    export function moreLoaded(jobMetric) {
        jobMetrics = [...jobMetrics, jobMetric]
    }
</script>

<Table>
    <thead>
        <tr>
            <th>
                <Button outline on:click={() => (isMetricSelectionOpen = true)}> <!-- log to click ', console.log(isMetricSelectionOpen)' -->
                    Metrics
                </Button>
            </th>
            {#each selectedMetrics as metric}
                <th colspan={selectedScopes[metric] == 'node' ? 3 : 4}>
                    <InputGroup>
                        <InputGroupText>
                            {metric}
                        </InputGroupText>
                        <select class="form-select"
                            bind:value={selectedScopes[metric]}
                            disabled={scopesForMetric(metric, jobMetrics).length == 1}>
                            {#each scopesForMetric(metric, jobMetrics) as scope}
                                <option value={scope}>{scope}</option>
                            {/each}
                        </select>
                    </InputGroup>
                </th>
            {/each}
        </tr>
        <tr>
            <th>Node</th>
            {#each selectedMetrics as metric}
                {#if selectedScopes[metric] != 'node'}
                    <th>Id</th>
                {/if}
                {#each ['min', 'avg', 'max'] as stat}
                    <th on:click={() => sortBy(metric, stat)}>
                        {stat}
                        {#if selectedScopes[metric] == 'node'}
                            <Icon name="caret-{sorting[metric][stat].dir}{sorting[metric][stat].active ? '-fill' : ''}" />
                        {/if}
                    </th>
                {/each}
            {/each}
        </tr>
    </thead>
    <tbody>
        {#each hosts as host (host)}
            <tr>
                <th scope="col">{host}</th>
                {#each selectedMetrics as metric (metric)}
                    <StatsTableEntry
                        host={host} metric={metric}
                        scope={selectedScopes[metric]}
                        jobMetrics={jobMetrics} />
                {/each}
            </tr>
        {/each}
    </tbody>
</Table>

<br/>

<MetricSelection
    cluster={job.cluster}
    configName='job_view_nodestats_selectedMetrics'
    allMetrics={new Set(allMetrics)}
    bind:metrics={selectedMetrics}
    bind:isOpen={isMetricSelectionOpen} />
