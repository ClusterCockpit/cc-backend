import {} from './header.entrypoint.js'
import Status from './Status.root.svelte'

new Status({
    target: document.getElementById('svelte-app'),
    props: {
        cluster: infos.cluster,
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
