import {} from './header.entrypoint.js'
import History from './History.svelte'


new History({
    target: document.getElementById('svelte-app'),
    props: {
        filterPresets: filterPresets
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})