import {} from './header.entrypoint.js'
import Partitions from './Partitions.root.svelte'

new Partitions({
    target: document.getElementById('svelte-app'),
    props: {
        cluster: infos.cluster,
        from: infos.from,
        to: infos.to
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
