<script>
    export let host
    export let metric
    export let scope
    export let jobMetrics

    function compareIds(a, b) {
        return a.id - b.id;
    }

    $: series = jobMetrics
        .find(jm => jm.name == metric && jm.scope == scope)
        ?.metric.series.filter(s => s.hostname == host && s.statistics != null)
        ?.sort(compareIds)
</script>

{#if series == null || series.length == 0}
    <td colspan={scope == 'node' ? 3 : 4}><i>No data</i></td>    
{:else if series.length == 1 && scope == 'node'}
    <td>
        {series[0].statistics.min}
    </td>
    <td>
        {series[0].statistics.avg}
    </td>
    <td>
        {series[0].statistics.max}
    </td>
{:else}
    <td colspan="4">
        <table style="width: 100%;">
            {#each series as s, i}
                <tr>
                    <th>{s.id ?? i}</th>
                    <td>{s.statistics.min}</td>
                    <td>{s.statistics.avg}</td>
                    <td>{s.statistics.max}</td>
                </tr>
            {/each}
        </table>
    </td>
{/if}
