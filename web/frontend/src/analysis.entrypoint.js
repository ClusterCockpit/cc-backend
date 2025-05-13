import {} from './header.entrypoint.js'
import Analysis from './Analysis.root.svelte'

filterPresets.cluster = cluster

new Analysis({
    target: document.getElementById('svelte-app'),
    props: {
        filterPresets: filterPresets,
        cluster: cluster
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
