<script>
    import { getContext } from 'svelte'
    import { init } from './utils.js'
    import { Card, CardHeader, CardTitle } from 'sveltestrap'

    import PlotSettings from './config/PlotSettings.svelte'
    import AdminSettings from './config/AdminSettings.svelte'

    const { query: initq } = init()

    const ccconfig = getContext('cc-config')

    export let isAdmin

</script>

{#if isAdmin == true}
<Card style="margin-bottom: 1.5em;">
    <CardHeader>
        <CardTitle class="mb-1">Admin Options</CardTitle>
    </CardHeader>
    <AdminSettings/>
</Card>
{/if}

<Card>
    <CardHeader>
        <CardTitle class="mb-1">Plotting Options</CardTitle>
    </CardHeader>
    <PlotSettings config={ccconfig}/>
</Card>
