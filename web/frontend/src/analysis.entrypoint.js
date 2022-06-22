import {} from './header.entrypoint.js'
import Analysis from './Analysis.root.svelte'

filterPresets.cluster = cluster

new Analysis({
    target: document.getElementById('svelte-app'),
    props: {
        filterPresets: filterPresets
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
