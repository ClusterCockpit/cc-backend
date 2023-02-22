import {} from './header.entrypoint.js'
import Jobs from './Jobs.root.svelte'

new Jobs({
    target: document.getElementById('svelte-app'),
    props: {
        filterPresets: filterPresets,
        authLevel: authLevel
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
